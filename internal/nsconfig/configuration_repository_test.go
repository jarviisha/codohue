package nsconfig

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
)

func TestConfigurationRepositoryConcurrency(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	ns := "configuration_revision_test"
	cleanupNS(t, db, ns)
	repo := NewRepository(db)
	svc := NewService(repo)
	if _, err := repo.Upsert(ctx, ns, &UpsertRequest{}); err != nil {
		t.Fatal(err)
	}
	baseline, err := svc.ReadConfiguration(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}
	patch := func(group, field, value string) *namespace.ConfigurationPatch {
		return &namespace.ConfigurationPatch{Group: group, Generation: baseline.Generation, BaseRevision: baseline.Groups[group].Revision, Changes: map[string]json.RawMessage{field: json.RawMessage(value)}}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, p := range []*namespace.ConfigurationPatch{patch("trending", "trending_ttl", "4321"), patch("recommendations", "max_results", "123")} {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := svc.PatchConfiguration(ctx, ns, p, false); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	_, err = svc.PatchConfiguration(ctx, ns, patch("trending", "trending_ttl", "5678"), false)
	var conflict *namespace.ConfigurationError
	if !errors.As(err, &conflict) || conflict.Status != 409 {
		t.Fatalf("stale write=%v", err)
	}
	latest, _ := svc.ReadConfiguration(ctx, ns)
	if latest.Groups["signals"].Revision != baseline.Groups["signals"].Revision {
		t.Fatal("unrelated revision changed")
	}
	baseline = latest
	noOp, err := svc.PatchConfiguration(ctx, ns, patch("trending", "trending_ttl", "4321"), false)
	if err != nil || noOp.Groups["trending"].Revision != baseline.Groups["trending"].Revision {
		t.Fatalf("no-op: %v", err)
	}
	validated, err := svc.PatchConfiguration(ctx, ns, patch("trending", "trending_ttl", "6789"), true)
	if err != nil || string(validated.Groups["trending"].Values["trending_ttl"]) != "4321" {
		t.Fatalf("validation mutated: %v", err)
	}
	if _, err = repo.Upsert(ctx, ns, &UpsertRequest{TrendingTTL: ptr(9999)}); err != nil {
		t.Fatal(err)
	}
	legacy, _ := svc.ReadConfiguration(ctx, ns)
	if legacy.Groups["trending"].Revision != baseline.Groups["trending"].Revision+1 {
		t.Fatal("legacy write did not invalidate revision")
	}
	baseline = legacy
	_, err = svc.PatchConfiguration(ctx, ns, patch("embeddings", "catalog_max_attempts", "7"), false)
	if err != nil {
		t.Fatal(err)
	}
	baseline, _ = svc.ReadConfiguration(ctx, ns)
	reset, err := svc.PatchConfiguration(ctx, ns, patch("embeddings", "catalog_max_attempts", "null"), false)
	if err != nil || string(reset.Groups["embeddings"].Values["catalog_max_attempts"]) != "null" {
		t.Fatalf("reset: %v", err)
	}
	baseline = reset
	bad := patch("embeddings", "embedding_dim", "-1")
	_, err = svc.PatchConfiguration(ctx, ns, bad, false)
	if err == nil {
		t.Fatal("invalid dimension accepted")
	}
	after, _ := svc.ReadConfiguration(ctx, ns)
	if after.Groups["embeddings"].Revision != reset.Groups["embeddings"].Revision {
		t.Fatal("invalid write persisted")
	}
	wrong := patch("signals", "lambda", "0.3")
	wrong.Generation++
	_, err = svc.PatchConfiguration(ctx, ns, wrong, false)
	if !errors.As(err, &conflict) || conflict.Status != 409 {
		t.Fatalf("generation=%v", err)
	}
}

func TestConfigurationSameGroupAndRecreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	ns := "configuration_generation_test"
	cleanupNS(t, db, ns)
	repo := NewRepository(db)
	svc := NewService(repo)
	if _, err := repo.Upsert(ctx, ns, &UpsertRequest{}); err != nil {
		t.Fatal(err)
	}
	initial, err := svc.ReadConfiguration(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}
	makePatch := func(value string) *namespace.ConfigurationPatch {
		return &namespace.ConfigurationPatch{Group: "trending", Generation: initial.Generation, BaseRevision: initial.Groups["trending"].Revision, Changes: map[string]json.RawMessage{"trending_ttl": json.RawMessage(value)}}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, value := range []string{"4321", "8765"} {
		go func() { <-start; _, err := svc.PatchConfiguration(ctx, ns, makePatch(value), false); results <- err }()
	}
	close(start)
	success, conflicts := 0, 0
	for range 2 {
		err := <-results
		var conflict *namespace.ConfigurationError
		switch {
		case err == nil:
			success++
		case errors.As(err, &conflict) && conflict.Status == 409:
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
	latest, _ := svc.ReadConfiguration(ctx, ns)
	if _, err := repo.ReplaceAPIKeyHash(ctx, ns, "test-hash"); err != nil {
		t.Fatal(err)
	}
	rotated, _ := svc.ReadConfiguration(ctx, ns)
	for group, g := range latest.Groups {
		if g.Revision != rotated.Groups[group].Revision {
			t.Fatal("key rotation advanced configuration revision")
		}
	}
	if _, err := db.Exec(ctx, `DELETE FROM namespace_configs WHERE namespace=$1`, ns); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE namespace_lifecycles SET state='deleted' WHERE namespace=$1`, ns); err != nil {
		t.Fatal(err)
	}
	_, err = svc.PatchConfiguration(ctx, ns, makePatch("1111"), false)
	var unavailable *namespace.ConfigurationError
	if !errors.As(err, &unavailable) || unavailable.Status != 404 {
		t.Fatalf("deleted patch=%v", err)
	}
	if _, err := repo.Upsert(ctx, ns, &UpsertRequest{}); err != nil {
		t.Fatal(err)
	}
	recreated, _ := svc.ReadConfiguration(ctx, ns)
	if recreated.Generation <= initial.Generation {
		t.Fatal("generation did not advance")
	}
	old := makePatch("1111")
	old.BaseRevision = recreated.Groups["trending"].Revision
	_, err = svc.PatchConfiguration(ctx, ns, old, false)
	if !errors.As(err, &unavailable) || unavailable.Status != 409 {
		t.Fatalf("old generation patch=%v", err)
	}
}

func TestConfigurationCatalogTransitionAtomic(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	ns := "configuration_catalog_test"
	cleanupNS(t, db, ns)
	repo := NewRepository(db)
	svc := NewService(repo)
	svc.registry = embedstrategy.NewRegistry()
	svc.registry.Register("test", "v1", func(embedstrategy.Params) (embedstrategy.Strategy, error) {
		return &stubStrategyT{id: "test", version: "v1", dim: 128}, nil
	})
	if _, err := repo.Upsert(ctx, ns, &UpsertRequest{Alpha: ptr(0.7)}); err != nil {
		t.Fatal(err)
	}
	initial, err := svc.ReadConfiguration(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}
	req := &namespace.ConfigurationPatch{Group: "embeddings", Generation: initial.Generation, BaseRevision: initial.Groups["embeddings"].Revision, Changes: map[string]json.RawMessage{"dense_source": json.RawMessage(`"catalog"`), "embedding_dim": json.RawMessage(`64`), "catalog_strategy_id": json.RawMessage(`"test"`), "catalog_strategy_version": json.RawMessage(`"v1"`)}}
	if _, err = svc.PatchConfiguration(ctx, ns, req, false); err == nil {
		t.Fatal("incompatible strategy accepted")
	}
	unchanged, _ := svc.ReadConfiguration(ctx, ns)
	if unchanged.Groups["embeddings"].Revision != initial.Groups["embeddings"].Revision || string(unchanged.Groups["embeddings"].Values["dense_source"]) != `"disabled"` {
		t.Fatal("partial catalog transition persisted")
	}
	req.Changes["embedding_dim"] = json.RawMessage(`128`)
	saved, err := svc.PatchConfiguration(ctx, ns, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved.Groups["recommendations"].Values["alpha"]) != "0.7" || saved.Groups["recommendations"].Revision != initial.Groups["recommendations"].Revision {
		t.Fatal("source transition rewrote recommendation settings")
	}
	// A different group with its original revision validates against the latest
	// catalog source/strategy/dimension instead of reconstructing an old snapshot.
	_, err = svc.PatchConfiguration(ctx, ns, &namespace.ConfigurationPatch{Group: "recommendations", Generation: initial.Generation, BaseRevision: initial.Groups["recommendations"].Revision, Changes: map[string]json.RawMessage{"alpha": json.RawMessage(`0.8`)}}, false)
	if err != nil {
		t.Fatal(err)
	}
	req.BaseRevision = saved.Groups["embeddings"].Revision
	req.Changes = map[string]json.RawMessage{"dense_source": json.RawMessage(`"byoe"`)}
	left, err := svc.PatchConfiguration(ctx, ns, req, false)
	if err != nil || string(left.Groups["embeddings"].Values["dense_source"]) != `"byoe"` {
		t.Fatalf("leave catalog: %v", err)
	}
}
