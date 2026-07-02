package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
	codohue "github.com/jarviisha/codohue/sdk/go"
)

type config struct {
	apiURL    string
	adminURL  string
	namespace string
	adminKey  string
	rate      float64
	users     int
	dim       int
	recsEvery time.Duration
	seed      uint64
	bootstrap bool
}

func main() {
	cfg := parseFlags()
	log.SetFlags(log.Ltime)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sim := newSimulator(cfg.seed, cfg.users)

	// Data-plane SDK client. The global admin key is always accepted as the
	// Bearer token, so the example does not need a per-namespace key.
	client, err := codohue.New(cfg.apiURL)
	if err != nil {
		log.Fatalf("sdk client: %v", err)
	}
	ns := client.Namespace(cfg.namespace, cfg.adminKey)

	if cfg.bootstrap {
		if err := bootstrap(ctx, cfg, ns); err != nil {
			log.Fatalf("bootstrap: %v", err)
		}
	} else {
		log.Printf("skipping bootstrap; assuming namespace %q already exists", cfg.namespace)
	}

	log.Printf("pumping events to %s ns=%q at ~%.1f events/sec (%d users) — Ctrl-C to stop",
		cfg.apiURL, cfg.namespace, cfg.rate, cfg.users)

	runPump(ctx, cfg, ns, sim)
	log.Print("stopped")
}

// bootstrap provisions the namespace through the admin plane and seeds the
// catalog content through the data plane.
func bootstrap(ctx context.Context, cfg config, ns *codohue.Namespace) error {
	admin, err := newAdminClient(cfg.adminURL, cfg.adminKey)
	if err != nil {
		return err
	}
	if err := admin.login(ctx); err != nil {
		return err
	}
	log.Printf("admin session established at %s", cfg.adminURL)

	if err := admin.upsertNamespace(ctx, cfg.namespace, cfg.dim); err != nil {
		return err
	}
	if err := admin.enableCatalog(ctx, cfg.namespace, cfg.dim); err != nil {
		return err
	}
	log.Printf("namespace %q configured (dense=byoe, catalog=internal-hashing-ngrams@v1 dim=%d)", cfg.namespace, cfg.dim)

	seeded := 0
	for _, it := range catalogItems {
		req := codohuetypes.CatalogIngestRequest{
			ObjectID: it.ObjectID,
			Content:  it.Title + ". " + it.Summary,
			Metadata: map[string]any{"category": it.Category, "title": it.Title},
		}
		if err := ns.IngestCatalog(ctx, req); err != nil {
			return err
		}
		seeded++
	}
	log.Printf("seeded %d catalog items (embedder will upsert dense vectors)", seeded)
	return nil
}

// runPump drives the event loop: one event per tick at the configured rate,
// with periodic recommendation read-back and a rolling stats line.
func runPump(ctx context.Context, cfg config, ns *codohue.Namespace, sim *simulator) {
	interval := time.Duration(float64(time.Second) / cfg.rate)
	if interval <= 0 {
		interval = time.Millisecond
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()

	stats := time.NewTicker(5 * time.Second)
	defer stats.Stop()

	recs := time.NewTicker(cfg.recsEvery)
	defer recs.Stop()

	var sent, failed atomic.Int64

	for {
		select {
		case <-ctx.Done():
			log.Printf("totals: sent=%d failed=%d", sent.Load(), failed.Load())
			return

		case <-tick.C:
			ev := sim.next()
			if err := ns.IngestEvent(ctx, ev); err != nil {
				if errors.Is(err, context.Canceled) {
					continue
				}
				failed.Add(1)
				if failed.Load()%50 == 1 {
					log.Printf("ingest error (subject=%s object=%s action=%s): %v",
						ev.SubjectID, ev.ObjectID, ev.Action, err)
				}
				continue
			}
			sent.Add(1)

		case <-stats.C:
			log.Printf("stats: sent=%d failed=%d", sent.Load(), failed.Load())

		case <-recs.C:
			readBack(ctx, ns, sim)
		}
	}
}

// readBack fetches recommendations for a couple of active users and the global
// trending list, so the operator can watch the loop close. Recommendations
// only become non-empty once cmd/cron has run at least one batch.
func readBack(ctx context.Context, ns *codohue.Namespace, sim *simulator) {
	for _, uid := range sim.sampleUserIDs(2) {
		resp, err := ns.Recommend(ctx, uid, codohue.WithLimit(5))
		if err != nil {
			log.Printf("recommend %s: %v", uid, err)
			continue
		}
		log.Printf("recs[%s] source=%s items=%v", uid, resp.Source, topObjectIDs(resp))
	}
	tr, err := ns.Trending(ctx, codohue.WithLimit(5))
	if err != nil {
		log.Printf("trending: %v", err)
		return
	}
	ids := make([]string, 0, len(tr.Items))
	for _, it := range tr.Items {
		ids = append(ids, it.ObjectID)
	}
	log.Printf("trending items=%v", ids)
}

func topObjectIDs(resp *codohuetypes.Response) []string {
	ids := make([]string, 0, len(resp.Items))
	for _, it := range resp.Items {
		ids = append(ids, it.ObjectID)
	}
	return ids
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.apiURL, "api", envOr("CODOHUE_API_URL", "http://localhost:2001"), "cmd/api base URL")
	flag.StringVar(&cfg.adminURL, "admin", envOr("CODOHUE_ADMIN_URL", "http://localhost:2002"), "cmd/admin base URL")
	flag.StringVar(&cfg.namespace, "ns", "loadgen", "namespace to pump data into")
	flag.StringVar(&cfg.adminKey, "admin-key", envOr("CODOHUE_ADMIN_API_KEY", "dev-secret-key"), "global admin API key (also used as data-plane bearer)")
	flag.Float64Var(&cfg.rate, "rate", 5, "target events per second")
	flag.IntVar(&cfg.users, "users", 50, "number of simulated users")
	flag.IntVar(&cfg.dim, "dim", 256, "embedding dimension (one of 64/128/256/512)")
	flag.DurationVar(&cfg.recsEvery, "recs-every", 15*time.Second, "how often to read recommendations/trending back")
	var seed uint64
	flag.Uint64Var(&seed, "seed", 42, "PRNG seed for reproducible runs")
	flag.BoolVar(&cfg.bootstrap, "bootstrap", true, "provision namespace + seed catalog before pumping")
	flag.Parse()
	cfg.seed = seed
	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
