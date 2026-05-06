package admin

import "time"

const demoNamespace = "demo"

type demoEvent struct {
	SubjectID string
	ObjectID  string
	Action    string
	Weight    float64
	DaysAgo   int
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
	EmbeddingDim:   intPtr(384),
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

func demoOccurredAt(now time.Time, daysAgo int) time.Time {
	return now.Add(-time.Duration(daysAgo) * 24 * time.Hour).UTC()
}

func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }
func stringPtr(v string) *string  { return &v }
