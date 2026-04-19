package compute

// UserDenseVectors derives a dense vector for each subject by mean-pooling
// the dense vectors of all items the subject has interacted with.
// Subjects with no interacted items that have a dense vector are omitted from the result.
//
// Staleness trade-off: this function runs inside the cron batch job, so the resulting
// subject vectors reflect behaviour up to the start of that batch run — not real-time.
// A subject who interacts with new items between two cron executions will have a stale
// dense component for the duration of that interval. The sparse CF path is unaffected
// because it queries Qdrant with vectors computed in the same batch.
//
// To reduce staleness: decrease BATCH_INTERVAL_MINUTES, or switch to BYOE and push
// updated subject embeddings via POST /v1/subjects/{ns}/{id}/embedding immediately
// after each interaction is recorded.
func UserDenseVectors(events []*RawEvent, itemVecs map[string][]float32) map[string][]float32 {
	if len(itemVecs) == 0 {
		return nil
	}

	// Accumulate item vectors per subject.
	type accumulator struct {
		sum   []float32
		count int
	}
	accum := make(map[string]*accumulator)

	for _, e := range events {
		vec, ok := itemVecs[e.ObjectID]
		if !ok {
			continue
		}
		a, exists := accum[e.SubjectID]
		if !exists {
			a = &accumulator{sum: make([]float32, len(vec))}
			accum[e.SubjectID] = a
		}
		for d, v := range vec {
			a.sum[d] += v
		}
		a.count++
	}

	// Compute mean vectors.
	result := make(map[string][]float32, len(accum))
	for subjectID, a := range accum {
		if a.count == 0 {
			continue
		}
		mean := make([]float32, len(a.sum))
		for d, v := range a.sum {
			mean[d] = v / float32(a.count)
		}
		result[subjectID] = mean
	}
	return result
}
