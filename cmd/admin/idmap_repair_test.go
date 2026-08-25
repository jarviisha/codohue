package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jarviisha/codohue/internal/core/idmap"
)

type fakeRepairRunner struct {
	auditReport  *idmap.AuditReport
	auditErr     error
	verifyReport *idmap.VerifyReport
	verifyErr    error
	applyErr     error
	resumeErr    error
	snapshotErr  error

	snapshotCalls int
	applyCalls    int
	resumeCalls   int
	lastPGRef     string
	lastQdrantRef map[string]string
}

func (f *fakeRepairRunner) Audit(context.Context) (*idmap.AuditReport, error) {
	if f.auditErr != nil {
		return nil, f.auditErr
	}
	if f.auditReport == nil {
		return &idmap.AuditReport{RunID: 1}, nil
	}
	return f.auditReport, nil
}

func (f *fakeRepairRunner) PrepareSnapshots(_ context.Context, _ int64, pgRef string, qdrantRefs map[string]string) error {
	f.snapshotCalls++
	f.lastPGRef = pgRef
	f.lastQdrantRef = qdrantRefs
	return f.snapshotErr
}

func (f *fakeRepairRunner) Apply(context.Context, int64) error {
	f.applyCalls++
	return f.applyErr
}

func (f *fakeRepairRunner) Verify(context.Context, int64) (*idmap.VerifyReport, error) {
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	if f.verifyReport == nil {
		return &idmap.VerifyReport{RunID: 1}, nil
	}
	return f.verifyReport, nil
}

func (f *fakeRepairRunner) Resume(context.Context, int64) error {
	f.resumeCalls++
	return f.resumeErr
}

func (f *fakeRepairRunner) QuarantineReport(context.Context, int64) ([]idmap.RepairItem, error) {
	return nil, nil
}

func runRepair(t *testing.T, runner repairRunner, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	err := runIdmapRepairCommand(context.Background(), args, runner, &out)
	return out.String(), err
}

func TestIdmapRepair_UnknownAndEmptySubcommands(t *testing.T) {
	for _, args := range [][]string{nil, {"nonsense"}} {
		if _, err := runRepair(t, &fakeRepairRunner{}, args...); err == nil {
			t.Errorf("args %v: expected a usage error", args)
		}
	}
}

// Audit is read-only, so it never touches apply or snapshots — the operator
// runs it to decide whether a repair is warranted at all.
func TestIdmapRepair_AuditMutatesNothing(t *testing.T) {
	runner := &fakeRepairRunner{auditReport: &idmap.AuditReport{
		RunID: 7, ManifestHash: "abc123", Total: 10, Resolved: 9, NeedsRepair: 3,
	}}

	out, err := runRepair(t, runner, "audit")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if runner.applyCalls != 0 || runner.snapshotCalls != 0 {
		t.Error("audit must not apply or record snapshots")
	}
	for _, want := range []string{"run 7", "10 identities", "9 resolved", "3 need repair", "abc123"} {
		if !strings.Contains(out, want) {
			t.Errorf("audit output missing %q:\n%s", want, out)
		}
	}
}

// Every quarantined item is printed, not a sample: each one blocks apply, so
// showing a subset just means another audit round.
func TestIdmapRepair_AuditListsEveryQuarantinedItem(t *testing.T) {
	runner := &fakeRepairRunner{auditReport: &idmap.AuditReport{
		RunID: 7, Total: 2,
		Quarantined: []idmap.RepairItem{
			{Namespace: "ns", EntityType: "object", StringID: "o1", Error: "two points claim this identity"},
			{Namespace: "ns", EntityType: "subject", StringID: "u9", Error: "no id_mappings row"},
		},
	}}

	out, err := runRepair(t, runner, "audit")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	for _, want := range []string{"ns/object/o1", "two points claim this identity", "ns/subject/u9", "no id_mappings row"} {
		if !strings.Contains(out, want) {
			t.Errorf("quarantine report missing %q:\n%s", want, out)
		}
	}
}

// Apply deletes points that may not be recomputable, so a run with no recorded
// recovery point must not start. The snapshot flags are required arguments,
// not hygiene.
func TestIdmapRepair_ApplyRequiresRunAndSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no arguments", []string{"apply"}},
		{"no run id", []string{"apply", "--pg-snapshot", "base-1", "--qdrant-snapshot", "ns_objects=snap-1"}},
		{"no postgres snapshot", []string{"apply", "--run", "7", "--qdrant-snapshot", "ns_objects=snap-1"}},
		{"no qdrant snapshot", []string{"apply", "--run", "7", "--pg-snapshot", "base-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRepairRunner{}
			if _, err := runRepair(t, runner, tc.args...); err == nil {
				t.Fatal("expected a validation error")
			}
			if runner.applyCalls != 0 || runner.snapshotCalls != 0 {
				t.Error("an invalid invocation must not reach the service")
			}
		})
	}
}

func TestIdmapRepair_ApplyRecordsSnapshotsBeforeMutating(t *testing.T) {
	runner := &fakeRepairRunner{}

	out, err := runRepair(t, runner, "apply",
		"--run", "7", "--pg-snapshot", "base-2026-08-25",
		"--qdrant-snapshot", "ns_objects=snap-a", "--qdrant-snapshot", "ns_objects_dense=snap-b")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if runner.snapshotCalls != 1 || runner.applyCalls != 1 {
		t.Fatalf("snapshots=%d applies=%d, want 1 each", runner.snapshotCalls, runner.applyCalls)
	}
	if runner.lastPGRef != "base-2026-08-25" {
		t.Errorf("pg snapshot = %q", runner.lastPGRef)
	}
	if runner.lastQdrantRef["ns_objects"] != "snap-a" || runner.lastQdrantRef["ns_objects_dense"] != "snap-b" {
		t.Errorf("qdrant snapshots = %v", runner.lastQdrantRef)
	}
	if !strings.Contains(out, "verify before unlocking") {
		t.Errorf("apply must tell the operator verification is still required:\n%s", out)
	}
}

// A failure to record snapshots stops before apply: the whole point of the
// flags is that the mutation never runs without them.
func TestIdmapRepair_SnapshotFailureBlocksApply(t *testing.T) {
	runner := &fakeRepairRunner{snapshotErr: errors.New("backup catalog unreachable")}

	if _, err := runRepair(t, runner, "apply",
		"--run", "7", "--pg-snapshot", "base-1", "--qdrant-snapshot", "ns_objects=snap-a"); err == nil {
		t.Fatal("expected the snapshot failure to surface")
	}
	if runner.applyCalls != 0 {
		t.Error("apply ran despite unrecorded snapshots")
	}
}

func TestIdmapRepair_SnapshotFlagShape(t *testing.T) {
	for _, bad := range []string{"no-equals-sign", "=missing-collection", "collection="} {
		runner := &fakeRepairRunner{}
		if _, err := runRepair(t, runner, "apply", "--run", "7", "--pg-snapshot", "base-1", "--qdrant-snapshot", bad); err == nil {
			t.Errorf("--qdrant-snapshot %q: expected a parse error", bad)
		}
	}
}

// Verify is the gate before the fleet is unlocked: an unfinished run has to
// exit non-zero, or an operator scripting the sequence would unlock on a
// half-applied repair.
func TestIdmapRepair_VerifyFailsWhenWorkRemains(t *testing.T) {
	runner := &fakeRepairRunner{verifyReport: &idmap.VerifyReport{
		RunID: 7, Checked: 5,
		Remaining: []idmap.RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1", State: idmap.RepairItemPending}},
		Unmoved:   []idmap.RepairItem{{Namespace: "ns", EntityType: "subject", StringID: "u1"}},
	}}

	out, err := runRepair(t, runner, "verify", "--run", "7")
	if err == nil {
		t.Fatal("an unfinished run must not verify")
	}
	for _, want := range []string{"unfinished ns/object/o1", "old point still present for ns/subject/u1"} {
		if !strings.Contains(out, want) {
			t.Errorf("verify output missing %q:\n%s", want, out)
		}
	}
}

func TestIdmapRepair_VerifySucceedsOnACleanRun(t *testing.T) {
	runner := &fakeRepairRunner{verifyReport: &idmap.VerifyReport{RunID: 7, Checked: 5}}

	out, err := runRepair(t, runner, "verify", "--run", "7")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(out, "complete") {
		t.Errorf("verify output missing the completion line:\n%s", out)
	}
}

func TestIdmapRepair_VerifyAndResumeRequireARunID(t *testing.T) {
	for _, subcommand := range []string{"verify", "resume"} {
		runner := &fakeRepairRunner{}
		if _, err := runRepair(t, runner, subcommand); err == nil {
			t.Errorf("%s without --run: expected a validation error", subcommand)
		}
		if runner.resumeCalls != 0 {
			t.Errorf("%s reached the service without a run id", subcommand)
		}
	}
}

func TestIdmapRepair_ResumeContinuesTheRun(t *testing.T) {
	runner := &fakeRepairRunner{}

	out, err := runRepair(t, runner, "resume", "--run", "7")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if runner.resumeCalls != 1 {
		t.Errorf("resume calls = %d, want 1", runner.resumeCalls)
	}
	if !strings.Contains(out, "verify before unlocking") {
		t.Errorf("resume must still require verification:\n%s", out)
	}
}

// The subcommand has to be reachable from the binary, not just callable in a
// test — that is what makes it an operator-facing tool.
func TestDispatchAdminCommand_RegistersIdmapRepair(t *testing.T) {
	serverCalls, lifecycleCalls, repairCalls := 0, 0, 0
	err := dispatchAdminCommand([]string{"idmap-repair", "audit"},
		func() error { serverCalls++; return nil },
		func([]string) error { lifecycleCalls++; return nil },
		func(args []string) error {
			repairCalls++
			if len(args) != 1 || args[0] != "audit" {
				t.Fatalf("forwarded args = %v", args)
			}
			return nil
		})
	if err != nil || serverCalls != 0 || lifecycleCalls != 0 || repairCalls != 1 {
		t.Fatalf("err=%v server=%d lifecycle=%d repair=%d", err, serverCalls, lifecycleCalls, repairCalls)
	}
}

// affectedCollections drives which snapshots apply demands, so it must reflect
// every collection the manifest touches and list each one once.
func TestAffectedCollections(t *testing.T) {
	items := []idmap.RepairItem{
		{Sources: map[string]any{"collections": map[string]any{"ns_objects": nil, "ns_objects_dense": nil}}},
		{Sources: map[string]any{"collections": map[string]any{"ns_objects": nil}}},
		{Sources: map[string]any{}},
		{},
	}

	got := affectedCollections(items)

	if len(got) != 2 {
		t.Fatalf("got %v, want two distinct collections", got)
	}
	seen := map[string]bool{}
	for _, collection := range got {
		seen[collection] = true
	}
	if !seen["ns_objects"] || !seen["ns_objects_dense"] {
		t.Errorf("got %v", got)
	}
}
