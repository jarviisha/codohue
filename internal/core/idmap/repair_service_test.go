package idmap

import "testing"

func evidenceFor(namespace string, mappings map[string]map[string]int64, points ...CollectionEvidence) *NamespaceEvidence {
	return &NamespaceEvidence{Namespace: namespace, Mappings: mappings, Points: points}
}

// The mapping row is authoritative because it is what every live lookup
// already returns. An identity whose points already sit on that id needs no
// work; one whose points sit elsewhere needs a copy.
func TestAuditNamespace_MappingDecidesTheTarget(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{
			"object": {"already-right": 10, "needs-move": 20},
		},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "already-right"},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 99, StringID: "needs-move"},
	)

	items := auditNamespace(evidence)

	byID := map[string]RepairItem{}
	for _, item := range items {
		byID[item.StringID] = item
	}
	settled := byID["already-right"]
	if !settled.Resolved() || settled.NeedsCopy() {
		t.Errorf("a point already on its mapped id needs no repair: %+v", settled)
	}
	moving := byID["needs-move"]
	if !moving.Resolved() || !moving.NeedsCopy() {
		t.Errorf("a point on the wrong id must be repaired: %+v", moving)
	}
	if *moving.TargetNumericID != 20 {
		t.Errorf("target = %d, want the mapped id 20", *moving.TargetNumericID)
	}
}

// Two points in ONE collection claiming the same identity means the
// authoritative vector cannot be chosen. Picking either would silently discard
// a vector that may not be recomputable, so the audit refuses.
func TestAuditNamespace_AmbiguousPointsAreQuarantined(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {"o1": 10}},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "o1"},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 11, StringID: "o1"},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].State != RepairItemQuarantined {
		t.Errorf("state = %s, want quarantined", items[0].State)
	}
	if items[0].Resolved() {
		t.Error("a quarantined item must not count as resolved")
	}
}

// The same numeric id appearing in two DIFFERENT collections is normal — the
// sparse and dense views of one object share an id — and must not be confused
// with ambiguity.
func TestAuditNamespace_SameIDAcrossCollectionsIsNotAmbiguous(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {"o1": 10}},
		CollectionEvidence{Collection: "ns_objects", EntityType: "object", NumericID: 10, StringID: "o1"},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "o1"},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].State == RepairItemQuarantined {
		t.Fatalf("one object across two collections must resolve cleanly: %+v", items)
	}
}

// A point whose payload carries no logical id cannot be tied to a mapping at
// all. Skipping it would leave it behind silently; the run is blocked instead.
func TestAuditNamespace_UnlabeledPointBlocksTheRun(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 42, StringID: ""},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].State != RepairItemQuarantined {
		t.Fatalf("an unlabeled point must be quarantined: %+v", items)
	}
}

// Points for an identity with no mapping row have no authoritative id to move
// onto — minting one would invent an identity the rest of the system never
// agreed to.
func TestAuditNamespace_PointsWithoutAMappingAreQuarantined(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {}},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 42, StringID: "orphan"},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].State != RepairItemQuarantined {
		t.Fatalf("an unmapped identity must be quarantined: %+v", items)
	}
}

// A mapping with no points is still part of the logical set, so verification
// covers it rather than silently ignoring identities that happen to be
// vectorless.
func TestAuditNamespace_MappingWithoutPointsIsStillAudited(t *testing.T) {
	evidence := evidenceFor("ns", map[string]map[string]int64{"subject": {"u1": 5}})

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].StringID != "u1" {
		t.Fatalf("expected the vectorless mapping in the manifest: %+v", items)
	}
	if items[0].NeedsCopy() {
		t.Error("an identity with no points needs no copy")
	}
}

// Subjects and objects are separate identity spaces: the same string in both
// is two identities, and merging them would fuse a user with a document.
func TestAuditNamespace_EntityTypesAreSeparateIdentities(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{
			"subject": {"shared": 1},
			"object":  {"shared": 2},
		},
		CollectionEvidence{Collection: "ns_subjects", EntityType: "subject", NumericID: 1, StringID: "shared"},
		CollectionEvidence{Collection: "ns_objects", EntityType: "object", NumericID: 2, StringID: "shared"},
	)

	items := auditNamespace(evidence)

	if len(items) != 2 {
		t.Fatalf("expected two distinct identities, got %d", len(items))
	}
	for _, item := range items {
		if item.State == RepairItemQuarantined {
			t.Errorf("cross-entity reuse is legitimate, not ambiguous: %+v", item)
		}
	}
}

// The manifest hash covers the decisions, not just the identities: if a target
// changed between audit and apply, apply must notice.
func TestManifestHash_CoversDecisionsAndIsOrderIndependent(t *testing.T) {
	ten, eleven := int64(10), int64(11)
	base := []RepairItem{
		{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{9}, TargetNumericID: &ten, State: RepairItemPending},
		{Namespace: "ns", EntityType: "subject", StringID: "u1", OldNumericIDs: []int64{1}, TargetNumericID: &eleven, State: RepairItemPending},
	}
	reordered := []RepairItem{base[1], base[0]}

	if ManifestHash(base) != ManifestHash(reordered) {
		t.Error("hash must not depend on item order")
	}

	retargeted := append([]RepairItem(nil), base...)
	other := int64(12)
	retargeted[0].TargetNumericID = &other
	if ManifestHash(base) == ManifestHash(retargeted) {
		t.Error("changing a target must change the hash")
	}

	requarantined := append([]RepairItem(nil), base...)
	requarantined[0].State = RepairItemQuarantined
	if ManifestHash(base) == ManifestHash(requarantined) {
		t.Error("changing an item's state must change the hash")
	}
}

// old_numeric_ids order comes from a map iteration upstream, so the hash sorts
// them — otherwise two identical audits would disagree.
func TestManifestHash_IgnoresOldIDOrdering(t *testing.T) {
	target := int64(10)
	a := []RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{9, 8}, TargetNumericID: &target}}
	b := []RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{8, 9}, TargetNumericID: &target}}

	if ManifestHash(a) != ManifestHash(b) {
		t.Error("old numeric id order must not affect the hash")
	}
}

// NeedsCopy is what decides whether points move at all, so a correct mapping
// must never be rewritten.
func TestRepairItem_NeedsCopy(t *testing.T) {
	target := int64(10)
	for _, tc := range []struct {
		name string
		item RepairItem
		want bool
	}{
		{"already on target", RepairItem{OldNumericIDs: []int64{10}, TargetNumericID: &target}, false},
		{"no points at all", RepairItem{TargetNumericID: &target}, false},
		{"one stale id", RepairItem{OldNumericIDs: []int64{9}, TargetNumericID: &target}, true},
		{"target plus a stale id", RepairItem{OldNumericIDs: []int64{9, 10}, TargetNumericID: &target}, true},
		{"quarantined", RepairItem{OldNumericIDs: []int64{9}, State: RepairItemQuarantined}, false},
		{"unresolved", RepairItem{OldNumericIDs: []int64{9}}, false},
	} {
		if got := tc.item.NeedsCopy(); got != tc.want {
			t.Errorf("%s: NeedsCopy = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestItemCollections_SortedAndTolerantOfMissingSources(t *testing.T) {
	item := RepairItem{Sources: map[string]any{"collections": map[string]any{"b": nil, "a": nil}}}
	got := itemCollections(item)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want sorted [a b]", got)
	}
	if got := itemCollections(RepairItem{}); got != nil {
		t.Errorf("missing sources must yield no collections, got %v", got)
	}
}
