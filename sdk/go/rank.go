package codohue

import (
	"context"
	"net/http"
	"net/url"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// Rank scores and ranks a list of candidate object IDs for a subject.
// The server enforces a maximum of 500 candidates per call.
//
// Every submitted candidate comes back. Check RankedItem.Scored before
// treating a score as a relevance verdict: Scored=false means the item was
// returned unscored (never indexed, or excluded by the namespace's
// seen-items / authored-objects filters). A response with
// Source="no_subject_vector" means the subject has no vector at all — the
// whole list is unscored in request order, so callers should keep their own
// ordering and may skip re-ranking that subject until it has interactions.
// Source="hybrid_rank" is the scored case.
//
// Scores are comparable across calls: ranking a candidate set in chunks
// yields the same relative ordering as one call over the union.
func (n *Namespace) Rank(ctx context.Context, subjectID string, candidates []string) (*codohuetypes.RankResponse, error) {
	body := codohuetypes.RankRequest{
		SubjectID:  subjectID,
		Candidates: candidates,
	}
	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/rankings"

	var out codohuetypes.RankResponse
	if err := n.client.do(ctx, http.MethodPost, path, n.apiKey, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
