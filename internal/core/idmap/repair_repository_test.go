package idmap

import (
	"strings"
	"testing"
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
