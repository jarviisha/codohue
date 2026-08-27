package qdrant

import "testing"

func TestCollectionNameIsGenerationAware(t *testing.T) {
	for _, tc := range []struct {
		kind              CollectionKind
		legacy, qualified string
	}{
		{CollectionSubjects, "feed_subjects", "feed_g3_subjects"},
		{CollectionObjects, "feed_objects", "feed_g3_objects"},
		{CollectionSubjectsDense, "feed_subjects_dense", "feed_g3_subjects_dense"},
		{CollectionObjectsDense, "feed_objects_dense", "feed_g3_objects_dense"},
	} {
		if got := CollectionName("feed", 1, tc.kind); got != tc.legacy {
			t.Errorf("legacy %s = %q", tc.kind, got)
		}
		if got := CollectionName("feed", 3, tc.kind); got != tc.qualified {
			t.Errorf("qualified %s = %q", tc.kind, got)
		}
	}
}
