//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/compute"
)

func TestComputeMaintenanceLockCoordinatesAppReset(t *testing.T) {
	repo := compute.NewRepository(testDB)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	releaseNamespace, err := repo.LockNamespace(ctx, "maintenance_lock_e2e")
	if err != nil {
		t.Fatalf("lock namespace: %v", err)
	}
	namespaceHeld := true
	defer func() {
		if namespaceHeld {
			releaseNamespace()
		}
	}()

	type lockResult struct {
		release func()
		err     error
	}
	maintenanceResult := make(chan lockResult, 1)
	go func() {
		release, lockErr := repo.LockAllNamespaces(ctx)
		maintenanceResult <- lockResult{release: release, err: lockErr}
	}()

	select {
	case result := <-maintenanceResult:
		if result.release != nil {
			result.release()
		}
		t.Fatalf("maintenance lock acquired before active namespace drained: %v", result.err)
	case <-time.After(150 * time.Millisecond):
	}

	releaseNamespace()
	namespaceHeld = false

	var releaseMaintenance func()
	select {
	case result := <-maintenanceResult:
		if result.err != nil {
			t.Fatalf("lock all namespaces: %v", result.err)
		}
		releaseMaintenance = result.release
	case <-ctx.Done():
		t.Fatalf("maintenance lock did not acquire after namespace drained: %v", ctx.Err())
	}

	_, ok, err := repo.TryLockNamespace(ctx, "maintenance_lock_e2e")
	if err != nil {
		t.Fatalf("try namespace during maintenance: %v", err)
	}
	if ok {
		t.Fatal("namespace lock acquired while exclusive maintenance lock was held")
	}

	releaseMaintenance()
	release, ok, err := repo.TryLockNamespace(ctx, "maintenance_lock_e2e")
	if err != nil {
		t.Fatalf("try namespace after maintenance: %v", err)
	}
	if !ok {
		t.Fatal("namespace lock remained blocked after maintenance lock release")
	}
	release()
}
