package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// timeUS is a fixed Jetstream cursor used across the table: 2026-09-16T10:00:00Z.
const timeUS int64 = 1789639200000000

func TestDecode(t *testing.T) {
	d := &decoder{namespace: "bluesky", samplePercent: 100}
	wantOccurred := time.UnixMicro(timeUS).UTC()

	tests := []struct {
		name        string
		raw         string
		wantEvent   *codohuetypes.EventPayload
		wantItem    *codohuetypes.CatalogStreamItem
		wantCursor  int64
		wantContent string
	}{
		{
			name: "like becomes a LIKE event pointing at the liked post",
			raw: `{"did":"did:plc:alice","time_us":1789639200000000,"kind":"commit",
				"commit":{"operation":"create","collection":"app.bsky.feed.like","rkey":"r1",
				"record":{"subject":{"uri":"at://did:plc:bob/app.bsky.feed.post/p1"}}}}`,
			wantEvent: &codohuetypes.EventPayload{
				Namespace: "bluesky", SubjectID: "did:plc:alice",
				ObjectID: "at://did:plc:bob/app.bsky.feed.post/p1",
				Action:   codohuetypes.ActionLike, OccurredAt: wantOccurred,
			},
			wantCursor: timeUS,
		},
		{
			name: "repost becomes a SHARE event",
			raw: `{"did":"did:plc:alice","time_us":1789639200000000,"kind":"commit",
				"commit":{"operation":"create","collection":"app.bsky.feed.repost","rkey":"r2",
				"record":{"subject":{"uri":"at://did:plc:bob/app.bsky.feed.post/p2"}}}}`,
			wantEvent: &codohuetypes.EventPayload{
				Namespace: "bluesky", SubjectID: "did:plc:alice",
				ObjectID: "at://did:plc:bob/app.bsky.feed.post/p2",
				Action:   codohuetypes.ActionShare, OccurredAt: wantOccurred,
			},
			wantCursor: timeUS,
		},
		{
			name: "top-level post becomes catalog content with no event",
			raw: `{"did":"did:plc:carol","time_us":1789639200000000,"kind":"commit",
				"commit":{"operation":"create","collection":"app.bsky.feed.post","rkey":"p3",
				"record":{"text":"a post long enough to be worth embedding","langs":["en"]}}}`,
			wantItem: &codohuetypes.CatalogStreamItem{
				Namespace:       "bluesky",
				ObjectID:        "at://did:plc:carol/app.bsky.feed.post/p3",
				AuthorSubjectID: "did:plc:carol",
			},
			wantContent: "a post long enough to be worth embedding",
			wantCursor:  timeUS,
		},
		{
			name: "reply yields both catalog content and a COMMENT event on the parent",
			raw: `{"did":"did:plc:carol","time_us":1789639200000000,"kind":"commit",
				"commit":{"operation":"create","collection":"app.bsky.feed.post","rkey":"p4",
				"record":{"text":"replying with more than twenty runes of text",
				"reply":{"parent":{"uri":"at://did:plc:bob/app.bsky.feed.post/p1"}}}}}`,
			wantEvent: &codohuetypes.EventPayload{
				Namespace: "bluesky", SubjectID: "did:plc:carol",
				ObjectID: "at://did:plc:bob/app.bsky.feed.post/p1",
				Action:   codohuetypes.ActionComment, OccurredAt: wantOccurred,
			},
			wantItem: &codohuetypes.CatalogStreamItem{
				Namespace:       "bluesky",
				ObjectID:        "at://did:plc:carol/app.bsky.feed.post/p4",
				AuthorSubjectID: "did:plc:carol",
			},
			wantContent: "replying with more than twenty runes of text",
			wantCursor:  timeUS,
		},
		{
			name: "post too short to embed is skipped",
			raw: `{"did":"did:plc:carol","time_us":1789639200000000,"kind":"commit",
				"commit":{"operation":"create","collection":"app.bsky.feed.post","rkey":"p5",
				"record":{"text":"hi"}}}`,
			wantCursor: timeUS,
		},
		{
			name: "a delete is not an interaction",
			raw: `{"did":"did:plc:alice","time_us":1789639200000000,"kind":"commit",
				"commit":{"operation":"delete","collection":"app.bsky.feed.like","rkey":"r1"}}`,
			wantCursor: timeUS,
		},
		{
			name:       "non-commit messages still advance the cursor",
			raw:        `{"did":"did:plc:alice","time_us":1789639200000000,"kind":"identity"}`,
			wantCursor: timeUS,
		},
		{
			name:       "malformed frames are dropped without a cursor",
			raw:        `{not json`,
			wantCursor: 0,
		},
		{
			name: "unwanted collections are ignored",
			raw: `{"did":"did:plc:alice","time_us":1789639200000000,"kind":"commit",
				"commit":{"operation":"create","collection":"app.bsky.graph.follow","rkey":"f1",
				"record":{"subject":"did:plc:bob"}}}`,
			wantCursor: timeUS,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, cursor := d.decode([]byte(tc.raw))

			if cursor != tc.wantCursor {
				t.Errorf("cursor = %d, want %d", cursor, tc.wantCursor)
			}

			switch {
			case tc.wantEvent == nil && got.event != nil:
				t.Errorf("unexpected event %+v", *got.event)
			case tc.wantEvent != nil && got.event == nil:
				t.Error("expected an event, got none")
			case tc.wantEvent != nil:
				if *got.event != *tc.wantEvent {
					t.Errorf("event = %+v, want %+v", *got.event, *tc.wantEvent)
				}
			}

			switch {
			case tc.wantItem == nil && got.item != nil:
				t.Errorf("unexpected catalog item %+v", *got.item)
			case tc.wantItem != nil && got.item == nil:
				t.Error("expected a catalog item, got none")
			case tc.wantItem != nil:
				if got.item.ObjectID != tc.wantItem.ObjectID {
					t.Errorf("item.ObjectID = %q, want %q", got.item.ObjectID, tc.wantItem.ObjectID)
				}
				if got.item.AuthorSubjectID != tc.wantItem.AuthorSubjectID {
					t.Errorf("item.AuthorSubjectID = %q, want %q", got.item.AuthorSubjectID, tc.wantItem.AuthorSubjectID)
				}
				if got.item.Content != tc.wantContent {
					t.Errorf("item.Content = %q, want %q", got.item.Content, tc.wantContent)
				}
			}
		})
	}
}

// A like on a post the feeder also ingested as catalog content must agree on
// object_id, otherwise the interaction graph and the dense vectors describe
// different objects.
func TestPostURIMatchesLikeSubjectURI(t *testing.T) {
	d := &decoder{namespace: "bluesky", samplePercent: 100}

	post, _ := d.decode([]byte(`{"did":"did:plc:bob","time_us":1789639200000000,"kind":"commit",
		"commit":{"operation":"create","collection":"app.bsky.feed.post","rkey":"abc123",
		"record":{"text":"the post that is about to be liked by somebody"}}}`))
	like, _ := d.decode([]byte(`{"did":"did:plc:alice","time_us":1789639200000001,"kind":"commit",
		"commit":{"operation":"create","collection":"app.bsky.feed.like","rkey":"r9",
		"record":{"subject":{"uri":"at://did:plc:bob/app.bsky.feed.post/abc123"}}}}`))

	if post.item == nil || like.event == nil {
		t.Fatal("expected both a catalog item and a like event")
	}
	if post.item.ObjectID != like.event.ObjectID {
		t.Errorf("object_id mismatch: catalog %q vs like %q", post.item.ObjectID, like.event.ObjectID)
	}
}

// createdAt in the record is client-supplied; the decoder must ignore it in
// favour of the firehose timestamp, or time decay reads attacker-chosen dates.
func TestDecodeIgnoresClientCreatedAt(t *testing.T) {
	d := &decoder{namespace: "bluesky", samplePercent: 100}

	got, _ := d.decode([]byte(`{"did":"did:plc:alice","time_us":1789639200000000,"kind":"commit",
		"commit":{"operation":"create","collection":"app.bsky.feed.like","rkey":"r1",
		"record":{"createdAt":"2099-01-01T00:00:00Z",
		"subject":{"uri":"at://did:plc:bob/app.bsky.feed.post/p1"}}}}`))

	if got.event == nil {
		t.Fatal("expected an event")
	}
	if want := time.UnixMicro(timeUS).UTC(); !got.event.OccurredAt.Equal(want) {
		t.Errorf("OccurredAt = %s, want %s (firehose time_us, not record createdAt)",
			got.event.OccurredAt, want)
	}
}

func TestKeepActor(t *testing.T) {
	t.Run("100 percent keeps every actor", func(t *testing.T) {
		d := &decoder{samplePercent: 100}
		for i := range 500 {
			if !d.keepActor(fmt.Sprintf("did:plc:%d", i)) {
				t.Fatalf("actor %d dropped at 100%%", i)
			}
		}
	})

	t.Run("sampling is stable for a given actor", func(t *testing.T) {
		d := &decoder{samplePercent: 50}
		for i := range 200 {
			did := fmt.Sprintf("did:plc:%d", i)
			if d.keepActor(did) != d.keepActor(did) {
				t.Fatalf("actor %q sampled inconsistently", did)
			}
		}
	})

	// The whole point of actor-level sampling is that the kept fraction tracks
	// the configured percentage, so volume can be tuned predictably.
	t.Run("kept fraction approximates the percentage", func(t *testing.T) {
		const n = 10000
		for _, pct := range []int{10, 50, 90} {
			d := &decoder{samplePercent: pct}
			kept := 0
			for i := range n {
				if d.keepActor(fmt.Sprintf("did:plc:actor%d", i)) {
					kept++
				}
			}
			got := float64(kept) / n * 100
			if got < float64(pct)-3 || got > float64(pct)+3 {
				t.Errorf("samplePercent=%d kept %.1f%%, want within 3 points", pct, got)
			}
		}
	})
}
