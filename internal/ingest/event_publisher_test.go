package ingest

import "testing"

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

func TestNewRedisEventTailPublisher_DefaultBuffer(t *testing.T) {
	p := NewRedisEventTailPublisher(nil, 0)
	if cap(p.ch) != 4096 {
		t.Errorf("default buffer: got cap %d, want 4096", cap(p.ch))
	}
}
