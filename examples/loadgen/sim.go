package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// simulator turns a fixed catalog into a never-ending, correlated stream of
// behavioral events. Each simulated user has a category-affinity vector, so
// the events they generate carry a learnable preference signal rather than
// uniform noise.
type simulator struct {
	rng     *rand.Rand
	users   []user
	byCat   map[string][]catalogItem
	cats    []string
	pending []codohuetypes.EventPayload // buffered events from the current session
}

// user is one simulated actor with a per-category affinity in [0,1].
type user struct {
	id      string
	affzero map[string]float64
}

func newSimulator(seed uint64, numUsers int) *simulator {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	cats := categories()

	byCat := map[string][]catalogItem{}
	for _, it := range catalogItems {
		byCat[it.Category] = append(byCat[it.Category], it)
	}

	users := make([]user, numUsers)
	for i := range users {
		aff := make(map[string]float64, len(cats))
		for _, c := range cats {
			// Base low interest everywhere; most categories stay near zero.
			aff[c] = rng.Float64() * 0.2
		}
		// Give each user one or two strong interests so behavior is coherent.
		strong := 1 + rng.IntN(2)
		for s := 0; s < strong; s++ {
			c := cats[rng.IntN(len(cats))]
			aff[c] = 0.7 + rng.Float64()*0.3
		}
		users[i] = user{id: fmt.Sprintf("u_%04d", i), affzero: aff}
	}

	return &simulator{rng: rng, users: users, byCat: byCat, cats: cats}
}

// next returns the next event to send, generating a fresh browsing session
// whenever the buffer drains. Emitting one buffered event per call keeps the
// per-session ordering (VIEW before any follow-up) intact while the caller
// controls the overall rate.
func (s *simulator) next() codohuetypes.EventPayload {
	if len(s.pending) == 0 {
		s.fillSession()
	}
	ev := s.pending[0]
	s.pending = s.pending[1:]
	ev.OccurredAt = time.Now().UTC()
	return ev
}

// fillSession simulates one user browsing several items and records the
// resulting events into the pending buffer.
func (s *simulator) fillSession() {
	u := s.users[s.rng.IntN(len(s.users))]
	views := 3 + s.rng.IntN(6) // 3..8 items this session

	for i := 0; i < views; i++ {
		it := s.pickItem(u)
		affinity := u.affzero[it.Category]

		// Every impression is a VIEW.
		s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionView))

		// Follow-up action sampled from the user's affinity for this item.
		switch {
		case affinity < 0.2:
			// Mostly uninterested — sometimes an explicit skip.
			if s.rng.Float64() < 0.5 {
				s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionSkip))
			}
		case affinity < 0.6:
			// Lukewarm — an occasional like.
			if s.rng.Float64() < 0.3 {
				s.pending = append(s.pending, s.event(u.id, it, codohuetypes.ActionLike))
			}
		default:
			// Strong interest — like, and sometimes comment or share on top.
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
}

// pickItem chooses an item for the user: usually from a category they like,
// occasionally a random exploration into something new.
func (s *simulator) pickItem(u user) catalogItem {
	if s.rng.Float64() < 0.15 {
		return catalogItems[s.rng.IntN(len(catalogItems))]
	}
	cat := s.weightedCategory(u)
	pool := s.byCat[cat]
	return pool[s.rng.IntN(len(pool))]
}

// weightedCategory samples a category in proportion to the user's affinity.
func (s *simulator) weightedCategory(u user) string {
	var total float64
	for _, c := range s.cats {
		total += u.affzero[c]
	}
	if total <= 0 {
		return s.cats[s.rng.IntN(len(s.cats))]
	}
	r := s.rng.Float64() * total
	for _, c := range s.cats {
		r -= u.affzero[c]
		if r <= 0 {
			return c
		}
	}
	return s.cats[len(s.cats)-1]
}

func (s *simulator) event(subjectID string, it catalogItem, action codohuetypes.Action) codohuetypes.EventPayload {
	return codohuetypes.EventPayload{
		SubjectID: subjectID,
		ObjectID:  it.ObjectID,
		Action:    action,
		Metadata:  map[string]string{"category": it.Category},
	}
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
