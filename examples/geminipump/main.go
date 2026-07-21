package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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
	apiURL       string
	adminURL     string
	namespace    string
	adminKey     string
	geminiKey    string
	geminiModel  string
	geminiBase   string
	rate         float64
	users        int
	dim          int
	catalogBatch int
	genEvery     time.Duration
	maxItems     int
	recsEvery    time.Duration
	seed         uint64
	bootstrap    bool
}

// seedCategories biases Gemini toward a stable topic set so user affinities stay
// coherent as the catalog grows. The model may still invent new categories; the
// simulator assigns affinities to those lazily.
var seedCategories = []string{
	"tech", "science", "sports", "cooking", "travel",
	"finance", "gaming", "music", "health", "art",
}

func main() {
	cfg := parseFlags()
	log.SetFlags(log.Ltime)

	if cfg.geminiKey == "" {
		log.Fatal("missing Gemini API key: set GEMINI_API_KEY or pass -gemini-key")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := newCatalogStore()
	sim := newSimulator(cfg.seed, cfg.users, store)
	gem := newGeminiClient(cfg.geminiKey, cfg.geminiModel, cfg.geminiBase)

	// Data-plane SDK client. The global admin key is always accepted as the
	// Bearer token, so the example does not need a per-namespace key.
	client, err := codohue.New(cfg.apiURL)
	if err != nil {
		log.Fatalf("sdk client: %v", err)
	}
	ns := client.Namespace(cfg.namespace, cfg.adminKey)

	if cfg.bootstrap {
		if err := bootstrap(ctx, cfg, gem, ns, store); err != nil {
			log.Fatalf("bootstrap: %v", err)
		}
	} else {
		log.Printf("skipping namespace provisioning; seeding an initial Gemini batch")
		if err := generateAndSeed(ctx, cfg, gem, ns, store, 0); err != nil {
			log.Fatalf("initial generation: %v", err)
		}
	}

	log.Printf("pumping events to %s ns=%q at ~%.1f events/sec (%d users); generating %d catalog items every %s (cap %d) via gemini %s",
		cfg.apiURL, cfg.namespace, cfg.rate, cfg.users, cfg.catalogBatch, cfg.genEvery, cfg.maxItems, cfg.geminiModel)

	go runGenerator(ctx, cfg, gem, ns, store)
	runPump(ctx, cfg, ns, sim, store)
	log.Print("stopped")
}

// bootstrap provisions the namespace through the admin plane, then seeds the
// first Gemini-generated catalog batch so the pump has content to work with.
func bootstrap(ctx context.Context, cfg config, gem *geminiClient, ns *codohue.Namespace, store *catalogStore) error {
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

	return generateAndSeed(ctx, cfg, gem, ns, store, 0)
}

// generateAndSeed asks Gemini for one batch, registers the new items in the
// store, and ingests them through the data plane. batch numbers the call so
// object IDs stay unique across the run.
func generateAndSeed(ctx context.Context, cfg config, gem *geminiClient, ns *codohue.Namespace, store *catalogStore, batch int) error {
	hint := fmt.Sprintf("This is batch #%d — pick a fresh angle so it does not repeat earlier batches.", batch)
	gen, err := gem.generateCatalog(ctx, cfg.catalogBatch, seedCategories, hint)
	if err != nil {
		return err
	}

	added := store.add(toCatalogItems(batch, gen, time.Now().UTC()))
	if len(added) == 0 {
		log.Printf("gemini batch #%d produced no new items", batch)
		return nil
	}

	seeded := 0
	for _, it := range added {
		req := codohuetypes.CatalogIngestRequest{
			ObjectID: it.ObjectID,
			Content:  it.Title + ". " + it.Summary,
			Metadata: map[string]any{"category": it.Category, "title": it.Title},
		}
		if err := ns.IngestCatalog(ctx, req); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			log.Printf("catalog ingest %s: %v", it.ObjectID, err)
			continue
		}
		seeded++
	}
	log.Printf("gemini batch #%d: seeded %d/%d new items (catalog size=%d)", batch, seeded, len(added), store.size())
	return nil
}

// runGenerator periodically tops up the catalog with fresh Gemini content until
// the store reaches the configured cap.
func runGenerator(ctx context.Context, cfg config, gem *geminiClient, ns *codohue.Namespace, store *catalogStore) {
	tick := time.NewTicker(cfg.genEvery)
	defer tick.Stop()

	batch := 1
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if store.size() >= cfg.maxItems {
				continue
			}
			if err := generateAndSeed(ctx, cfg, gem, ns, store, batch); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("gemini generate (batch #%d): %v", batch, err)
				continue
			}
			batch++
		}
	}
}

// runPump drives the event loop: one event per tick at the configured rate,
// with periodic recommendation read-back and a rolling stats line.
func runPump(ctx context.Context, cfg config, ns *codohue.Namespace, sim *simulator, store *catalogStore) {
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
			log.Printf("totals: sent=%d failed=%d catalog=%d", sent.Load(), failed.Load(), store.size())
			return

		case <-tick.C:
			ev, ok := sim.next()
			if !ok {
				continue // catalog still empty; wait for the first batch
			}
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
			log.Printf("stats: sent=%d failed=%d catalog=%d", sent.Load(), failed.Load(), store.size())

		case <-recs.C:
			readBack(ctx, ns, sim)
		}
	}
}

// readBack fetches recommendations for a couple of active users and the global
// trending list, so the operator can watch the loop close. Recommendations only
// become non-empty once cmd/cron has run at least one batch.
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
	flag.StringVar(&cfg.namespace, "ns", "geminipump", "namespace to pump data into")
	flag.StringVar(&cfg.adminKey, "admin-key", envOr("CODOHUE_ADMIN_API_KEY", "dev-secret-key"), "global admin API key (also used as data-plane bearer)")
	flag.StringVar(&cfg.geminiKey, "gemini-key", os.Getenv("GEMINI_API_KEY"), "Google Gemini API key")
	flag.StringVar(&cfg.geminiModel, "gemini-model", envOr("GEMINI_MODEL", "gemini-2.5-flash"), "Gemini model id")
	flag.StringVar(&cfg.geminiBase, "gemini-base", envOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"), "Gemini API base URL")
	flag.Float64Var(&cfg.rate, "rate", 5, "target events per second")
	flag.IntVar(&cfg.users, "users", 50, "number of simulated users")
	flag.IntVar(&cfg.dim, "dim", 256, "embedding dimension (one of 64/128/256/512)")
	flag.IntVar(&cfg.catalogBatch, "catalog-batch", 12, "catalog items to request from Gemini per generation")
	flag.DurationVar(&cfg.genEvery, "gen-every", 2*time.Minute, "how often to generate a fresh catalog batch")
	flag.IntVar(&cfg.maxItems, "max-items", 300, "stop generating once the catalog reaches this size")
	flag.DurationVar(&cfg.recsEvery, "recs-every", 15*time.Second, "how often to read recommendations/trending back")
	var seed uint64
	flag.Uint64Var(&seed, "seed", 42, "PRNG seed for reproducible event patterns")
	flag.BoolVar(&cfg.bootstrap, "bootstrap", true, "provision namespace + enable catalog before pumping")
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
