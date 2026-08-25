package idmap

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The manifest hash is what apply checks before it touches anything, so it has
// to cover every decision the operator reviewed and nothing that varies for
// unrelated reasons.
func TestManifestHash_CoversTheAuditedIdentitySet(t *testing.T) {
	ten := int64(10)
	base := []RepairItem{
		{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{9}, TargetNumericID: &ten},
	}
	hash := ManifestHash(base)

	for _, tc := range []struct {
		name   string
		mutate func([]RepairItem) []RepairItem
	}{
		{"an added identity", func(items []RepairItem) []RepairItem {
			return append(items, RepairItem{Namespace: "ns", EntityType: "object", StringID: "o2"})
		}},
		{"a different namespace", func(items []RepairItem) []RepairItem {
			out := append([]RepairItem(nil), items...)
			out[0].Namespace = "other"
			return out
		}},
		{"a different entity type", func(items []RepairItem) []RepairItem {
			out := append([]RepairItem(nil), items...)
			out[0].EntityType = "subject"
			return out
		}},
		{"an extra observed id", func(items []RepairItem) []RepairItem {
			out := append([]RepairItem(nil), items...)
			out[0].OldNumericIDs = []int64{9, 11}
			return out
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ManifestHash(tc.mutate(base)) == hash {
				t.Errorf("%s must change the manifest hash", tc.name)
			}
		})
	}
}

// An unresolved target is a distinct decision from any resolved one — the
// operator is being told "this cannot proceed", not "this moves to id 0".
func TestManifestHash_DistinguishesResolvedFromUnresolved(t *testing.T) {
	zero := int64(0)
	unresolved := []RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1"}}
	resolvedToZero := []RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1", TargetNumericID: &zero}}

	if ManifestHash(unresolved) == ManifestHash(resolvedToZero) {
		t.Error("a nil target must not hash the same as a target of 0")
	}
}

// The hash is a fingerprint, not a serialization: two runs of the same audit
// must agree, and an empty manifest must still produce a stable value.
func TestManifestHash_IsDeterministicAndTotal(t *testing.T) {
	if a, b := ManifestHash(nil), ManifestHash(nil); a != b {
		t.Errorf("empty manifest hashed differently: %s vs %s", a, b)
	}
	if got := ManifestHash(nil); len(got) != 64 {
		t.Errorf("hash length = %d, want a 64-char sha256 hex digest", len(got))
	}

	ten := int64(10)
	items := []RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1", TargetNumericID: &ten}}
	first := ManifestHash(items)
	for i := 0; i < 10; i++ {
		if ManifestHash(items) != first {
			t.Fatal("hash is not deterministic across repeated calls")
		}
	}
}

// Field separators matter: without them, ("ab", "c") and ("a", "bc") would
// hash identically and two different identity sets would look like one.
func TestManifestHash_FieldsCannotRunTogether(t *testing.T) {
	a := []RepairItem{{Namespace: "ns", EntityType: "ob", StringID: "ject"}}
	b := []RepairItem{{Namespace: "ns", EntityType: "o", StringID: "bject"}}

	if ManifestHash(a) == ManifestHash(b) {
		t.Error("identity fields collided across the separator")
	}
}

// Resume reads item state to decide what is left, so the state vocabulary has
// to match what the schema's CHECK constraint accepts — a typo here would only
// surface as a constraint violation mid-apply.
func TestRepairItemStates_MatchTheSchemaVocabulary(t *testing.T) {
	// Mirrors id_mapping_repair_items_state_chk in migration 026.
	allowed := map[RepairItemState]bool{
		"pending": true, "copied": true, "verified": true,
		"cleaned": true, "quarantined": true, "failed": true,
	}
	for _, state := range []RepairItemState{
		RepairItemPending, RepairItemCopied, RepairItemVerified,
		RepairItemCleaned, RepairItemQuarantined, RepairItemFailed,
	} {
		if !allowed[state] {
			t.Errorf("item state %q is not accepted by the schema CHECK", state)
		}
	}
}

func TestRepairRunStates_MatchTheSchemaVocabulary(t *testing.T) {
	// Mirrors id_mapping_repair_runs_state_chk in migration 026.
	allowed := map[RepairRunState]bool{
		"audited": true, "snapshotting": true, "applying": true,
		"verifying": true, "complete": true, "failed": true,
	}
	for _, state := range []RepairRunState{
		RepairRunAudited, RepairRunSnapshotting, RepairRunApplying,
		RepairRunVerifying, RepairRunComplete, RepairRunFailed,
	} {
		if !allowed[state] {
			t.Errorf("run state %q is not accepted by the schema CHECK", state)
		}
	}
}

// A quarantined item is never resolved regardless of what else it carries:
// apply keys off this to refuse, so a stray target must not make it look ready.
func TestRepairItem_QuarantinedIsNeverResolved(t *testing.T) {
	target := int64(10)
	item := RepairItem{State: RepairItemQuarantined, TargetNumericID: &target}

	if item.Resolved() {
		t.Error("a quarantined item with a target must still read as unresolved")
	}
	if item.NeedsCopy() {
		t.Error("a quarantined item must never be copied")
	}
}

// TargetConflictWith reads free-form JSONB, so it must tolerate a manifest
// written before the key existed and a value of the wrong type.
func TestRepairItem_TargetConflictWithToleratesLooseSources(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources map[string]any
		want    string
	}{
		{"nil sources", nil, ""},
		{"pre-conflict manifest", map[string]any{"collections": map[string]any{}}, ""},
		{"wrong type", map[string]any{"target_conflict_with": 42}, ""},
		{"present", map[string]any{"target_conflict_with": "squatter"}, "squatter"},
	} {
		if got := (RepairItem{Sources: tc.sources}).TargetConflictWith(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The repair errors are what the CLI branches on, so each has to be
// distinguishable and say what an operator should do next.
func TestRepairErrors_AreDistinctAndActionable(t *testing.T) {
	all := []error{
		ErrRepairRunNotFound, ErrRepairRunActive, ErrManifestChanged,
		ErrQuarantinedItems, ErrSnapshotsRequired,
	}
	seen := map[string]bool{}
	for _, err := range all {
		message := err.Error()
		if seen[message] {
			t.Errorf("duplicate error message %q", message)
		}
		seen[message] = true
		if !strings.HasPrefix(message, "idmap: ") {
			t.Errorf("error %q is not namespaced to this package", message)
		}
	}
}

// ─── PostgreSQL-backed lifecycle ─────────────────────────────────────────────
//
// These exercise the repair repository's SQL. Nothing else does: the pure
// tests above cover hashing and the state vocabulary, and the service tests
// drive a fake store. The statements here — a two-table transactional insert,
// a partial unique index, and a jsonb append-and-dedup — are the kind that
// only a real PostgreSQL validates.

func openRepairTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// newRepairRun opens a run and guarantees it is closed out, so the partial
// unique index on in-flight runs does not leak into the next test.
func newRepairRun(t *testing.T, repo *RepairRepository, items []RepairItem) *RepairRun {
	t.Helper()
	ctx := context.Background()
	run, err := repo.CreateRun(ctx, items, time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	t.Cleanup(func() {
		// Terminal state first so the active-run index is free even if the
		// delete is blocked; the cascade removes the items.
		_ = repo.SetRunState(context.Background(), run.ID, RepairRunComplete, "")
		_, _ = repo.db.Exec(context.Background(), `DELETE FROM id_mapping_repair_runs WHERE id = $1`, run.ID)
	})
	return run
}

func repairTestItems(namespace string) []RepairItem {
	target := int64(10)
	return []RepairItem{
		{
			Namespace: namespace, EntityType: "object", StringID: "o1",
			OldNumericIDs: []int64{20}, TargetNumericID: &target,
			PayloadHash: "phash", VectorHash: "vhash", State: RepairItemPending,
			Sources: map[string]any{"collections": map[string]any{namespace + "_objects_dense": nil}},
		},
		{
			Namespace: namespace, EntityType: "subject", StringID: "u1",
			State: RepairItemQuarantined, Error: "no id_mappings row",
		},
	}
}

// The run row and its manifest land in one transaction: a run whose items
// went missing (or vice versa) could never be resumed.
func TestRepairRepository_CreateRunPersistsTheWholeManifest(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))
	ctx := context.Background()
	namespace := "idmap_repair_create"
	items := repairTestItems(namespace)

	run := newRepairRun(t, repo, items)

	if run.State != RepairRunAudited {
		t.Errorf("state = %q, want audited", run.State)
	}
	if run.ManifestHash != ManifestHash(items) {
		t.Errorf("stored hash %q does not match the audited manifest", run.ManifestHash)
	}

	stored, err := repo.ListItems(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored %d item(s), want 2", len(stored))
	}
	// Ordered by (namespace, entity_type, string_id), so a resumed run walks
	// the same sequence the audit reviewed.
	if stored[0].EntityType != "object" || stored[1].EntityType != "subject" {
		t.Errorf("items came back in %s/%s order", stored[0].EntityType, stored[1].EntityType)
	}
	object := stored[0]
	if object.PayloadHash != "phash" || object.VectorHash != "vhash" {
		t.Errorf("preservation hashes lost: payload=%q vector=%q", object.PayloadHash, object.VectorHash)
	}
	if len(object.OldNumericIDs) != 1 || object.OldNumericIDs[0] != 20 {
		t.Errorf("old numeric ids = %v", object.OldNumericIDs)
	}
	if object.TargetNumericID == nil || *object.TargetNumericID != 10 {
		t.Errorf("target = %v", object.TargetNumericID)
	}
	if _, ok := object.Sources["collections"]; !ok {
		t.Errorf("sources lost the collection evidence: %v", object.Sources)
	}
	// A quarantined item carries its reason, which is what the operator acts on.
	if stored[1].State != RepairItemQuarantined || stored[1].Error == "" {
		t.Errorf("quarantine reason lost: %+v", stored[1])
	}
}

// Two concurrent reconciliations would each believe they own the numeric-id
// space, so the schema allows only one in-flight run.
func TestRepairRepository_OnlyOneRunMayBeInFlight(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))
	ctx := context.Background()
	namespace := "idmap_repair_active"
	newRepairRun(t, repo, repairTestItems(namespace))

	_, err := repo.CreateRun(ctx, repairTestItems(namespace), time.Now().UTC())
	if !errors.Is(err, ErrRepairRunActive) {
		t.Fatalf("second run: got %v, want ErrRepairRunActive", err)
	}

	active, err := repo.ActiveRun(ctx)
	if err != nil {
		t.Fatalf("ActiveRun: %v", err)
	}
	if active == nil {
		t.Fatal("the in-flight run was not reported as active")
	}
}

// A terminal run frees the slot — this is what lets a re-audit supersede a run
// that mutated nothing.
func TestRepairRepository_TerminalRunReleasesTheSlot(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))
	ctx := context.Background()
	run := newRepairRun(t, repo, repairTestItems("idmap_repair_terminal"))

	if err := repo.SetRunState(ctx, run.ID, RepairRunFailed, "superseded by a later audit"); err != nil {
		t.Fatalf("SetRunState: %v", err)
	}

	active, err := repo.ActiveRun(ctx)
	if err != nil {
		t.Fatalf("ActiveRun: %v", err)
	}
	if active != nil {
		t.Fatalf("a failed run still holds the slot: %+v", active)
	}

	reloaded, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if reloaded.Error == "" || reloaded.CompletedAt == nil {
		t.Errorf("terminal run lost its diagnostic or completion time: %+v", reloaded)
	}
}

func TestRepairRepository_GetRunReportsAnUnknownID(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))

	if _, err := repo.GetRun(context.Background(), -1); !errors.Is(err, ErrRepairRunNotFound) {
		t.Fatalf("got %v, want ErrRepairRunNotFound", err)
	}
}

// Both stores are recorded together: a recovery that restores only one of them
// reintroduces exactly the cross-store divergence the repair is fixing.
func TestRepairRepository_RecordSnapshotsRoundTrips(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))
	ctx := context.Background()
	run := newRepairRun(t, repo, repairTestItems("idmap_repair_snapshots"))

	refs := map[string]string{"ns_objects": "snap-a", "ns_objects_dense": "snap-b"}
	if err := repo.RecordSnapshots(ctx, run.ID, "base-1", refs); err != nil {
		t.Fatalf("RecordSnapshots: %v", err)
	}

	reloaded, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if reloaded.PGSnapshotRef != "base-1" {
		t.Errorf("pg snapshot = %q", reloaded.PGSnapshotRef)
	}
	if len(reloaded.QdrantSnapshotRefs) != 2 || reloaded.QdrantSnapshotRefs["ns_objects_dense"] != "snap-b" {
		t.Errorf("qdrant snapshots = %v", reloaded.QdrantSnapshotRefs)
	}
}

// The rebuild record is appended per namespace as each one completes, so an
// interrupted apply leaves an accurate partial record rather than an
// all-or-nothing claim. Re-recording the same namespace must not duplicate it.
func TestRepairRepository_RecordRebuiltNamespaceAppendsAndDeduplicates(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))
	ctx := context.Background()
	run := newRepairRun(t, repo, repairTestItems("idmap_repair_rebuilt"))

	fresh, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if len(fresh.RebuiltNamespaces) != 0 {
		t.Fatalf("a new run already claims rebuilds: %v", fresh.RebuiltNamespaces)
	}

	for _, namespace := range []string{"alpha", "beta", "alpha"} {
		if err := repo.RecordRebuiltNamespace(ctx, run.ID, namespace); err != nil {
			t.Fatalf("RecordRebuiltNamespace(%q): %v", namespace, err)
		}
	}

	reloaded, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if len(reloaded.RebuiltNamespaces) != 2 {
		t.Fatalf("rebuilt = %v, want alpha and beta recorded once each", reloaded.RebuiltNamespaces)
	}
	seen := map[string]bool{}
	for _, namespace := range reloaded.RebuiltNamespaces {
		seen[namespace] = true
	}
	if !seen["alpha"] || !seen["beta"] {
		t.Errorf("rebuilt = %v", reloaded.RebuiltNamespaces)
	}
}

// Per-item state is what makes a crashed apply resumable mid-manifest instead
// of restarting a partially applied run.
func TestRepairRepository_ItemStateTransitionsArePersisted(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))
	ctx := context.Background()
	namespace := "idmap_repair_states"
	run := newRepairRun(t, repo, repairTestItems(namespace))

	items, err := repo.ListItems(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	object := items[0]

	for _, state := range []RepairItemState{RepairItemCopied, RepairItemCleaned, RepairItemVerified} {
		if err := repo.SetItemState(ctx, object, state, ""); err != nil {
			t.Fatalf("SetItemState(%q): %v", state, err)
		}
		reloaded, err := repo.ListItemsInState(ctx, run.ID, state)
		if err != nil {
			t.Fatalf("ListItemsInState(%q): %v", state, err)
		}
		if len(reloaded) != 1 || reloaded[0].StringID != "o1" {
			t.Fatalf("state %q: got %d item(s)", state, len(reloaded))
		}
	}

	// The quarantine report is the same query, and it is what blocks apply.
	quarantined, err := repo.ListItemsInState(ctx, run.ID, RepairItemQuarantined)
	if err != nil {
		t.Fatalf("ListItemsInState(quarantined): %v", err)
	}
	if len(quarantined) != 1 || quarantined[0].Error == "" {
		t.Fatalf("quarantine report = %+v", quarantined)
	}
}

// seedNamespace creates the rows a namespace-scoped mapping depends on.
//
// The chain is not obvious and biting it is a runtime FK error, not a compile
// one: `id_mappings.namespace` references `namespace_configs` (migration 025),
// and `namespace_configs (namespace, generation)` references
// `namespace_lifecycles` (migration 024). A test that inserts a mapping for an
// invented namespace fails on the first of those.
//
// `id_mapping_repair_items.namespace` deliberately has no such key — the
// manifest is an audit record and must outlive the namespace it describes — so
// the repair-run tests above do not need this.
func seedNamespace(t *testing.T, db *pgxpool.Pool, namespace string) {
	t.Helper()
	ctx := context.Background()

	if _, err := db.Exec(ctx, `
		INSERT INTO namespace_lifecycles (namespace, generation, state, activated_at)
		VALUES ($1, 1, 'active', NOW())
		ON CONFLICT (namespace) DO NOTHING`, namespace); err != nil {
		t.Fatalf("seed namespace lifecycle: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO namespace_configs (namespace) VALUES ($1)
		ON CONFLICT (namespace) DO NOTHING`, namespace); err != nil {
		t.Fatalf("seed namespace config: %v", err)
	}

	t.Cleanup(func() {
		clean := context.Background()
		// namespace_configs first: the mapping FK cascades from it, and the
		// lifecycle row is what it points at.
		_, _ = db.Exec(clean, `DELETE FROM namespace_configs WHERE namespace = $1`, namespace)
		_, _ = db.Exec(clean, `DELETE FROM namespace_lifecycles WHERE namespace = $1`, namespace)
	})
}

// Retarget moves an EXISTING mapping; minting a parallel one would leave the
// identity resolvable two ways.
func TestRepairRepository_RetargetMappingMovesAnExistingRow(t *testing.T) {
	db := openRepairTestDB(t)
	repo := NewRepairRepository(db)
	ctx := context.Background()
	namespace := "idmap_repair_retarget"

	seedNamespace(t, db, namespace)
	if _, err := db.Exec(ctx, `
		INSERT INTO id_mappings (string_id, namespace, entity_type)
		VALUES ('o1', $1, 'object')`, namespace); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	fresh, err := repo.NextNumericID(ctx)
	if err != nil {
		t.Fatalf("NextNumericID: %v", err)
	}
	if fresh <= 0 {
		t.Fatalf("reserved id %d is not positive", fresh)
	}

	if err := repo.RetargetMapping(ctx, namespace, "object", "o1", fresh); err != nil {
		t.Fatalf("RetargetMapping: %v", err)
	}

	mappings, err := repo.LoadMappings(ctx, namespace)
	if err != nil {
		t.Fatalf("LoadMappings: %v", err)
	}
	if got := mappings["object"]["o1"]; got != fresh {
		t.Errorf("mapping = %d, want the retargeted %d", got, fresh)
	}
}

// Retargeting an identity that has no mapping is a bug in the manifest, not a
// silent no-op — it would leave the copied point unreachable.
func TestRepairRepository_RetargetMappingRefusesAnUnknownIdentity(t *testing.T) {
	repo := NewRepairRepository(openRepairTestDB(t))

	err := repo.RetargetMapping(context.Background(), "idmap_repair_absent", "object", "nope", 99)
	if err == nil {
		t.Fatal("retargeting a mapping that does not exist must fail loudly")
	}
}
