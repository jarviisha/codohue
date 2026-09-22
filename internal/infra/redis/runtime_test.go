package redis

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

type runtimeHook struct{ process func(goredis.Cmder) error }

func (h runtimeHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h runtimeHook) ProcessHook(_ goredis.ProcessHook) goredis.ProcessHook {
	return func(_ context.Context, cmd goredis.Cmder) error { return h.process(cmd) }
}
func (h runtimeHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func TestRuntimeReporterAndReader(t *testing.T) {
	published := make(chan []any, 1)
	client := goredis.NewClient(&goredis.Options{Addr: "unused"})
	defer client.Close()
	client.AddHook(runtimeHook{process: func(cmd goredis.Cmder) error { published <- cmd.Args(); return nil }})
	stop := StartRuntimeReporter(context.Background(), client, "cron", []config.RuntimeSetting{{Name: "batch_interval_minutes", Value: 5}})
	defer stop()
	select {
	case args := <-published:
		if args[0] != "set" || !strings.HasPrefix(args[1].(string), runtimePrefix+"cron:") {
			t.Fatalf("unexpected publish: %v", args)
		}
		if args[len(args)-2] != "ex" || args[len(args)-1] != int64(120) {
			t.Fatalf("missing expiry: %v", args)
		}
		var report config.RuntimeSnapshot
		if err := json.Unmarshal(args[2].([]byte), &report); err != nil {
			t.Fatal(err)
		}
		if report.Process != "cron" || report.ReportedAt.IsZero() || report.StartedAt.IsZero() {
			t.Fatalf("incomplete report: %+v", report)
		}
	case <-time.After(time.Second):
		t.Fatal("report not published")
	}
	reader := goredis.NewClient(&goredis.Options{Addr: "unused"})
	defer reader.Close()
	reader.AddHook(runtimeHook{process: func(cmd goredis.Cmder) error {
		switch c := cmd.(type) {
		case *goredis.ScanCmd:
			c.SetVal([]string{"expired", "alive", "alive"}, 0)
		case *goredis.StringCmd:
			if c.Args()[1] == "expired" {
				return goredis.Nil
			}
			c.SetVal(`{"process":"admin","settings":[]}`)
		}
		return nil
	}})
	reports, err := RuntimeSnapshots(context.Background(), reader)
	if err != nil || len(reports) != 1 || reports[0].Process != "admin" {
		t.Fatalf("reports=%+v err=%v", reports, err)
	}
}

func TestRuntimeReadFailureIsNotEmptySuccess(t *testing.T) {
	client := goredis.NewClient(&goredis.Options{Addr: "unused"})
	defer client.Close()
	client.AddHook(runtimeHook{process: func(goredis.Cmder) error { return errors.New("unavailable") }})
	if _, err := RuntimeSnapshots(context.Background(), client); err == nil {
		t.Fatal("missing read error")
	}
	if _, err := RuntimeSnapshots(context.Background(), nil); err == nil {
		t.Fatal("nil client must be unavailable")
	}
}
