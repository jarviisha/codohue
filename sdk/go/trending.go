package codohue

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// Trending returns the trending items for this namespace from the Redis ZSET.
// Use WithLimit, WithOffset, and WithWindowHours to customize the query.
func (n *Namespace) Trending(ctx context.Context, opts ...ListOption) (*codohuetypes.TrendingResponse, error) {
	o := buildListOptions(opts)

	q := url.Values{}
	if o.limit > 0 {
		q.Set("limit", strconv.Itoa(o.limit))
	}
	if o.offset > 0 {
		q.Set("offset", strconv.Itoa(o.offset))
	}
	if o.windowHours > 0 {
		q.Set("window_hours", strconv.Itoa(o.windowHours))
	}

	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/trending"
	var out codohuetypes.TrendingResponse
	if err := n.client.do(ctx, http.MethodGet, path, n.apiKey, q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
