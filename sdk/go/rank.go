package codohue

import (
	"context"
	"net/http"
	"net/url"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// Rank scores and ranks a list of candidate object IDs for a subject.
// The server enforces a maximum of 500 candidates per call.
func (n *Namespace) Rank(ctx context.Context, subjectID string, candidates []string) (*codohuetypes.RankResponse, error) {
	body := codohuetypes.RankRequest{
		SubjectID:  subjectID,
		Namespace:  n.namespace,
		Candidates: candidates,
	}
	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/rank"

	var out codohuetypes.RankResponse
	if err := n.client.do(ctx, http.MethodPost, path, n.apiKey, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
