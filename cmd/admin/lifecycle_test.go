package main

import (
	"context"
	"errors"
	"testing"
)

type fakeLegacyDisabler struct {
	evidence string
	changed  bool
	err      error
	calls    int
}

func (f *fakeLegacyDisabler) DisableLegacyEnvelopes(_ context.Context, evidence string) (bool, error) {
	f.calls++
	f.evidence = evidence
	return f.changed, f.err
}

func TestRunLifecycleCommandRequiresAllAndAdoptionEvidence(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"disable-legacy-envelopes"},
		{"disable-legacy-envelopes", "--all"},
		{"disable-legacy-envelopes", "--adoption-evidence", "release-3"},
	} {
		if _, err := runLifecycleCommand(context.Background(), args, &fakeLegacyDisabler{}); err == nil {
			t.Fatalf("args %v: expected validation error", args)
		}
	}
}

func TestRunLifecycleCommandPassesEvidenceAndIsIdempotent(t *testing.T) {
	fake := &fakeLegacyDisabler{changed: true}
	changed, err := runLifecycleCommand(context.Background(), []string{"disable-legacy-envelopes", "--all", "--adoption-evidence", "deploy/release-3#producers"}, fake)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if fake.calls != 1 || fake.evidence != "deploy/release-3#producers" {
		t.Fatalf("calls=%d evidence=%q", fake.calls, fake.evidence)
	}
	fake.changed = false
	changed, err = runLifecycleCommand(context.Background(), []string{"disable-legacy-envelopes", "--all", "--adoption-evidence", "deploy/release-3#producers"}, fake)
	if err != nil || changed {
		t.Fatalf("second call changed=%v err=%v", changed, err)
	}
}

func TestRunLifecycleCommandPropagatesFailure(t *testing.T) {
	want := errors.New("database unavailable")
	_, err := runLifecycleCommand(context.Background(), []string{"disable-legacy-envelopes", "--all", "--adoption-evidence", "release-3"}, &fakeLegacyDisabler{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
}

func TestDispatchAdminCommandRegistersLifecycle(t *testing.T) {
	serverCalls, lifecycleCalls := 0, 0
	err := dispatchAdminCommand([]string{"lifecycle", "disable-legacy-envelopes"}, func() error {
		serverCalls++
		return nil
	}, func(args []string) error {
		lifecycleCalls++
		if len(args) != 1 || args[0] != "disable-legacy-envelopes" {
			t.Fatalf("forwarded args=%v", args)
		}
		return nil
	}, func([]string) error { return nil })
	if err != nil || serverCalls != 0 || lifecycleCalls != 1 {
		t.Fatalf("err=%v server=%d lifecycle=%d", err, serverCalls, lifecycleCalls)
	}
}
