package ssetest

import (
	"strings"
	"testing"
	"time"
)

func TestReadParsesEventNameDataAndID(t *testing.T) {
	body := strings.NewReader(
		"id: 1\nevent: phase_started\ndata: {\"phase\":1}\n\n" +
			"event: log_line\ndata: {\"msg\":\"hi\"}\n\n" +
			"id: 3\nevent: ping\ndata: {}\n\n",
	)
	events := Read(t, body, 3, time.Second)

	if events[0] != (Event{ID: "1", Name: "phase_started", Data: `{"phase":1}`}) {
		t.Errorf("event[0]=%+v", events[0])
	}
	if events[1] != (Event{Name: "log_line", Data: `{"msg":"hi"}`}) {
		t.Errorf("event[1]=%+v", events[1])
	}
	if events[2] != (Event{ID: "3", Name: "ping", Data: "{}"}) {
		t.Errorf("event[2]=%+v", events[2])
	}
}
