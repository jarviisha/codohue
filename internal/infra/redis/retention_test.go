package redis

import (
	"context"
	"errors"
	"testing"
)

type fakeRetentionBackend struct {
	groups     []retentionGroup
	groupsErr  error
	pending    map[string]retentionPending
	pendingErr map[string]error
	length     int64
	lengthErr  error
	trimmed    int64
	trimErr    error
	trimCalls  []string
}

func (f *fakeRetentionBackend) Groups(context.Context, string) ([]retentionGroup, error) {
	return f.groups, f.groupsErr
}

func (f *fakeRetentionBackend) Pending(_ context.Context, _, group string) (retentionPending, error) {
	return f.pending[group], f.pendingErr[group]
}

func (f *fakeRetentionBackend) Length(context.Context, string) (int64, error) {
	return f.length, f.lengthErr
}

func (f *fakeRetentionBackend) TrimMinID(_ context.Context, _, frontier string) (int64, error) {
	f.trimCalls = append(f.trimCalls, frontier)
	return f.trimmed, f.trimErr
}

func TestRetentionUsesMinimumMultiGroupSafeFrontier(t *testing.T) {
	backend := &fakeRetentionBackend{
		groups: []retentionGroup{
			{Name: "primary", Pending: 0, LastDeliveredID: "50-0", Lag: 3},
			{Name: "analytics", Pending: 2, LastDeliveredID: "70-0", Lag: 1},
			{Name: "unexpected", Pending: 0, LastDeliveredID: "40-0"},
		},
		pending: map[string]retentionPending{"analytics": {Count: 2, OldestID: "45-0"}},
		length:  100,
		trimmed: 39,
	}
	r := newRetentionWithBackend(backend, false)

	result, err := r.RunOnce(context.Background(), StreamSpec{
		Name: "codohue:events", Kind: "events", ExpectedGroups: []string{"primary", "analytics"},
	})
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.SafeFrontier != "40-0" {
		t.Fatalf("SafeFrontier = %q, want 40-0", result.SafeFrontier)
	}
	if result.Pending != 2 || result.Undelivered != 3 || result.UnexpectedGroups != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(backend.trimCalls) != 1 || backend.trimCalls[0] != "40-0" {
		t.Fatalf("exact trim calls = %v, want [40-0]", backend.trimCalls)
	}
	if result.Trimmed != 39 || result.Length != 100 || result.DryRun {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRetentionDryRunComputesWithoutTrimming(t *testing.T) {
	backend := &fakeRetentionBackend{
		groups: []retentionGroup{{Name: "primary", LastDeliveredID: "9-1"}},
		length: 10,
	}
	r := newRetentionWithBackend(backend, true)
	result, err := r.RunOnce(context.Background(), StreamSpec{
		Name: "codohue:catalog", Kind: "catalog", ExpectedGroups: []string{"primary"},
	})
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !result.DryRun || result.SafeFrontier != "9-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(backend.trimCalls) != 0 {
		t.Fatalf("dry run trimmed at %v", backend.trimCalls)
	}
}

func TestRetentionFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		backend *fakeRetentionBackend
		stage   string
	}{
		{name: "group inspection", backend: &fakeRetentionBackend{groupsErr: errors.New("redis down")}, stage: "groups"},
		{name: "no groups", backend: &fakeRetentionBackend{}, stage: "groups"},
		{name: "missing expected group", backend: &fakeRetentionBackend{groups: []retentionGroup{{Name: "other", LastDeliveredID: "1-0"}}}, stage: "groups"},
		{name: "pending inspection", backend: &fakeRetentionBackend{
			groups:     []retentionGroup{{Name: "primary", Pending: 1, LastDeliveredID: "2-0"}},
			pendingErr: map[string]error{"primary": errors.New("pel unavailable")},
		}, stage: "pel"},
		{name: "contradictory pending", backend: &fakeRetentionBackend{
			groups:  []retentionGroup{{Name: "primary", Pending: 1, LastDeliveredID: "2-0"}},
			pending: map[string]retentionPending{"primary": {}},
		}, stage: "frontier"},
		{name: "malformed frontier", backend: &fakeRetentionBackend{
			groups: []retentionGroup{{Name: "primary", LastDeliveredID: "bad"}},
		}, stage: "frontier"},
		{name: "length", backend: &fakeRetentionBackend{
			groups: []retentionGroup{{Name: "primary", LastDeliveredID: "1-0"}}, lengthErr: errors.New("xlen failed"),
		}, stage: "length"},
		{name: "trim", backend: &fakeRetentionBackend{
			groups: []retentionGroup{{Name: "primary", LastDeliveredID: "1-0"}}, trimErr: errors.New("trim failed"),
		}, stage: "trim"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRetentionWithBackend(tt.backend, false)
			_, err := r.RunOnce(context.Background(), StreamSpec{
				Name: "stream", Kind: "events", ExpectedGroups: []string{"primary"},
			})
			var retentionErr *RetentionError
			if !errors.As(err, &retentionErr) || retentionErr.Stage != tt.stage {
				t.Fatalf("error = %v, want RetentionError stage %q", err, tt.stage)
			}
			if tt.stage != "trim" && len(tt.backend.trimCalls) != 0 {
				t.Fatalf("fail-closed path attempted trim: %v", tt.backend.trimCalls)
			}
		})
	}
}

func TestCompareStreamIDs(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1-0", "2-0", -1},
		{"2-0", "2-0", 0},
		{"2-1", "2-0", 1},
		{"10-0", "2-99", 1},
	}
	for _, tt := range tests {
		got, err := compareStreamIDs(tt.a, tt.b)
		if err != nil {
			t.Fatalf("compare %q %q: %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Errorf("compare %q %q = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	if _, err := compareStreamIDs("bad", "1-0"); err == nil {
		t.Fatal("expected malformed ID error")
	}
}
