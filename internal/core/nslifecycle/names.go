package nslifecycle

import "fmt"

// PhysicalKind is a namespace-owned Redis or Qdrant resource kind.
type PhysicalKind string

const (
	KindRecommendationCache PhysicalKind = "recommendation_cache"
	KindTrending            PhysicalKind = "trending"
	KindEmbedStream         PhysicalKind = "embed_stream"
	KindSubjects            PhysicalKind = "subjects"
	KindObjects             PhysicalKind = "objects"
	KindSubjectsDense       PhysicalKind = "subjects_dense"
	KindObjectsDense        PhysicalKind = "objects_dense"
)

// PhysicalName preserves all generation-1 names and qualifies generation 2+
// so delayed writers cannot make old artifacts visible to current readers.
func PhysicalName(kind PhysicalKind, namespace string, generation int64) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("physical name: namespace is required")
	}
	if generation < 1 {
		return "", fmt.Errorf("physical name: generation must be positive")
	}
	qualified := namespace
	if generation > 1 {
		qualified = fmt.Sprintf("%s:g%d", namespace, generation)
	}
	switch kind {
	case KindRecommendationCache:
		return "rec:" + qualified, nil
	case KindTrending:
		return "trending:" + qualified, nil
	case KindEmbedStream:
		return "catalog:embed:" + qualified, nil
	case KindSubjects, KindObjects, KindSubjectsDense, KindObjectsDense:
		if generation == 1 {
			return namespace + "_" + string(kind), nil
		}
		return fmt.Sprintf("%s_g%d_%s", namespace, generation, kind), nil
	default:
		return "", fmt.Errorf("physical name: unknown kind %q", kind)
	}
}

// MustPhysicalName returns a validated name for internal call sites whose kind
// and lifecycle have already been checked.
func MustPhysicalName(kind PhysicalKind, namespace string, generation int64) string {
	name, err := PhysicalName(kind, namespace, generation)
	if err != nil {
		panic(err)
	}
	return name
}
