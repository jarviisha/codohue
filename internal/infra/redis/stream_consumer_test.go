package redis

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func TestStreamConsumerInit(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "created"},
		{name: "already exists", err: errors.New("BUSYGROUP exists")},
		{name: "failure", err: errors.New("redis down"), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			consumer := NewStreamConsumer(StreamConsumerConfig{Stream: "s", Group: "g"}, StreamConsumerFuncs{
				CreateGroup: func(context.Context, string, string, string) error { return tc.err },
			}, nil, StreamConsumerObserver{})
			if got := consumer.Init(context.Background()); (got != nil) != tc.want {
				t.Fatalf("Init() error = %v", got)
			}
		})
	}
}

func TestStreamConsumerReapStateMachine(t *testing.T) {
	t.Run("advances empty pages and resets at terminal", func(t *testing.T) {
		var starts []string
		var outcomes []ReclaimOutcome
		var handled []string
		reclaimed := 0
		consumer := testStreamConsumer(func(_ context.Context, args *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error) {
			starts = append(starts, args.Start)
			if len(starts) == 1 {
				return []goredis.XMessage{{ID: "1-0"}}, "100-0", nil
			}
			return nil, "0-0", nil
		}, func(outcome ReclaimOutcome) { outcomes = append(outcomes, outcome) })
		consumer.handle = func(_ context.Context, msg goredis.XMessage) { handled = append(handled, msg.ID) }
		consumer.observe.Reclaimed = func(count int) { reclaimed += count }

		if got := consumer.ReapOnce(context.Background(), ""); got != "0-0" {
			t.Fatalf("cursor = %q", got)
		}
		if fmt.Sprint(starts) != "[0-0 100-0]" || fmt.Sprint(outcomes) != "[terminal]" || fmt.Sprint(handled) != "[1-0]" || reclaimed != 1 {
			t.Fatalf("starts=%v outcomes=%v handled=%v reclaimed=%d", starts, outcomes, handled, reclaimed)
		}
	})

	t.Run("retains cursor at budget", func(t *testing.T) {
		calls := 0
		consumer := testStreamConsumer(func(context.Context, *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error) {
			calls++
			return nil, fmt.Sprintf("%d-0", calls), nil
		}, nil)
		if got := consumer.ReapOnce(context.Background(), "700-0"); got != "3-0" || calls != 3 {
			t.Fatalf("cursor=%q calls=%d", got, calls)
		}
	})

	t.Run("retains transient error and resets nogroup", func(t *testing.T) {
		transientErr := errors.New("redis down")
		claimErr := transientErr
		var observed []error
		var outcomes []ReclaimOutcome
		consumer := testStreamConsumer(func(context.Context, *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error) {
			return nil, "", claimErr
		}, func(outcome ReclaimOutcome) { outcomes = append(outcomes, outcome) })
		consumer.observe.ReclaimError = func(_ context.Context, err error) { observed = append(observed, err) }
		if got := consumer.ReapOnce(context.Background(), "700-0"); got != "700-0" {
			t.Fatalf("transient cursor=%q", got)
		}
		claimErr = errors.New("NOGROUP missing")
		if got := consumer.ReapOnce(context.Background(), "700-0"); got != "0-0" {
			t.Fatalf("nogroup cursor=%q", got)
		}
		if len(observed) != 1 || !errors.Is(observed[0], transientErr) || fmt.Sprint(outcomes) != "[error nogroup]" {
			t.Fatalf("observed=%v outcomes=%v", observed, outcomes)
		}
	})
}

func TestStreamConsumerRunRecreatesMissingGroupAndHandlesMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads := 0
	creates := 0
	var handled []string
	consumer := NewStreamConsumer(StreamConsumerConfig{
		Stream: "s", Group: "g", Consumer: "c", ReadCount: 2,
		ReadBlock: time.Millisecond, ReapInterval: time.Hour,
		ReadBackoffMin: time.Millisecond, ReadBackoffMax: time.Millisecond,
	}, StreamConsumerFuncs{
		CreateGroup: func(context.Context, string, string, string) error { creates++; return nil },
		ReadGroup: func(context.Context, *goredis.XReadGroupArgs) ([]goredis.XStream, error) {
			reads++
			if reads == 1 {
				return nil, errors.New("NOGROUP missing")
			}
			cancel()
			return []goredis.XStream{{Messages: []goredis.XMessage{{ID: "1-0"}}}}, nil
		},
		AutoClaim: func(context.Context, *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error) {
			return nil, "0-0", nil
		},
	}, func(_ context.Context, msg goredis.XMessage) { handled = append(handled, msg.ID) }, StreamConsumerObserver{})

	consumer.Run(ctx)
	if creates != 1 || fmt.Sprint(handled) != "[1-0]" {
		t.Fatalf("creates=%d handled=%v", creates, handled)
	}
}

func TestStreamConsumerRunObservesReadAndRecreateErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads := 0
	recreateErrors := 0
	readErrors := 0
	consumer := NewStreamConsumer(StreamConsumerConfig{
		Stream: "s", Group: "g", Consumer: "c", ReadCount: 1,
		ReadBlock: time.Millisecond, ReapInterval: time.Hour,
		ReadBackoffMin: time.Millisecond, ReadBackoffMax: 2 * time.Millisecond,
	}, StreamConsumerFuncs{
		CreateGroup: func(context.Context, string, string, string) error { return errors.New("redis down") },
		ReadGroup: func(context.Context, *goredis.XReadGroupArgs) ([]goredis.XStream, error) {
			reads++
			switch reads {
			case 1:
				return nil, goredis.Nil
			case 2:
				return nil, errors.New("NOGROUP missing")
			case 3:
				return nil, errors.New("read failed")
			default:
				cancel()
				return nil, context.Canceled
			}
		},
		AutoClaim: func(context.Context, *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error) {
			return nil, "0-0", nil
		},
	}, func(context.Context, goredis.XMessage) {}, StreamConsumerObserver{
		ReadError:     func(context.Context, error) { readErrors++ },
		RecreateError: func(context.Context, error) { recreateErrors++ },
	})

	consumer.Run(ctx)
	if reads != 4 || readErrors != 2 || recreateErrors != 1 {
		t.Fatalf("reads=%d read_errors=%d recreate_errors=%d", reads, readErrors, recreateErrors)
	}
}

func TestStreamConsumerReapLoopRunsOnTicker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	consumer := NewStreamConsumer(StreamConsumerConfig{
		Stream: "s", Group: "g", Consumer: "c",
		ReapInterval: time.Millisecond, ReapCount: 1, ReapPageBudget: 1,
	}, StreamConsumerFuncs{
		AutoClaim: func(context.Context, *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error) {
			cancel()
			return nil, "0-0", nil
		},
	}, func(context.Context, goredis.XMessage) {}, StreamConsumerObserver{})
	go func() {
		consumer.reapLoop(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reap loop did not stop after its tick")
	}
}

func testStreamConsumer(
	autoClaim func(context.Context, *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error),
	completed func(ReclaimOutcome),
) *StreamConsumer {
	return NewStreamConsumer(StreamConsumerConfig{
		Stream: "s", Group: "g", Consumer: "c", ReapCount: 10, ReapPageBudget: 3,
	}, StreamConsumerFuncs{AutoClaim: autoClaim}, func(context.Context, goredis.XMessage) {}, StreamConsumerObserver{
		ReclaimCompleted: completed,
	})
}
