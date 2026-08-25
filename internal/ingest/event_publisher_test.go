package ingest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEventTailChannel(t *testing.T) {
	if got := EventTailChannel("prod"); got != "codohue:events-tail:prod" {
		t.Errorf("EventTailChannel: got %q", got)
	}
	if EventTailChannelPattern != "codohue:events-tail:*" {
		t.Errorf("pattern: got %q", EventTailChannelPattern)
	}
}

func TestRedisEventTailPublisher_NonBlockingDrop(t *testing.T) {
	// buffer=2, Run never started → first two Publishes fill it, third drops.
	p := NewRedisEventTailPublisher(nil, 2)
	var drops int
	p.dropped = func() { drops++ }

	p.Publish(EventTailMessage{ID: 1, Namespace: "ns"})
	p.Publish(EventTailMessage{ID: 2, Namespace: "ns"}) // buffer now full
	p.Publish(EventTailMessage{ID: 3, Namespace: "ns"}) // dropped

	if drops != 1 {
		t.Errorf("expected exactly 1 drop, got %d", drops)
	}
}

// The tail crosses a process boundary (cmd/api → Redis pub/sub → cmd/admin),
// so the generation must be additive on that wire: absent for generation-1
// namespaces, and decodable by a build that predates the field.
func TestEventTailMessage_GenerationWireCompatibility(t *testing.T) {
	legacy, err := json.Marshal(EventTailMessage{ID: 1, Namespace: "ns", Action: "LIKE"})
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if strings.Contains(string(legacy), "namespace_generation") {
		t.Errorf("generation 0 must not reach the wire: %s", legacy)
	}

	qualified, err := json.Marshal(EventTailMessage{ID: 1, Namespace: "ns", NamespaceGeneration: 6, Action: "LIKE"})
	if err != nil {
		t.Fatalf("marshal qualified: %v", err)
	}
	if !strings.Contains(string(qualified), `"namespace_generation":6`) {
		t.Errorf("generation 6 missing from wire: %s", qualified)
	}

	var decoded EventTailMessage
	if err := json.Unmarshal([]byte(`{"id":3,"namespace":"ns","action":"VIEW"}`), &decoded); err != nil {
		t.Fatalf("decode pre-generation payload: %v", err)
	}
	if decoded.NamespaceGeneration != 0 || decoded.ID != 3 {
		t.Errorf("pre-generation payload decoded as %+v", decoded)
	}
}

func TestNewRedisEventTailPublisher_DefaultBuffer(t *testing.T) {
	p := NewRedisEventTailPublisher(nil, 0)
	if cap(p.ch) != 4096 {
		t.Errorf("default buffer: got cap %d, want 4096", cap(p.ch))
	}
}
