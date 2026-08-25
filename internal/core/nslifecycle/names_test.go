package nslifecycle

import "testing"

func TestPhysicalNamePreservesGenerationOneAndQualifiesLaterGenerations(t *testing.T) {
	tests := []struct {
		kind              PhysicalKind
		legacy, qualified string
	}{
		{KindRecommendationCache, "rec:tenant", "rec:tenant:g2"},
		{KindTrending, "trending:tenant", "trending:tenant:g2"},
		{KindEmbedStream, "catalog:embed:tenant", "catalog:embed:tenant:g2"},
		{KindSubjects, "tenant_subjects", "tenant_g2_subjects"},
		{KindObjects, "tenant_objects", "tenant_g2_objects"},
		{KindSubjectsDense, "tenant_subjects_dense", "tenant_g2_subjects_dense"},
		{KindObjectsDense, "tenant_objects_dense", "tenant_g2_objects_dense"},
	}
	for _, tt := range tests {
		if got, err := PhysicalName(tt.kind, "tenant", 1); err != nil || got != tt.legacy {
			t.Errorf("%s generation 1 = %q, %v; want %q", tt.kind, got, err, tt.legacy)
		}
		if got, err := PhysicalName(tt.kind, "tenant", 2); err != nil || got != tt.qualified {
			t.Errorf("%s generation 2 = %q, %v; want %q", tt.kind, got, err, tt.qualified)
		}
	}
}

func TestPhysicalNameRejectsInvalidInput(t *testing.T) {
	if _, err := PhysicalName(KindObjects, "", 1); err == nil {
		t.Fatal("empty namespace accepted")
	}
	if _, err := PhysicalName(KindObjects, "tenant", 0); err == nil {
		t.Fatal("generation zero accepted")
	}
	if _, err := PhysicalName(PhysicalKind("unknown"), "tenant", 1); err == nil {
		t.Fatal("unknown kind accepted")
	}
}
