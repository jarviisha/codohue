package main

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// catalogItem is one piece of Gemini-generated content. Title and Summary are
// concatenated into the catalog Content that feeds the embedder; Category drives
// each simulated user's affinity so the events we emit form a learnable signal.
// AuthorSubjectID attributes the item to one of the simulated users so the
// objects table fills up and the exclude_authored filter has data to act on.
type catalogItem struct {
	ObjectID        string
	Title           string
	Summary         string
	Category        string
	AuthorSubjectID string
	CreatedAt       time.Time
}

// catalogStore holds the growing catalog. The generator goroutine appends to it
// while the pump goroutine reads from it, so every access is guarded. Appends
// never mutate existing backing arrays in place, so a snapshot taken under the
// read lock stays safe to read after the lock is released.
type catalogStore struct {
	mu    sync.RWMutex
	items []catalogItem
	ids   map[string]bool
	byCat map[string][]catalogItem
	cats  []string
}

func newCatalogStore() *catalogStore {
	return &catalogStore{ids: map[string]bool{}, byCat: map[string][]catalogItem{}}
}

// add inserts items, skipping any with an empty or already-seen object_id, and
// returns the subset actually added (so the caller ingests each item once).
func (c *catalogStore) add(in []catalogItem) []catalogItem {
	c.mu.Lock()
	defer c.mu.Unlock()

	added := make([]catalogItem, 0, len(in))
	for _, it := range in {
		if it.ObjectID == "" || it.Category == "" || c.ids[it.ObjectID] {
			continue
		}
		c.ids[it.ObjectID] = true
		c.items = append(c.items, it)
		if _, ok := c.byCat[it.Category]; !ok {
			c.cats = append(c.cats, it.Category)
		}
		c.byCat[it.Category] = append(c.byCat[it.Category], it)
		added = append(added, it)
	}
	return added
}

func (c *catalogStore) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// snapshot returns a consistent view of the catalog for one browsing session.
// The item slices are shared read-only; the category list and map are copied so
// the reader never races a concurrent add.
type catalogSnapshot struct {
	items []catalogItem
	byCat map[string][]catalogItem
	cats  []string
}

func (c *catalogStore) snapshot() catalogSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cats := append([]string(nil), c.cats...)
	byCat := make(map[string][]catalogItem, len(c.byCat))
	for k, v := range c.byCat {
		byCat[k] = v // header copy; underlying array is append-only, safe to read
	}
	return catalogSnapshot{items: c.items, byCat: byCat, cats: cats}
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify normalizes a model-suggested id into a safe slug fragment.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugUnsafe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

// toCatalogItems converts a Gemini batch into store items with globally unique
// object IDs. The model's suggested id is only used as a slug hint; the batch
// and index prefix guarantee uniqueness across the whole run. Each item is
// attributed to a deterministic author from the numUsers simulated-user pool
// (matching the simulator's u_%04d ids); every eighth item is deliberately left
// unattributed so the unattributed rendering and filtering paths stay exercised.
func toCatalogItems(batch int, gen []genItem, now time.Time, numUsers int) []catalogItem {
	out := make([]catalogItem, 0, len(gen))
	for i, g := range gen {
		title := strings.TrimSpace(g.Title)
		summary := strings.TrimSpace(g.Summary)
		category := strings.ToLower(strings.TrimSpace(g.Category))
		if title == "" || category == "" {
			continue
		}
		slug := slugify(g.ObjectID)
		if slug == "" {
			slug = slugify(title)
		}
		var author string
		if numUsers > 0 && (batch+i)%8 != 0 {
			author = fmt.Sprintf("u_%04d", (batch*13+i)%numUsers)
		}
		out = append(out, catalogItem{
			ObjectID:        fmt.Sprintf("g%03d_%02d_%s", batch, i, slug),
			Title:           title,
			Summary:         summary,
			Category:        category,
			AuthorSubjectID: author,
			CreatedAt:       now,
		})
	}
	return out
}
