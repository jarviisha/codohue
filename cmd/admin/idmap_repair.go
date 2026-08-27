package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jarviisha/codohue/internal/compute"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// repairRunner is the subset of idmap.RepairService the CLI drives. Declared
// as an interface so argument handling and reporting can be tested without a
// live PostgreSQL and Qdrant.
type repairRunner interface {
	Audit(ctx context.Context) (*idmap.AuditReport, error)
	PrepareSnapshots(ctx context.Context, runID int64, pgRef string, qdrantRefs map[string]string) error
	Apply(ctx context.Context, runID int64) error
	Verify(ctx context.Context, runID int64) (*idmap.VerifyReport, error)
	Resume(ctx context.Context, runID int64) error
	QuarantineReport(ctx context.Context, runID int64) ([]idmap.RepairItem, error)
}

const idmapRepairUsage = `usage: idmap-repair <audit|quarantine|apply|verify|resume>

  audit                                          inventory both stores, record an immutable manifest (read-only)
  quarantine --run <id>                          re-list what is blocking a run, without re-auditing
  apply  --run <id> --pg-snapshot <ref> \
         --qdrant-snapshot <collection>=<ref>    move identities onto their authoritative numeric ids
  verify --run <id>                              prove every manifest tuple before unlocking the fleet
  resume --run <id>                              continue a failed run from durable item state`

// runIdmapRepairCommand parses and dispatches one idmap-repair invocation.
func runIdmapRepairCommand(ctx context.Context, args []string, runner repairRunner, out *strings.Builder) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", idmapRepairUsage)
	}
	switch args[0] {
	case "audit":
		return runRepairAudit(ctx, runner, out)
	case "quarantine":
		return runRepairQuarantine(ctx, args[1:], runner, out)
	case "apply":
		return runRepairApply(ctx, args[1:], runner, out)
	case "verify":
		return runRepairVerify(ctx, args[1:], runner, out)
	case "resume":
		return runRepairResume(ctx, args[1:], runner, out)
	default:
		return fmt.Errorf("unknown idmap-repair subcommand %q\n%s", args[0], idmapRepairUsage)
	}
}

func runRepairAudit(ctx context.Context, runner repairRunner, out *strings.Builder) error {
	report, err := runner.Audit(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "run %d audited: %d identities, %d resolved, %d need repair, %d quarantined\n",
		report.RunID, report.Total, report.Resolved, report.NeedsRepair, len(report.Quarantined))
	fmt.Fprintf(out, "manifest %s\n", report.ManifestHash)
	if len(report.Quarantined) > 0 {
		// Printed in full rather than sampled: the operator has to resolve
		// every one of these before apply will run, so showing a subset only
		// means another audit round.
		fmt.Fprintf(out, "\nquarantined — apply is blocked until each is resolved:\n")
		for _, item := range report.Quarantined {
			fmt.Fprintf(out, "  %s/%s/%s: %s\n", item.Namespace, item.EntityType, item.StringID, item.Error)
		}
	}
	return nil
}

func runRepairApply(ctx context.Context, args []string, runner repairRunner, out *strings.Builder) error {
	flags := flag.NewFlagSet("idmap-repair apply", flag.ContinueOnError)
	runID := flags.Int64("run", 0, "audited run id")
	pgSnapshot := flags.String("pg-snapshot", "", "PostgreSQL backup reference taken at the same checkpoint")
	var qdrantSnapshots snapshotRefs
	flags.Var(&qdrantSnapshots, "qdrant-snapshot", "collection=snapshot reference (repeatable)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse idmap-repair apply flags: %w", err)
	}
	if *runID <= 0 || *pgSnapshot == "" || len(qdrantSnapshots) == 0 {
		// Snapshots are required arguments, not optional hygiene: apply
		// deletes points that may not be recomputable, so a run with no
		// recorded recovery point must not start.
		return fmt.Errorf("apply requires --run, --pg-snapshot and at least one --qdrant-snapshot\n%s", idmapRepairUsage)
	}

	if err := runner.PrepareSnapshots(ctx, *runID, *pgSnapshot, qdrantSnapshots); err != nil {
		return err
	}
	if err := runner.Apply(ctx, *runID); err != nil {
		return err
	}
	fmt.Fprintf(out, "run %d applied; verify before unlocking the fleet\n", *runID)
	return nil
}

func runRepairVerify(ctx context.Context, args []string, runner repairRunner, out *strings.Builder) error {
	runID, err := parseRunFlag("idmap-repair verify", args)
	if err != nil {
		return err
	}
	report, err := runner.Verify(ctx, runID)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "run %d verified %d identities\n", report.RunID, report.Checked)
	if len(report.Remaining) == 0 && len(report.Unmoved) == 0 {
		fmt.Fprintf(out, "complete: every tuple is on its authoritative id and no old point remains\n")
		return nil
	}
	for _, item := range report.Remaining {
		fmt.Fprintf(out, "  unfinished %s/%s/%s (state %s)\n", item.Namespace, item.EntityType, item.StringID, item.State)
	}
	// Problems carry the reason each item failed; without them the operator is
	// left comparing point ids by hand to work out why the gate refused.
	for _, problem := range report.Problems {
		fmt.Fprintf(out, "  %s\n", problem)
	}
	return fmt.Errorf("run %d is not verifiable: %d unfinished, %d old point(s) present",
		runID, len(report.Remaining), len(report.Unmoved))
}

// runRepairQuarantine re-lists a run's blockers.
//
// Needed because apply refuses while anything is quarantined and the operator
// works through the list over time — re-auditing just to see what is left
// would discard the manifest they are working from.
func runRepairQuarantine(ctx context.Context, args []string, runner repairRunner, out *strings.Builder) error {
	runID, err := parseRunFlag("idmap-repair quarantine", args)
	if err != nil {
		return err
	}
	items, err := runner.QuarantineReport(ctx, runID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintf(out, "run %d has no quarantined items; apply is not blocked\n", runID)
		return nil
	}
	fmt.Fprintf(out, "run %d is blocked by %d unresolved item(s):\n", runID, len(items))
	for _, item := range items {
		fmt.Fprintf(out, "  %s/%s/%s: %s\n", item.Namespace, item.EntityType, item.StringID, item.Error)
	}
	return nil
}

func runRepairResume(ctx context.Context, args []string, runner repairRunner, out *strings.Builder) error {
	runID, err := parseRunFlag("idmap-repair resume", args)
	if err != nil {
		return err
	}
	if err := runner.Resume(ctx, runID); err != nil {
		return err
	}
	fmt.Fprintf(out, "run %d resumed; verify before unlocking the fleet\n", runID)
	return nil
}

func parseRunFlag(name string, args []string) (int64, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	runID := flags.Int64("run", 0, "repair run id")
	if err := flags.Parse(args); err != nil {
		return 0, fmt.Errorf("parse %s flags: %w", name, err)
	}
	if *runID <= 0 {
		return 0, fmt.Errorf("%s requires --run\n%s", name, idmapRepairUsage)
	}
	return *runID, nil
}

// snapshotRefs collects repeated --qdrant-snapshot collection=ref flags.
type snapshotRefs map[string]string

func (r *snapshotRefs) String() string {
	if r == nil || len(*r) == 0 {
		return ""
	}
	encoded, err := json.Marshal(*r)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func (r *snapshotRefs) Set(value string) error {
	collection, ref, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(collection) == "" || strings.TrimSpace(ref) == "" {
		return fmt.Errorf("expected collection=snapshot_reference, got %q", value)
	}
	if *r == nil {
		*r = snapshotRefs{}
	}
	(*r)[strings.TrimSpace(collection)] = strings.TrimSpace(ref)
	return nil
}

// runIdmapRepairCLI is the wiring entry point registered in main.
func runIdmapRepairCLI(args []string) error {
	service, cleanup, err := buildRepairService(context.Background())
	if err != nil {
		return err
	}
	defer cleanup()

	var out strings.Builder
	runErr := runIdmapRepairCommand(context.Background(), args, service, &out)
	if out.Len() > 0 {
		if _, err := fmt.Fprint(os.Stdout, out.String()); err != nil {
			return errors.Join(runErr, fmt.Errorf("write repair report: %w", err))
		}
	}
	return runErr
}

// buildRepairService assembles the reconciliation from the same clients the
// admin server uses. Returned with a cleanup because the CLI owns the
// connections for the life of one command.
func buildRepairService(ctx context.Context) (*idmap.RepairService, func(), error) {
	cfg, err := config.LoadAdmin()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	db, err := infrapg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	qdrantClient, err := infraqdrant.NewClient(cfg.QdrantHost, cfg.QdrantPort)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("connect qdrant: %w", err)
	}

	computeRepo := compute.NewRepository(db)
	idmapSvc := idmap.NewService(idmap.NewRepository(db))
	computeSvc := compute.NewService(computeRepo, idmapSvc, qdrantClient)
	lifecycleLocker, err := nslifecycle.NewPostgresLocker(db)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("create lifecycle locker: %w", err)
	}
	lifecycleSvc := nslifecycle.NewService(nslifecycle.NewRepository(db), lifecycleLocker)
	nsConfigSvc := nsconfig.NewService(nsconfig.NewRepository(db))

	repairRepo := idmap.NewRepairRepository(db)
	// One definition of "which generation is this namespace on", shared by the
	// audit (which derives physical collection names from it) and the rebuild
	// (which asserts a lease for it). Two resolvers could disagree and the
	// repair would inspect one generation while rebuilding another.
	generationOf := func(ctx context.Context, namespace string) (int64, error) {
		nsCfg, err := nsConfigSvc.Get(ctx, namespace)
		if err != nil {
			return 0, fmt.Errorf("load config for %q: %w", namespace, err)
		}
		if nsCfg == nil || nsCfg.Generation < 1 {
			return 1, nil
		}
		return nsCfg.Generation, nil
	}
	evidence := &repairEvidenceSource{
		repo:       repairRepo,
		qdrant:     qdrantClient,
		namespaces: computeRepo.GetActiveNamespaces,
		generation: generationOf,
	}

	service := idmap.NewRepairService(
		repairRepo,
		evidence,
		&qdrantPointMover{client: qdrantClient},
		&sparseRebuildAdapter{svc: computeSvc, generation: generationOf, lambda: defaultRepairLambda},
		&globalFenceAdapter{svc: lifecycleSvc},
	)
	return service, func() { lifecycleLocker.Close(); db.Close() }, nil
}

// defaultRepairLambda matches the compute job's fallback decay. The rebuild is
// a normal full recompute; nothing about the repair changes the decay model.
const defaultRepairLambda = 0.05
