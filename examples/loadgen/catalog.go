package main

// catalogItem is one piece of content in the simulated platform. Title and
// Summary are concatenated into the catalog Content that feeds the embedder;
// Category drives each simulated user's affinity so the events we emit form a
// learnable signal rather than uniform noise.
type catalogItem struct {
	ObjectID string
	Title    string
	Summary  string
	Category string
}

// catalogItems is a small content catalog spread across a handful of
// categories. Three items per category gives the recommender clear
// co-occurrence structure to pick up on.
var catalogItems = []catalogItem{
	// tech
	{"art_rust_async", "Understanding async Rust", "How futures, executors, and pinning fit together in the Rust async runtime.", "tech"},
	{"art_k8s_pitfalls", "Kubernetes pitfalls in production", "Resource limits, liveness probes, and the failure modes teams hit at scale.", "tech"},
	{"art_postgres_index", "Indexing strategies in Postgres", "When B-tree, GIN, and BRIN indexes pay off and when they quietly hurt.", "tech"},

	// science
	{"art_webb_images", "What the Webb telescope revealed", "Early-universe galaxies and the infrared images reshaping cosmology.", "science"},
	{"art_crispr_ethics", "The ethics of CRISPR", "Germline editing, consent, and where the regulatory lines are drawn.", "science"},
	{"art_fusion_net", "Fusion finally hit net energy", "Inside the ignition milestone and the long road still left to a power plant.", "science"},

	// sports
	{"art_marathon_pace", "Pacing a first marathon", "Negative splits, fueling windows, and the wall most beginners hit at 30km.", "sports"},
	{"art_offside_var", "How VAR changed offside", "Semi-automated tracking and the millimetre calls that divide fans.", "sports"},
	{"art_nba_spacing", "Why spacing won the NBA", "The three-point revolution and the death of the mid-range jumper.", "sports"},

	// cooking
	{"art_sourdough", "A reliable sourdough starter", "Hydration, feeding ratios, and reading the float test before you bake.", "cooking"},
	{"art_wok_hei", "Chasing wok hei at home", "Why home stoves struggle and the tricks that get you most of the way there.", "cooking"},
	{"art_knife_skills", "Knife skills that matter", "The five cuts worth drilling and how to keep an edge between sharpenings.", "cooking"},

	// travel
	{"art_japan_rail", "Japan by rail in two weeks", "Routing the JR Pass, reserving seats, and the towns worth the detour.", "travel"},
	{"art_patagonia_w", "Trekking the Patagonia W", "Permits, weather windows, and packing for four seasons in one day.", "travel"},
	{"art_lisbon_food", "Eating your way through Lisbon", "Tascas, pastéis, and the neighbourhoods locals actually eat in.", "travel"},

	// finance
	{"art_index_funds", "The case for index funds", "Fees, compounding, and why most active managers lose to the market.", "finance"},
	{"art_bond_ladder", "Building a bond ladder", "Laddering maturities for income without betting on interest rates.", "finance"},
	{"art_tax_loss", "Tax-loss harvesting basics", "Offsetting gains, the wash-sale rule, and when it is not worth the effort.", "finance"},

	// gaming
	{"art_roguelike", "Why roguelikes endure", "Permadeath, procedural runs, and the loop that keeps players coming back.", "gaming"},
	{"art_speedrun", "The craft of speedrunning", "Frame-perfect tricks, routing, and the communities behind world records.", "gaming"},
	{"art_indie_econ", "The economics of indie games", "Wishlists, launch windows, and surviving the post-release cliff.", "gaming"},

	// music
	{"art_jazz_modes", "Modal jazz, explained", "How Kind of Blue traded chord changes for modes and opened the music up.", "music"},
	{"art_synth_basics", "Subtractive synthesis basics", "Oscillators, filters, and envelopes — building a patch from scratch.", "music"},
	{"art_vinyl_revival", "The vinyl revival", "Why physical records outsell CDs again and what listeners are chasing.", "music"},
}

// categories returns the distinct category names in catalogItems, in first-seen
// order, so the simulator can build per-user affinity vectors over them.
func categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range catalogItems {
		if !seen[it.Category] {
			seen[it.Category] = true
			out = append(out, it.Category)
		}
	}
	return out
}
