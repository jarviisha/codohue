package codohue

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// ListOption configures list-style endpoints (Recommend, Trending). Each option
// is a no-op on endpoints that don't accept the corresponding parameter.
type ListOption func(*listOptions)

type listOptions struct {
	limit       int
	offset      int
	windowHours int
}

// WithLimit caps the number of items returned. Applies to Recommend and Trending.
func WithLimit(n int) ListOption { return func(o *listOptions) { o.limit = n } }

// WithOffset skips the first n items. Applies to Trending.
func WithOffset(n int) ListOption { return func(o *listOptions) { o.offset = n } }

// WithWindowHours overrides the trending look-back window in hours. Applies to Trending.
func WithWindowHours(n int) ListOption { return func(o *listOptions) { o.windowHours = n } }

func buildListOptions(opts []ListOption) listOptions {
	var o listOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Recommend returns collaborative-filtering recommendations for the given
// subject in this namespace.
func (n *Namespace) Recommend(ctx context.Context, subjectID string, opts ...ListOption) (*codohuetypes.Response, error) {
	o := buildListOptions(opts)

	q := url.Values{}
	q.Set("subject_id", subjectID)
	if o.limit > 0 {
		q.Set("limit", strconv.Itoa(o.limit))
	}

	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/recommendations"
	var out codohuetypes.Response
	if err := n.client.do(ctx, http.MethodGet, path, n.apiKey, q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
