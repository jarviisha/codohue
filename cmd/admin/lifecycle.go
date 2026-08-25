package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
)

// lifecycleCoordinatorAdapter composes locking and durable persistence into
// the narrow surface consumed by the admin domain.
type lifecycleCoordinatorAdapter struct {
	service *nslifecycle.Service
	repo    *nslifecycle.Repository
}

func (a *lifecycleCoordinatorAdapter) WithWriter(ctx context.Context, namespace string, fn func(context.Context, *nslifecycle.NamespaceLifecycle) error) error {
	return a.service.WithWriter(ctx, namespace, fn)
}

func (a *lifecycleCoordinatorAdapter) WithNamespaceExclusive(ctx context.Context, namespace string, fn func(context.Context, *nslifecycle.NamespaceLifecycle) error) error {
	return a.service.WithNamespaceExclusive(ctx, namespace, fn)
}

func (a *lifecycleCoordinatorAdapter) WithGlobalExclusive(ctx context.Context, fn func(context.Context, *nslifecycle.SystemLifecycle) error) error {
	return a.service.WithGlobalExclusive(ctx, fn)
}

func (a *lifecycleCoordinatorAdapter) StartDelete(ctx context.Context, namespace string) (*nslifecycle.NamespaceLifecycle, error) {
	return a.repo.StartDelete(ctx, namespace)
}

func (a *lifecycleCoordinatorAdapter) CompleteDelete(ctx context.Context, namespace string, generation int64) error {
	return a.repo.CompleteDelete(ctx, namespace, generation)
}

func (a *lifecycleCoordinatorAdapter) RecordNamespaceError(ctx context.Context, namespace string, generation int64, message string) error {
	return a.repo.RecordNamespaceError(ctx, namespace, generation, message)
}

func (a *lifecycleCoordinatorAdapter) StartReset(ctx context.Context) error {
	return a.repo.StartReset(ctx)
}

func (a *lifecycleCoordinatorAdapter) CompleteReset(ctx context.Context) error {
	return a.repo.CompleteReset(ctx)
}

func (a *lifecycleCoordinatorAdapter) RecordResetError(ctx context.Context, message string) error {
	return a.repo.RecordResetError(ctx, message)
}

func (a *lifecycleCoordinatorAdapter) ListNonDeleted(ctx context.Context) ([]*nslifecycle.NamespaceLifecycle, error) {
	return a.repo.ListNonDeleted(ctx)
}

type legacyEnvelopeDisabler interface {
	DisableLegacyEnvelopes(context.Context, string) (bool, error)
}

func runLifecycleCommand(ctx context.Context, args []string, disabler legacyEnvelopeDisabler) (bool, error) {
	if len(args) == 0 || args[0] != "disable-legacy-envelopes" {
		return false, fmt.Errorf("usage: lifecycle disable-legacy-envelopes --all --adoption-evidence <evidence>")
	}
	flags := flag.NewFlagSet("lifecycle disable-legacy-envelopes", flag.ContinueOnError)
	all := flags.Bool("all", false, "close legacy envelopes for all namespaces")
	evidence := flags.String("adoption-evidence", "", "producer adoption evidence reference")
	if err := flags.Parse(args[1:]); err != nil {
		return false, fmt.Errorf("parse lifecycle flags: %w", err)
	}
	if flags.NArg() != 0 || !*all || *evidence == "" {
		return false, fmt.Errorf("usage: lifecycle disable-legacy-envelopes --all --adoption-evidence <evidence>")
	}
	return disabler.DisableLegacyEnvelopes(ctx, *evidence)
}

func runLifecycleCLI(args []string) error {
	cfg, err := config.LoadAdmin()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx := context.Background()
	db, err := infrapg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer db.Close()
	repo := nslifecycle.NewRepository(db)
	service := nslifecycle.NewService(repo, nslifecycle.NewPostgresLocker(db))
	changed, err := runLifecycleCommand(ctx, args, service)
	if err != nil {
		return err
	}
	slog.Info("legacy envelope gate closed", "changed", changed)
	return nil
}
