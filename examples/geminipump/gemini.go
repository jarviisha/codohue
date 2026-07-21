package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// geminiClient calls the Google Generative Language REST API to produce catalog
// content. It uses structured output (a response JSON schema) so the model
// returns a parseable array rather than free-form prose.
type geminiClient struct {
	apiKey string
	model  string
	base   string
	http   *http.Client
}

func newGeminiClient(apiKey, model, base string) *geminiClient {
	return &geminiClient{
		apiKey: apiKey,
		model:  model,
		base:   strings.TrimRight(base, "/"),
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// genItem is one catalog item as returned by the model. object_id is only a
// suggestion; the caller re-derives a globally unique ID before ingesting.
type genItem struct {
	ObjectID string `json:"object_id"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Category string `json:"category"`
}

// generateCatalog asks Gemini for n catalog items. hint biases the batch (e.g.
// a rotating category focus) so successive calls do not all look alike.
func (g *geminiClient) generateCatalog(ctx context.Context, n int, categories []string, hint string) ([]genItem, error) {
	prompt := fmt.Sprintf(`You are generating seed content for a content-recommendation platform.

Produce exactly %d distinct, realistic pieces of content. For each item return:
- object_id: a short lowercase slug (letters, digits, underscores), unique within this batch
- title: a concise, specific headline (max ~10 words)
- summary: one or two sentences describing the content
- category: a single lowercase word

Prefer these categories so items cluster into coherent topics: %s.
Spread the items across several of them. Make titles specific and varied — no
duplicates, no numbered "Item 1" placeholders. %s`,
		n, strings.Join(categories, ", "), hint)

	itemSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"object_id": map[string]any{"type": "string"},
			"title":     map[string]any{"type": "string"},
			"summary":   map[string]any{"type": "string"},
			"category":  map[string]any{"type": "string"},
		},
		"required":         []string{"object_id", "title", "summary", "category"},
		"propertyOrdering": []string{"object_id", "title", "summary", "category"},
	}

	reqBody := map[string]any{
		"contents": []any{
			map[string]any{"parts": []any{map[string]any{"text": prompt}}},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"responseSchema":   map[string]any{"type": "array", "items": itemSchema},
			"temperature":      1.1,
		},
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", g.base, g.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey) // header form keeps the key out of URLs/logs

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini status %d: %s", resp.StatusCode, truncate(string(body), 400))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("gemini decode envelope: %w", err)
	}
	if parsed.PromptFeedback.BlockReason != "" {
		return nil, fmt.Errorf("gemini blocked prompt: %s", parsed.PromptFeedback.BlockReason)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		reason := ""
		if len(parsed.Candidates) > 0 {
			reason = parsed.Candidates[0].FinishReason
		}
		return nil, fmt.Errorf("gemini returned no content (finish=%q)", reason)
	}

	text := parsed.Candidates[0].Content.Parts[0].Text
	var items []genItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return nil, fmt.Errorf("gemini decode items: %w (payload: %s)", err, truncate(text, 300))
	}
	return items, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
