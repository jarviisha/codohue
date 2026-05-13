package embedder

import (
	"strings"
	"testing"
)

func toStrings(features [][]byte) []string {
	out := make([]string, len(features))
	for i, f := range features {
		out[i] = string(f)
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func count(haystack []string, needle string) int {
	n := 0
	for _, s := range haystack {
		if s == needle {
			n++
		}
	}
	return n
}

func TestTokenize_EmptyAndWhitespaceOnly(t *testing.T) {
	cases := []string{"", "   ", "\t\n  \r"}
	for _, c := range cases {
		c := c
		t.Run(strings.Trim(c, " \t\n\r"), func(t *testing.T) {
			if got := Tokenize(c); got != nil {
				t.Fatalf("Tokenize(%q) = %v, want nil", c, toStrings(got))
			}
		})
	}
}

func TestTokenize_PunctuationOnlyReturnsNil(t *testing.T) {
	if got := Tokenize("!!! ??? ..."); got != nil {
		t.Fatalf("expected nil for punctuation-only input, got %v", toStrings(got))
	}
}

func TestTokenize_SimpleEnglish_LowercasesAndSplits(t *testing.T) {
	got := toStrings(Tokenize("Hello WORLD foo"))
	for _, want := range []string{"hello", "world", "foo"} {
		if !contains(got, want) {
			t.Errorf("missing word feature %q in %v", want, got)
		}
	}
	if contains(got, "Hello") {
		t.Errorf("uppercase word leaked: %v", got)
	}
}

func TestTokenize_StripsTrailingPunctuation(t *testing.T) {
	got := toStrings(Tokenize("hello, world! foo."))
	for _, want := range []string{"hello", "world", "foo"} {
		if !contains(got, want) {
			t.Errorf("missing word feature %q in %v", want, got)
		}
	}
	for _, banned := range []string{"hello,", "world!", "foo."} {
		if contains(got, banned) {
			t.Errorf("punctuation leaked into feature %q in %v", banned, got)
		}
	}
}

func TestTokenize_DropsURLs(t *testing.T) {
	got := toStrings(Tokenize("check https://example.com/path?x=1 it's nice"))
	for _, banned := range []string{"https://example.com/path?x=1", "https://example.com/path", "example.com"} {
		if contains(got, banned) {
			t.Errorf("URL leaked as feature %q: %v", banned, got)
		}
	}
	if !contains(got, "check") {
		t.Errorf("expected 'check' word feature: %v", got)
	}
	if !contains(got, "nice") {
		t.Errorf("expected 'nice' word feature: %v", got)
	}
}

func TestTokenize_DropsHTTPAndHTTPSOnly(t *testing.T) {
	// ftp:// and other schemes are not in the V1 drop list; they survive as
	// word tokens (possibly with their punctuation stripped) so this test
	// pins V1 behaviour.
	got := toStrings(Tokenize("ftp://files.example.com/x"))
	if len(got) == 0 {
		t.Fatal("expected ftp:// token to survive (V1 only drops http/https)")
	}
}

func TestTokenize_KeepsHashtagsAndMentions(t *testing.T) {
	// Hashtags and mentions are surrounded by no whitespace, so they are
	// single tokens. The leading '#' / '@' is stripped by the
	// unicode.IsPunct trim, leaving the bare label as the surviving
	// feature — this is V1 behaviour and is the expected outcome.
	got := toStrings(Tokenize("loving #weekend and @alice"))
	if !contains(got, "weekend") {
		t.Errorf("expected 'weekend' from #weekend, got %v", got)
	}
	if !contains(got, "alice") {
		t.Errorf("expected 'alice' from @alice, got %v", got)
	}
}

func TestTokenize_VietnameseSurvivesViaNgrams(t *testing.T) {
	// Vietnamese tokens are short — many are 2–4 syllables, each 1–4 runes.
	// V1 ships no dictionary; n-grams (n=3..5, strictly shorter than the
	// token) carry the sub-syllable signal. This test asserts that:
	//   1. the input survives whitespace tokenization,
	//   2. each surviving syllable appears as a word feature,
	//   3. for syllables longer than 3 runes, character 3-grams are also
	//      emitted (sub-token signal that is the V1 quality story).
	content := "Hôm nay trời đẹp"
	got := toStrings(Tokenize(content))

	for _, want := range []string{"hôm", "nay", "trời", "đẹp"} {
		if !contains(got, want) {
			t.Errorf("missing Vietnamese word feature %q in %v", want, got)
		}
	}

	// "trời" has 4 runes (t r ờ i): 3-grams are "trờ" and "rời".
	for _, want := range []string{"trờ", "rời"} {
		if !contains(got, want) {
			t.Errorf("missing 3-gram %q from 'trời' in %v", want, got)
		}
	}
}

func TestTokenize_NoNgramWhenWordTooShort(t *testing.T) {
	// "ai" has 2 runes, smaller than ngramMin. Only the word feature is
	// emitted; no n-grams. Crucially, we MUST NOT emit "ai" twice (once as
	// word, once as a degenerate n-gram of length 2 — which would be
	// shorter than ngramMin anyway, and a length-3 n-gram is impossible).
	got := toStrings(Tokenize("ai"))
	if len(got) != 1 || got[0] != "ai" {
		t.Fatalf("expected single feature 'ai', got %v", got)
	}
}

func TestTokenize_NoDuplicateWhenWordEqualsNgramLength(t *testing.T) {
	// "Hôm" has exactly 3 runes. The whole-word n-gram (n=3) would equal
	// the word feature itself, so the strict-less-than-len rule must skip
	// it. The "hôm" feature must appear exactly once.
	got := toStrings(Tokenize("Hôm"))
	if c := count(got, "hôm"); c != 1 {
		t.Fatalf("expected 'hôm' to appear exactly once, got %d in %v", c, got)
	}
}

func TestTokenize_NgramSlidingWindow(t *testing.T) {
	// "abcdef" has 6 runes. 3-grams: abc, bcd, cde, def. 4-grams: abcd,
	// bcde, cdef. 5-grams: abcde, bcdef. Plus the word feature itself.
	got := toStrings(Tokenize("abcdef"))
	wantSubset := []string{"abcdef", "abc", "bcd", "cde", "def", "abcd", "bcde", "cdef", "abcde", "bcdef"}
	for _, w := range wantSubset {
		if !contains(got, w) {
			t.Errorf("missing expected feature %q in %v", w, got)
		}
	}
	// Total expected: 1 word + 4 + 3 + 2 = 10 features.
	if len(got) != 10 {
		t.Errorf("expected 10 features, got %d: %v", len(got), got)
	}
}

func TestTokenize_NFCNormalisation(t *testing.T) {
	// "é" can be precomposed (one rune) or decomposed (two runes: e + ◌́).
	// Both must produce the same feature stream after NFC normalisation.
	precomposed := "café" // U+00E9
	decomposed := "café" // e + combining acute accent

	gotA := toStrings(Tokenize(precomposed))
	gotB := toStrings(Tokenize(decomposed))

	if len(gotA) != len(gotB) {
		t.Fatalf("feature counts differ: %d vs %d (%v vs %v)", len(gotA), len(gotB), gotA, gotB)
	}
	for i := range gotA {
		if gotA[i] != gotB[i] {
			t.Errorf("feature %d differs: %q vs %q", i, gotA[i], gotB[i])
		}
	}
}

func TestTokenize_DeterministicAcrossCalls(t *testing.T) {
	content := "Hôm nay trời đẹp quá, ai cũng muốn ra biển! #weekend"
	first := toStrings(Tokenize(content))
	second := toStrings(Tokenize(content))
	if len(first) != len(second) {
		t.Fatalf("non-deterministic feature count: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("feature %d differs across calls: %q vs %q", i, first[i], second[i])
		}
	}
}

func TestTokenize_BatchOfMixedContent(t *testing.T) {
	got := toStrings(Tokenize("HELLO world https://x.com END"))
	if !contains(got, "hello") || !contains(got, "world") || !contains(got, "end") {
		t.Errorf("expected hello/world/end in %v", got)
	}
	for _, banned := range []string{"https://x.com", "https", "x.com"} {
		if contains(got, banned) {
			t.Errorf("URL token leaked: %v", got)
		}
	}
}
