package qdrant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/qdrant/go-client/qdrant"
)

// PointInventory is one point as the migration-022 audit sees it: the payload
// identity it claims, the numeric id it is stored under, and hashes of what it
// carries.
//
// The hashes exist so a copy can be *proved* rather than assumed. A dense
// vector pushed by a client (BYOE) or produced by a strategy the operator has
// since retired is not recomputable — if the copy silently truncated or
// reordered it, no later run could notice.
type PointInventory struct {
	Collection  string
	NumericID   uint64
	StringID    string
	PayloadHash string
	VectorHash  string
}

// repairScrollPageSize keeps each inventory page small enough that a large
// collection does not have to be held in one response.
const repairScrollPageSize = uint32(512)

// PointReader is the read surface the inventory needs. Declared here so the
// repair primitives can be driven without a live Qdrant.
type PointReader interface {
	ScrollAndOffset(ctx context.Context, request *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error)
}

// PointWriter is the mutation surface a repair needs: upsert the copy, then
// delete the original.
type PointWriter interface {
	Upsert(ctx context.Context, request *qdrant.UpsertPoints) (*qdrant.UpdateResult, error)
	Delete(ctx context.Context, request *qdrant.DeletePoints) (*qdrant.UpdateResult, error)
	Get(ctx context.Context, request *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error)
}

// InventoryCollection walks every point in a collection, recording the payload
// string id alongside the numeric id it is stored under.
//
// idField is the payload key holding the logical identity ("subject_id" for
// subject collections, "object_id" for object ones). A point whose payload
// lacks it is still returned, with an empty StringID: a point that cannot be
// associated with a logical identity is precisely the ambiguous evidence the
// audit must quarantine rather than skip.
func InventoryCollection(ctx context.Context, client PointReader, collection, idField string) ([]PointInventory, error) {
	var out []PointInventory
	var offset *qdrant.PointId
	for {
		points, next, err := client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
			CollectionName: collection,
			Limit:          qdrant.PtrOf(repairScrollPageSize),
			Offset:         offset,
			WithPayload:    qdrant.NewWithPayload(true),
			WithVectors:    qdrant.NewWithVectors(true),
		})
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", collection, err)
		}
		for _, point := range points {
			out = append(out, PointInventory{
				Collection:  collection,
				NumericID:   point.GetId().GetNum(),
				StringID:    point.GetPayload()[idField].GetStringValue(),
				PayloadHash: HashPayload(point.GetPayload()),
				VectorHash:  HashVectors(point.GetVectors()),
			})
		}
		if next == nil || len(points) == 0 {
			break
		}
		offset = next
	}
	return out, nil
}

// HashPayload fingerprints a point payload deterministically.
//
// Keys are sorted because Go map iteration order is random, and an unstable
// hash would report every verified copy as corrupt.
func HashPayload(payload map[string]*qdrant.Value) string {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\x1f", key, payload[key].String())
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// HashVectors fingerprints a point's vectors, named vectors included.
//
// Float bits are hashed rather than a formatted value so a copy that loses
// precision is detected, not rounded away.
func HashVectors(vectors *qdrant.VectorsOutput) string {
	if vectors == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(vectors.String()))
	return hex.EncodeToString(sum[:])
}

// CopyPointVerified copies a point to a new numeric id and proves the copy
// landed byte-identical before reporting success.
//
// The verification read is not paranoia about Qdrant: an upsert that silently
// coerced a vector (wrong dimension, dropped named vector) would otherwise be
// discovered only when recommendations quietly degraded, long after the
// original point was deleted.
func CopyPointVerified(ctx context.Context, client PointWriter, collection string, from, to uint64) error {
	if from == to {
		return nil
	}
	source, err := readPoint(ctx, client, collection, from)
	if err != nil {
		return err
	}
	if source == nil {
		return fmt.Errorf("copy point %d in %s: source point is missing", from, collection)
	}

	wantPayload := HashPayload(source.GetPayload())
	wantVectors := HashVectors(source.GetVectors())

	if _, err := client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Wait:           qdrant.PtrOf(true),
		Points: []*qdrant.PointStruct{{
			Id:      qdrant.NewIDNum(to),
			Payload: source.GetPayload(),
			Vectors: vectorsFromOutput(source.GetVectors()),
		}},
	}); err != nil {
		return fmt.Errorf("copy point %d→%d in %s: %w", from, to, collection, err)
	}

	copied, err := readPoint(ctx, client, collection, to)
	if err != nil {
		return err
	}
	if copied == nil {
		return fmt.Errorf("verify copy %d→%d in %s: point is absent after upsert", from, to, collection)
	}
	if got := HashPayload(copied.GetPayload()); got != wantPayload {
		return fmt.Errorf("verify copy %d→%d in %s: payload hash %s, want %s", from, to, collection, got, wantPayload)
	}
	if got := HashVectors(copied.GetVectors()); got != wantVectors {
		return fmt.Errorf("verify copy %d→%d in %s: vector hash %s, want %s", from, to, collection, got, wantVectors)
	}
	return nil
}

// DeletePoint removes a single point. Called only after CopyPointVerified has
// proved the replacement exists — the reverse order would lose an
// unrecomputable vector on any failure in between.
func DeletePoint(ctx context.Context, client PointWriter, collection string, id uint64) error {
	if _, err := client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Wait:           qdrant.PtrOf(true),
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{Ids: []*qdrant.PointId{qdrant.NewIDNum(id)}},
			},
		},
	}); err != nil {
		return fmt.Errorf("delete point %d from %s: %w", id, collection, err)
	}
	return nil
}

// PointAbsent reports whether a numeric id no longer exists, which is what
// verification checks after cleanup.
func PointAbsent(ctx context.Context, client PointWriter, collection string, id uint64) (bool, error) {
	point, err := readPoint(ctx, client, collection, id)
	if err != nil {
		return false, err
	}
	return point == nil, nil
}

func readPoint(ctx context.Context, client PointWriter, collection string, id uint64) (*qdrant.RetrievedPoint, error) {
	points, err := client.Get(ctx, &qdrant.GetPoints{
		CollectionName: collection,
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(id)},
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, fmt.Errorf("read point %d from %s: %w", id, collection, err)
	}
	if len(points) == 0 {
		return nil, nil
	}
	return points[0], nil
}

// vectorsFromOutput converts a read result back into upsert input.
//
// The read and write shapes are different protobuf types with the same
// oneof cases, and the legacy flat Data/Indices accessors are empty for
// anything Qdrant returns today — reading through them silently produced an
// empty vector, which is exactly the "copy that looks like it worked" this
// package exists to prevent. Every case is converted explicitly, and an
// unrecognised one returns nil so CopyPointVerified's hash check fails loudly
// rather than writing a truncated point.
func vectorsFromOutput(out *qdrant.VectorsOutput) *qdrant.Vectors {
	if out == nil {
		return nil
	}
	if named := out.GetVectors(); named != nil {
		converted := make(map[string]*qdrant.Vector, len(named.GetVectors()))
		for name, vector := range named.GetVectors() {
			converted[name] = vectorFromOutput(vector)
		}
		return &qdrant.Vectors{
			VectorsOptions: &qdrant.Vectors_Vectors{Vectors: &qdrant.NamedVectors{Vectors: converted}},
		}
	}
	if single := out.GetVector(); single != nil {
		return &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vector{Vector: vectorFromOutput(single)}}
	}
	return nil
}

func vectorFromOutput(out *qdrant.VectorOutput) *qdrant.Vector {
	switch {
	case out.GetDense() != nil:
		return &qdrant.Vector{Vector: &qdrant.Vector_Dense{Dense: out.GetDense()}}
	case out.GetSparse() != nil:
		return &qdrant.Vector{Vector: &qdrant.Vector_Sparse{Sparse: out.GetSparse()}}
	case out.GetMultiDense() != nil:
		return &qdrant.Vector{Vector: &qdrant.Vector_MultiDense{MultiDense: out.GetMultiDense()}}
	default:
		return nil
	}
}

// ValidateSnapshotRefs checks that every affected collection has a recorded
// snapshot before any mutation runs.
//
// A partially-snapshotted repair has no recovery path: restoring PostgreSQL
// without the matching Qdrant collection reintroduces exactly the cross-store
// divergence the repair is fixing.
func ValidateSnapshotRefs(collections []string, refs map[string]string) error {
	var missing []string
	for _, collection := range collections {
		if strings.TrimSpace(refs[collection]) == "" {
			missing = append(missing, collection)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing qdrant snapshot references for: %s", strings.Join(missing, ", "))
	}
	return nil
}

// IsMissingCollection reports whether an error is Qdrant saying the collection
// does not exist. A namespace that has never had dense vectors simply has no
// dense collection — that is not evidence of a problem, so the audit skips it
// rather than aborting the whole run.
func IsMissingCollection(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "doesn't exist") ||
		strings.Contains(message, "does not exist") ||
		strings.Contains(message, "not found")
}
