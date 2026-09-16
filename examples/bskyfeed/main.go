package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
	"github.com/jarviisha/codohue/sdk/go/redistream"
)

const (
	// queueCapacity buffers roughly a minute of firehose at full rate, which
	// absorbs a Redis hiccup without stalling the socket read.
	queueCapacity = 20000

	// publishBatch and publishInterval bound one XADD pipeline: flush at
	// whichever comes first so a quiet moment still drains the buffer.
	publishBatch    = 200
	publishInterval = time.Second

	statsInterval = 30 * time.Second

	// shutdownFlush is how long the final drain gets after the context is
	// cancelled, so buffered records are not lost on a normal stop.
	shutdownFlush = 5 * time.Second
)

// validEmbeddingDims mirrors what internal-hashing-ngrams v1 accepts. Checked
// locally so a typo fails at startup rather than as a 422 from the admin API.
var validEmbeddingDims = []int{64, 128, 256, 512}

type config struct {
	redisURL      string
	adminURL      string
	adminKey      string
	namespace     string
	jetstreamURL  string
	samplePercent int
	embeddingDim  int
	bootstrap     bool
}

// stats are the counters reported on the periodic log line.
type stats struct {
	events     atomic.Int64
	items      atomic.Int64
	dropped    atomic.Int64
	failed     atomic.Int64
	reconnects atomic.Int64
}

func main() {
	log.SetFlags(log.Ltime)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	redisOpts, err := redis.ParseURL(cfg.redisURL)
	if err != nil {
		log.Fatalf("REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	if cfg.bootstrap {
		if err := bootstrap(ctx, cfg); err != nil {
			log.Fatalf("bootstrap: %v", err)
		}
	} else {
		log.Printf("bootstrap disabled; assuming namespace %q already exists", cfg.namespace)
	}

	log.Printf("feeding bluesky → ns=%q sample=%d%% (Ctrl-C to stop)", cfg.namespace, cfg.samplePercent)

	var st stats
	queue := make(chan message, queueCapacity)
	go func() {
		defer close(queue)
		readJetstream(ctx, cfg, queue, &st)
	}()

	publish(ctx, cfg, rdb, queue, &st)

	log.Printf("stopped: events=%d items=%d dropped=%d failed=%d reconnects=%d",
		st.events.Load(), st.items.Load(), st.dropped.Load(), st.failed.Load(), st.reconnects.Load())
}

// publish drains the queue into the two Redis streams, batching to keep the
// XADD round-trips proportional to throughput rather than to record count.
// It returns once the queue is closed, after a final flush.
func publish(ctx context.Context, cfg config, rdb redis.UniversalClient, queue <-chan message, st *stats) {
	events := redistream.NewProducer(rdb)
	catalog := redistream.NewCatalogProducer(rdb)

	pendingEvents := make([]codohuetypes.EventPayload, 0, publishBatch)
	pendingItems := make([]codohuetypes.CatalogStreamItem, 0, publishBatch)

	flush := func(ctx context.Context) {
		if len(pendingEvents) > 0 {
			if _, err := events.PublishBatch(ctx, pendingEvents); err != nil {
				st.failed.Add(int64(len(pendingEvents)))
				log.Printf("publish events (%d): %v", len(pendingEvents), err)
			} else {
				st.events.Add(int64(len(pendingEvents)))
			}
			pendingEvents = pendingEvents[:0]
		}
		if len(pendingItems) > 0 {
			if _, err := catalog.PublishBatch(ctx, pendingItems); err != nil {
				st.failed.Add(int64(len(pendingItems)))
				log.Printf("publish catalog (%d): %v", len(pendingItems), err)
			} else {
				st.items.Add(int64(len(pendingItems)))
			}
			pendingItems = pendingItems[:0]
		}
	}

	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()
	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	for {
		select {
		case msg, ok := <-queue:
			if !ok {
				// The context that closed the queue is already cancelled, so
				// the final flush needs an independent deadline to land.
				final, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownFlush)
				flush(final)
				cancel()
				return
			}
			if msg.event != nil {
				pendingEvents = append(pendingEvents, *msg.event)
			}
			if msg.item != nil {
				pendingItems = append(pendingItems, *msg.item)
			}
			if len(pendingEvents) >= publishBatch || len(pendingItems) >= publishBatch {
				flush(ctx)
			}

		case <-ticker.C:
			flush(ctx)

		case <-statsTicker.C:
			log.Printf("stats: events=%d items=%d dropped=%d failed=%d reconnects=%d queue=%d",
				st.events.Load(), st.items.Load(), st.dropped.Load(),
				st.failed.Load(), st.reconnects.Load(), len(queue))
		}
	}
}

func loadConfig() (config, error) {
	cfg := config{
		redisURL:     envOr("REDIS_URL", "redis://localhost:6379"),
		adminURL:     envOr("CODOHUE_ADMIN_URL", "http://localhost:2002"),
		adminKey:     envOr("CODOHUE_ADMIN_API_KEY", "dev-secret-key"),
		namespace:    envOr("CODOHUE_BSKY_NAMESPACE", "bluesky"),
		jetstreamURL: envOr("CODOHUE_BSKY_JETSTREAM_URL", defaultJetstreamURL),
		bootstrap:    envOr("CODOHUE_BSKY_BOOTSTRAP", "true") != "false",
	}

	var err error
	if cfg.samplePercent, err = envInt("CODOHUE_BSKY_SAMPLE_PERCENT", 100); err != nil {
		return cfg, err
	}
	if cfg.samplePercent < 1 || cfg.samplePercent > 100 {
		return cfg, fmt.Errorf("CODOHUE_BSKY_SAMPLE_PERCENT must be 1..100, got %d", cfg.samplePercent)
	}
	if cfg.embeddingDim, err = envInt("CODOHUE_BSKY_EMBEDDING_DIM", 256); err != nil {
		return cfg, err
	}
	if !contains(validEmbeddingDims, cfg.embeddingDim) {
		return cfg, fmt.Errorf("CODOHUE_BSKY_EMBEDDING_DIM must be one of %v, got %d", validEmbeddingDims, cfg.embeddingDim)
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return v, nil
}

func contains(haystack []int, needle int) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
