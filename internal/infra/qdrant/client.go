package qdrant

import (
	"fmt"

	"github.com/qdrant/go-client/qdrant"
)

var newQdrantClientFn = qdrant.NewClient

// NewClient creates a Qdrant gRPC client connected to the given host:port.
func NewClient(host string, port int) (*qdrant.Client, error) {
	client, err := newQdrantClientFn(&qdrant.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("create qdrant client: %w", err)
	}

	return client, nil
}
