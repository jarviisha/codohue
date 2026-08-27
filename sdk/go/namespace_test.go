package codohue

import "testing"

func TestNamespaceGenerationIsAdditiveAndOptional(t *testing.T) {
	client, err := New("http://example.test")
	if err != nil {
		t.Fatal(err)
	}
	legacy := client.Namespace("feed", "key")
	if legacy.Generation() != 0 {
		t.Fatalf("legacy generation = %d", legacy.Generation())
	}
	qualified := client.NamespaceWithOptions("feed", "key", WithNamespaceGeneration(3))
	if qualified.Name() != "feed" || qualified.Generation() != 3 {
		t.Fatalf("qualified namespace = %q generation=%d", qualified.Name(), qualified.Generation())
	}
}

func TestWithNamespaceGenerationIgnoresInvalidValues(t *testing.T) {
	client, _ := New("http://example.test")
	namespace := client.NamespaceWithOptions("feed", "key", WithNamespaceGeneration(-1))
	if namespace.Generation() != 0 {
		t.Fatalf("generation = %d", namespace.Generation())
	}
}
