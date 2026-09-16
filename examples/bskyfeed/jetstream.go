package main

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// defaultJetstreamURL is a public Jetstream instance. Jetstream re-serves the
// AT Protocol firehose as plain JSON instead of CBOR/CAR, needs no auth, and
// lets the subscriber filter server-side by collection.
const defaultJetstreamURL = "wss://jetstream2.us-east.bsky.network/subscribe"

// Collections this feeder subscribes to. Anything else (follows, blocks,
// profile edits) carries no subject→object signal worth ingesting.
const (
	collectionLike   = "app.bsky.feed.like"
	collectionRepost = "app.bsky.feed.repost"
	collectionPost   = "app.bsky.feed.post"
)

const (
	// maxMessageBytes bounds a single frame. Jetstream records are small;
	// the limit exists so a malformed frame cannot exhaust memory.
	maxMessageBytes = 1 << 20

	// minCatalogTextRunes skips posts too short to embed usefully. Counted in
	// runes, not bytes: a large share of Bluesky traffic is Japanese, where a
	// byte threshold would reject perfectly good posts.
	minCatalogTextRunes = 20

	// cursorRewind is how far back a reconnect resumes. Jetstream replays from
	// a cursor, so rewinding past the last record seen covers the records that
	// were in flight when the socket dropped. Duplicates are harmless — the
	// catalog upsert is keyed by (namespace, object_id) and a replayed event
	// is at worst a repeated interaction.
	cursorRewind = 5 * time.Second

	maxBackoff = 30 * time.Second
)

// jetstreamMessage is the subset of a Jetstream message this feeder reads.
// Record stays raw so it can be decoded against the shape its collection
// implies.
type jetstreamMessage struct {
	DID    string `json:"did"`
	TimeUS int64  `json:"time_us"`
	Kind   string `json:"kind"`
	Commit *struct {
		Operation  string          `json:"operation"`
		Collection string          `json:"collection"`
		RKey       string          `json:"rkey"`
		Record     json.RawMessage `json:"record"`
	} `json:"commit"`
}

// subjectRecord is the shared shape of a like and a repost: both point at the
// post they act on through subject.uri.
type subjectRecord struct {
	Subject struct {
		URI string `json:"uri"`
	} `json:"subject"`
}

// postRecord is the part of a post record the feeder uses. Reply is set only
// on replies and identifies the post being replied to.
type postRecord struct {
	Text  string   `json:"text"`
	Langs []string `json:"langs"`
	Reply *struct {
		Parent struct {
			URI string `json:"uri"`
		} `json:"parent"`
	} `json:"reply"`
}

// message is one decoded record. Both fields are optional and a reply sets
// both: its text is catalog content and its parent link is an interaction.
type message struct {
	event *codohuetypes.EventPayload
	item  *codohuetypes.CatalogStreamItem
}

func (m message) empty() bool { return m.event == nil && m.item == nil }

// decoder turns Jetstream messages into Codohue payloads for one namespace.
type decoder struct {
	namespace     string
	samplePercent int
}

// keepActor decides whether an actor's records enter the feed.
//
// Sampling by actor rather than by record is deliberate. Dropping a random
// fraction of records thins every actor's history, and collaborative
// filtering reads co-occurrence: an actor seen twice out of ten interactions
// contributes almost nothing. Keeping a whole subset of actors instead leaves
// each kept actor's history intact, so halving the volume halves the subject
// count without degrading the signal per subject.
func (d *decoder) keepActor(did string) bool {
	if d.samplePercent >= 100 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(did))
	return int(h.Sum32()%100) < d.samplePercent
}

// decode returns the payloads a raw Jetstream frame implies, plus the frame's
// time_us cursor (returned even for skipped frames so a reconnect can resume
// from the real position rather than the last kept record).
func (d *decoder) decode(raw []byte) (message, int64) {
	var msg jetstreamMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return message{}, 0
	}
	if msg.Kind != "commit" || msg.Commit == nil || msg.Commit.Operation != "create" {
		return message{}, msg.TimeUS
	}
	if msg.DID == "" || !d.keepActor(msg.DID) {
		return message{}, msg.TimeUS
	}

	// time_us is Jetstream's own receive timestamp. The record's createdAt is
	// client-supplied and routinely skewed or backdated, which would corrupt
	// the time-decay weighting in compute and trending.
	occurred := time.UnixMicro(msg.TimeUS).UTC()

	switch msg.Commit.Collection {
	case collectionLike:
		return message{event: d.interaction(msg, occurred, codohuetypes.ActionLike)}, msg.TimeUS
	case collectionRepost:
		return message{event: d.interaction(msg, occurred, codohuetypes.ActionShare)}, msg.TimeUS
	case collectionPost:
		return d.post(msg, occurred), msg.TimeUS
	}
	return message{}, msg.TimeUS
}

// interaction builds the event for a like or repost.
func (d *decoder) interaction(msg jetstreamMessage, occurred time.Time, action codohuetypes.Action) *codohuetypes.EventPayload {
	var rec subjectRecord
	if err := json.Unmarshal(msg.Commit.Record, &rec); err != nil || rec.Subject.URI == "" {
		return nil
	}
	return &codohuetypes.EventPayload{
		Namespace:  d.namespace,
		SubjectID:  msg.DID,
		ObjectID:   rec.Subject.URI,
		Action:     action,
		OccurredAt: occurred,
	}
}

// post builds the catalog item for a post and, when the post is a reply, the
// COMMENT event linking its author to the parent post.
func (d *decoder) post(msg jetstreamMessage, occurred time.Time) message {
	var rec postRecord
	if err := json.Unmarshal(msg.Commit.Record, &rec); err != nil {
		return message{}
	}

	var out message
	if text := strings.TrimSpace(rec.Text); utf8.RuneCountInString(text) >= minCatalogTextRunes {
		out.item = &codohuetypes.CatalogStreamItem{
			Namespace:       d.namespace,
			ObjectID:        postURI(msg.DID, msg.Commit.RKey),
			Content:         text,
			AuthorSubjectID: msg.DID,
			Metadata:        map[string]any{"source": "bluesky", "langs": rec.Langs},
		}
	}
	if rec.Reply != nil && rec.Reply.Parent.URI != "" {
		out.event = &codohuetypes.EventPayload{
			Namespace:  d.namespace,
			SubjectID:  msg.DID,
			ObjectID:   rec.Reply.Parent.URI,
			Action:     codohuetypes.ActionComment,
			OccurredAt: occurred,
		}
	}
	return out
}

// postURI rebuilds the AT-URI of a post from its author and record key, so a
// post ingested as catalog content shares an object_id with the likes and
// reposts that reference it.
func postURI(did, rkey string) string {
	return "at://" + did + "/" + collectionPost + "/" + rkey
}

// readJetstream keeps a subscription alive for the lifetime of ctx, pushing
// decoded messages onto out. It reconnects with exponential backoff and
// resumes from the last cursor it saw.
func readJetstream(ctx context.Context, cfg config, out chan<- message, st *stats) {
	d := &decoder{namespace: cfg.namespace, samplePercent: cfg.samplePercent}
	backoff := time.Second
	var cursor int64

	for ctx.Err() == nil {
		read, last, err := streamOnce(ctx, cfg, cursor, d, out, st)
		if last > 0 {
			cursor = last - cursorRewind.Microseconds()
		}
		if ctx.Err() != nil {
			return
		}

		st.reconnects.Add(1)
		log.Printf("jetstream disconnected after %d messages: %v", read, err)

		// A connection that delivered data was healthy; only a connection that
		// failed immediately should escalate the wait.
		if read > 0 {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// streamOnce runs a single subscription until it fails, returning how many
// messages it read and the last cursor seen.
func streamOnce(ctx context.Context, cfg config, cursor int64, d *decoder, out chan<- message, st *stats) (int, int64, error) {
	q := url.Values{}
	for _, c := range []string{collectionLike, collectionRepost, collectionPost} {
		q.Add("wantedCollections", c)
	}
	if cursor > 0 {
		q.Set("cursor", strconv.FormatInt(cursor, 10))
	}

	conn, _, err := websocket.Dial(ctx, cfg.jetstreamURL+"?"+q.Encode(), nil)
	if err != nil {
		return 0, 0, err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxMessageBytes)
	log.Printf("connected to %s (cursor=%d)", cfg.jetstreamURL, cursor)

	var read int
	var last int64
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return read, last, err
		}
		read++

		msg, ts := d.decode(raw)
		if ts > 0 {
			last = ts
		}
		if msg.empty() {
			continue
		}
		// Never block the socket read: Jetstream drops subscribers that fall
		// behind, and a dropped record costs less than a dropped connection.
		select {
		case out <- msg:
		default:
			st.dropped.Add(1)
		}
	}
}
