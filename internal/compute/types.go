package compute

// SubjectVector represents the sparse interaction vector of a subject.
type SubjectVector struct {
	SubjectID string
	NumericID uint64
	Indices   []uint32
	Values    []float32
}

// ObjectVector represents the sparse interaction vector of an object.
type ObjectVector struct {
	ObjectID  string
	NumericID uint64
	Indices   []uint32
	Values    []float32
}

// RawEvent is an event record fetched from Postgres for vector computation.
type RawEvent struct {
	SubjectID       string
	ObjectID        string
	Action          string
	Weight          float64
	OccurredAt      int64  // Unix timestamp
	ObjectCreatedAt *int64 // Unix timestamp, nil when not provided by the event source
}

// DenseVector holds the dense embedding for an item or subject.
type DenseVector struct {
	ID        string
	NumericID uint64
	Vector    []float32
}

// InteractionSequence is the time-ordered list of objects a subject has interacted
// with. Used as a "sentence" in Item2Vec skip-gram training.
type InteractionSequence struct {
	SubjectID string
	ObjectIDs []string // sorted by occurred_at ascending
}

// Item2VecConfig holds the skip-gram training hyperparameters.
type Item2VecConfig struct {
	Dim        int // output embedding dimension
	Window     int // context window size (items on each side)
	MinCount   int // discard items with fewer interactions than this
	Epochs     int // training epochs
	NegSamples int // negative samples per positive (target, context) pair
}
