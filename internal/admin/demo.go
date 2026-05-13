package admin

import (
	"crypto/sha256"
	"time"
)

const demoNamespace = "demo"

// demoCatalogStrategyDim must match one of the dims registered by
// internal/embedder for "internal-hashing-ngrams@v1". Keeping it at 256
// gives a healthy bucket count without blowing up Qdrant payload size.
const (
	demoCatalogStrategyID      = "internal-hashing-ngrams"
	demoCatalogStrategyVersion = "v1"
	demoCatalogStrategyDim     = 256
)

type demoEvent struct {
	SubjectID string
	ObjectID  string
	Action    string
	Weight    float64
	DaysAgo   int
}

// demoCatalogItem is one row that will be seeded into catalog_items for the
// demo namespace. Content is short, human-readable, and aligned with the
// object_ids referenced by demoDataset so the catalog browse page and the
// event-driven recommendations refer to the same logical products.
type demoCatalogItem struct {
	ObjectID string
	Content  string
	Metadata map[string]any
}

var demoNamespaceConfig = NamespaceUpsertRequest{
	ActionWeights: map[string]float64{
		"VIEW":     1,
		"LIKE":     3,
		"CART":     4,
		"PURCHASE": 8,
	},
	Lambda:         floatPtr(0.92),
	Gamma:          floatPtr(0.12),
	Alpha:          floatPtr(0.65),
	MaxResults:     intPtr(20),
	SeenItemsDays:  intPtr(30),
	DenseStrategy:  stringPtr("disabled"),
	EmbeddingDim:   intPtr(demoCatalogStrategyDim),
	DenseDistance:  stringPtr("cosine"),
	TrendingWindow: intPtr(72),
	TrendingTTL:    intPtr(3600),
	LambdaTrending: floatPtr(0.18),
}

var demoDataset = []demoEvent{
	{SubjectID: "u_ava", ObjectID: "item_wireless_keyboard", Action: "VIEW", Weight: 1, DaysAgo: 6},
	{SubjectID: "u_ava", ObjectID: "item_wireless_keyboard", Action: "LIKE", Weight: 3, DaysAgo: 5},
	{SubjectID: "u_ava", ObjectID: "item_usb_c_hub", Action: "VIEW", Weight: 1, DaysAgo: 4},
	{SubjectID: "u_ava", ObjectID: "item_usb_c_hub", Action: "CART", Weight: 4, DaysAgo: 3},
	{SubjectID: "u_ava", ObjectID: "item_standing_desk_mat", Action: "PURCHASE", Weight: 8, DaysAgo: 2},

	{SubjectID: "u_ben", ObjectID: "item_usb_c_hub", Action: "VIEW", Weight: 1, DaysAgo: 7},
	{SubjectID: "u_ben", ObjectID: "item_laptop_stand", Action: "LIKE", Weight: 3, DaysAgo: 5},
	{SubjectID: "u_ben", ObjectID: "item_noise_canceling_headphones", Action: "CART", Weight: 4, DaysAgo: 2},
	{SubjectID: "u_ben", ObjectID: "item_wireless_mouse", Action: "PURCHASE", Weight: 8, DaysAgo: 1},

	{SubjectID: "u_chloe", ObjectID: "item_standing_desk_mat", Action: "VIEW", Weight: 1, DaysAgo: 8},
	{SubjectID: "u_chloe", ObjectID: "item_laptop_stand", Action: "VIEW", Weight: 1, DaysAgo: 6},
	{SubjectID: "u_chloe", ObjectID: "item_laptop_stand", Action: "LIKE", Weight: 3, DaysAgo: 5},
	{SubjectID: "u_chloe", ObjectID: "item_desk_lamp", Action: "CART", Weight: 4, DaysAgo: 2},

	{SubjectID: "u_diego", ObjectID: "item_noise_canceling_headphones", Action: "VIEW", Weight: 1, DaysAgo: 7},
	{SubjectID: "u_diego", ObjectID: "item_noise_canceling_headphones", Action: "PURCHASE", Weight: 8, DaysAgo: 3},
	{SubjectID: "u_diego", ObjectID: "item_monitor_arm", Action: "VIEW", Weight: 1, DaysAgo: 2},
	{SubjectID: "u_diego", ObjectID: "item_monitor_arm", Action: "LIKE", Weight: 3, DaysAgo: 1},

	{SubjectID: "u_emma", ObjectID: "item_wireless_mouse", Action: "VIEW", Weight: 1, DaysAgo: 6},
	{SubjectID: "u_emma", ObjectID: "item_desk_lamp", Action: "VIEW", Weight: 1, DaysAgo: 4},
	{SubjectID: "u_emma", ObjectID: "item_desk_lamp", Action: "LIKE", Weight: 3, DaysAgo: 3},
	{SubjectID: "u_emma", ObjectID: "item_usb_c_hub", Action: "PURCHASE", Weight: 8, DaysAgo: 1},

	{SubjectID: "u_finn", ObjectID: "item_laptop_stand", Action: "VIEW", Weight: 1, DaysAgo: 9},
	{SubjectID: "u_finn", ObjectID: "item_monitor_arm", Action: "CART", Weight: 4, DaysAgo: 4},
	{SubjectID: "u_finn", ObjectID: "item_standing_desk_mat", Action: "LIKE", Weight: 3, DaysAgo: 3},
	{SubjectID: "u_finn", ObjectID: "item_wireless_keyboard", Action: "PURCHASE", Weight: 8, DaysAgo: 1},
}

// demoCatalogDataset is the bundled catalog content for the demo namespace.
// Each entry corresponds to an object_id referenced by demoDataset so the
// admin catalog browse page lines up with the interaction events. Content
// is short and product-like to make the embeddings non-trivial without
// approaching the default 32 KiB cap.
var demoCatalogDataset = []demoCatalogItem{
	{
		ObjectID: "item_wireless_keyboard",
		Content:  "Slim wireless mechanical keyboard with low-profile switches, multi-device Bluetooth pairing, and a backlit white aluminum chassis. Optimized for long typing sessions.",
		Metadata: map[string]any{"category": "peripherals", "price_tier": "mid"},
	},
	{
		ObjectID: "item_usb_c_hub",
		Content:  "Compact USB-C hub with HDMI 4K@60Hz, two USB-A 3.2 ports, SD/microSD card readers, and 100W power delivery passthrough. Plug-and-play, no drivers required.",
		Metadata: map[string]any{"category": "accessories", "price_tier": "budget"},
	},
	{
		ObjectID: "item_standing_desk_mat",
		Content:  "Ergonomic anti-fatigue standing desk mat with contoured terrain points. Encourages micro-movements while standing, reducing pressure on heels and lower back.",
		Metadata: map[string]any{"category": "ergonomics", "price_tier": "mid"},
	},
	{
		ObjectID: "item_laptop_stand",
		Content:  "Adjustable aluminum laptop stand that elevates the screen to eye level. Foldable, portable, and stable up to 17-inch laptops. Vents allow airflow to reduce thermal throttling.",
		Metadata: map[string]any{"category": "ergonomics", "price_tier": "budget"},
	},
	{
		ObjectID: "item_noise_canceling_headphones",
		Content:  "Over-ear wireless headphones with hybrid active noise canceling, transparency mode, multi-point Bluetooth, and 40-hour battery life. Tuned for both focused work and music listening.",
		Metadata: map[string]any{"category": "audio", "price_tier": "premium"},
	},
	{
		ObjectID: "item_wireless_mouse",
		Content:  "Lightweight wireless ergonomic mouse with high-DPI optical sensor, side scroll wheel, programmable side buttons, and USB-C fast charging. Bluetooth + 2.4GHz dongle.",
		Metadata: map[string]any{"category": "peripherals", "price_tier": "mid"},
	},
	{
		ObjectID: "item_desk_lamp",
		Content:  "Smart LED desk lamp with adjustable color temperature, brightness presets, and a flicker-free panel that protects the eyes during long work sessions. Touch controls and USB-C charging.",
		Metadata: map[string]any{"category": "lighting", "price_tier": "mid"},
	},
	{
		ObjectID: "item_monitor_arm",
		Content:  "Heavy-duty gas-spring monitor arm with VESA 75x75 / 100x100, full motion tilt, swivel, and rotation. Supports 17 to 32 inch monitors up to 9kg, integrated cable management.",
		Metadata: map[string]any{"category": "ergonomics", "price_tier": "premium"},
	},
}

func demoOccurredAt(now time.Time, daysAgo int) time.Time {
	return now.Add(-time.Duration(daysAgo) * 24 * time.Hour).UTC()
}

// demoContentHash mirrors internal/catalog.ContentHash so the seeded rows
// carry the same canonical hash an ingest from the data plane would produce.
// Re-declared here because internal/admin cannot import internal/catalog
// (peer-domain import; enforced by architecture/imports_test.go).
func demoContentHash(content string) []byte {
	sum := sha256.Sum256([]byte(content))
	return sum[:]
}

func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }
func stringPtr(v string) *string  { return &v }
