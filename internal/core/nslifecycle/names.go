package nslifecycle

import (
	"encoding/base64"
	"fmt"
)

// PhysicalKind is a namespace-owned Redis or Qdrant resource kind.
type PhysicalKind string

// Namespace-owned physical resources whose names carry the generation.
const (
	KindRecommendationCache PhysicalKind = "recommendation_cache"
	KindTrending            PhysicalKind = "trending"
	KindEmbedStream         PhysicalKind = "embed_stream"
	KindSubjects            PhysicalKind = "subjects"
	KindObjects             PhysicalKind = "objects"
	KindSubjectsDense       PhysicalKind = "subjects_dense"
	KindObjectsDense        PhysicalKind = "objects_dense"
)

// RedisNamespace is the namespace token inside colon-separated Redis keys.
// Generation 1 keeps the bare name so existing keys stay addressable.
//
// Exported because callers that build a key from parts (the recommendation
// cache, whose key also encodes subject and paging) must qualify it exactly
// the way PhysicalName does — a second copy of the rule is a second thing to
// drift.
func RedisNamespace(namespace string, generation int64) string {
	if generation < 2 {
		return namespace
	}
	return fmt.Sprintf("%s:g%d", namespace, generation)
}

// QdrantNamespace is the namespace token inside underscore-separated Qdrant
// collection names. Same contract as RedisNamespace, different separator
// because Qdrant collection names cannot contain a colon.
func QdrantNamespace(namespace string, generation int64) string {
	if generation < 2 {
		return namespace
	}
	return fmt.Sprintf("%s_g%d", namespace, generation)
}

// PhysicalName preserves all generation-1 names and qualifies generation 2+
// so delayed writers cannot make old artifacts visible to current readers.
func PhysicalName(kind PhysicalKind, namespace string, generation int64) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("physical name: namespace is required")
	}
	if generation < 1 {
		return "", fmt.Errorf("physical name: generation must be positive")
	}
	qualified := RedisNamespace(namespace, generation)
	switch kind {
	case KindRecommendationCache:
		// The namespace is base64'd because the rest of the key encodes a
		// caller-supplied subject id the same way, and a raw ':' in either
		// would make the key ambiguous. `v2` is the key-shape version: the
		// serving path writes these keys and namespace deletion scans for
		// them, so the shape has to be defined exactly once.
		return "rec:v2:" + base64.RawURLEncoding.EncodeToString([]byte(qualified)), nil
	case KindTrending:
		return "trending:" + qualified, nil
	case KindEmbedStream:
		return "catalog:embed:" + qualified, nil
	case KindSubjects, KindObjects, KindSubjectsDense, KindObjectsDense:
		return QdrantNamespace(namespace, generation) + "_" + string(kind), nil
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
