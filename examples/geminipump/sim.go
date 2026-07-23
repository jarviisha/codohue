package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// simulator turns the growing catalog into a never-ending, correlated stream of
// behavioral events. Each simulated user has a per-category affinity, so the
// events they generate carry a learnable preference signal rather than uniform
// noise. Only the pump goroutine drives the simulator, so its own state (rng,
// user affinities, pending buffer) needs no locking — the shared catalog is
// read through the store's snapshot.
type simulator struct {
	rng     *rand.Rand
	store   *catalogStore
	users   []user
	pending []codohuetypes.EventPayload // buffered events from the current session
}

// user is one simulated actor with a per-category affinity in [0,1]. Affinities
// for categories that appear later (as Gemini invents new ones) are filled in
// lazily the first time the user encounters them.
type user struct {
	id      string
	affzero map[string]float64
	strong  int // how many strong interests to assign among new categories
}

func newSimulator(seed uint64, numUsers int, store *catalogStore) *simulator {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	users := make([]user, numUsers)
	for i := range users {
		users[i] = user{
			id:      fmt.Sprintf("u_%04d", i),
			affzero: map[string]float64{},
			strong:  1 + rng.IntN(2), // each user ends up with 1-2 strong interests
		}
	}
	return &simulator{rng: rng, store: store, users: users}
}

// affinity returns the user's interest in a category, assigning one the first
// time the category is seen: mostly low, with a bounded number of strong picks.
func (s *simulator) affinity(u *user, cat string) float64 {
	if v, ok := u.affzero[cat]; ok {
		return v
	}
	v := s.rng.Float64() * 0.2 // base low interest everywhere
	if u.strong > 0 && s.rng.Float64() < 0.25 {
		v = 0.7 + s.rng.Float64()*0.3
		u.strong--
	}
	u.affzero[cat] = v
	return v
}

// next returns the next event to send, generating a fresh browsing session
// whenever the buffer drains. Returns ok=false when the catalog is still empty
// (before the first Gemini batch lands), so the caller can skip the tick.
func (s *simulator) next() (codohuetypes.EventPayload, bool) {
	for len(s.pending) == 0 {
		if !s.fillSession() {
			return codohuetypes.EventPayload{}, false
		}
	}
	ev := s.pending[0]
	s.pending = s.pending[1:]
	ev.OccurredAt = time.Now().UTC()
	return ev, true
}

// fillSession simulates one user browsing several items and records the
// resulting events into the pending buffer. Returns false when there is nothing
// to browse yet.
func (s *simulator) fillSession() bool {
	snap := s.store.snapshot()
	if len(snap.items) == 0 {
		return false
	}
	u := &s.users[s.rng.IntN(len(s.users))]
	views := 3 + s.rng.IntN(6) // 3..8 items this session

	for i := 0; i < views; i++ {
		it := s.pickItem(u, snap)
		aff := s.affinity(u, it.Category)

		// Every impression is a VIEW.
		s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionView))

		// Follow-up action sampled from the user's affinity for this item.
		switch {
		case aff < 0.2:
			if s.rng.Float64() < 0.5 {
				s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionSkip))
			}
		case aff < 0.6:
			if s.rng.Float64() < 0.3 {
				s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionLike))
			}
		default:
			if s.rng.Float64() < 0.8 {
				s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionLike))
			}
			if s.rng.Float64() < 0.35 {
				s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionComment))
			}
			if s.rng.Float64() < 0.25 {
				s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionShare))
			}
		}
	}
	return true
}

// pickItem chooses an item for the user: usually from a category they like,
// occasionally a random exploration into something new.
func (s *simulator) pickItem(u *user, snap catalogSnapshot) catalogItem {
	if s.rng.Float64() < 0.15 || len(snap.cats) == 0 {
		return snap.items[s.rng.IntN(len(snap.items))]
	}
	cat := s.weightedCategory(u, snap)
	pool := snap.byCat[cat]
	if len(pool) == 0 {
		return snap.items[s.rng.IntN(len(snap.items))]
	}
	return pool[s.rng.IntN(len(pool))]
}

// weightedCategory samples a category in proportion to the user's affinity.
func (s *simulator) weightedCategory(u *user, snap catalogSnapshot) string {
	var total float64
	for _, c := range snap.cats {
		total += s.affinity(u, c)
	}
	if total <= 0 {
		return snap.cats[s.rng.IntN(len(snap.cats))]
	}
	r := s.rng.Float64() * total
	for _, c := range snap.cats {
		r -= s.affinity(u, c)
		if r <= 0 {
			return c
		}
	}
	return snap.cats[len(snap.cats)-1]
}

func (s *simulator) event(subjectID string, it catalogItem, action codohuetypes.Action) codohuetypes.EventPayload {
	// Event metadata was dropped from the wire contract (the events table
	// never stored it); category lives on the catalog item, not the event.
	ev := codohuetypes.EventPayload{
		SubjectID: subjectID,
		ObjectID:  it.ObjectID,
		Action:    action,
	}
	if !it.CreatedAt.IsZero() {
		created := it.CreatedAt
		ev.ObjectCreatedAt = &created
	}
	return ev
}

// sampleUserIDs returns up to n distinct simulated user IDs, for read-back of
// recommendations.
func (s *simulator) sampleUserIDs(n int) []string {
	if n > len(s.users) {
		n = len(s.users)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, s.users[s.rng.IntN(len(s.users))].id)
	}
	return out
}
