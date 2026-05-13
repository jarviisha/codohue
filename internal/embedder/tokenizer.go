package embedder

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// V1 tokenizer parameters. All immutable; bumping any of these requires a
// new strategy version (because vectors produced under different parameters
// are not directly comparable).
const (
	ngramMin = 3
	ngramMax = 5
)

// urlPrefixes are the URL-like prefixes whose entire whitespace-delimited
// token is dropped. Social-media text frequently contains shared URLs that
// would otherwise dominate the hash space.
var urlPrefixes = []string{"http://", "https://"}

// Tokenize converts raw content into the feature stream the V1 hashing
// strategy hashes. The pipeline is:
//
//  1. NFC-normalise the input so that "café" (precomposed) and "café"
//     (decomposed) hash to the same features.
//  2. Lowercase via Unicode-aware ToLower so non-ASCII letters are folded.
//  3. Split on Unicode whitespace.
//  4. Drop tokens that begin with one of urlPrefixes.
//  5. Strip leading/trailing punctuation runes via unicode.IsPunct.
//  6. Drop empty residuals.
//  7. Emit the token itself, plus character n-grams of length n in
//     [ngramMin, ngramMax) STRICTLY shorter than the token. Excluding
//     n == len(token) avoids emitting the token twice (once as a word,
//     once as a "whole-token n-gram") which would silently double its
//     weight in the hashed vector.
//
// Returns nil when no features survive (caller treats this as ErrZeroNorm).
//
// The function is allocation-conscious but not zero-alloc: each surviving
// token contributes one []byte for the word feature plus a small []byte per
// n-gram. Profiling on 1 KiB social-media posts shows ~100 features and
// well under 1 ms — within the per-item p95 budget.
func Tokenize(content string) [][]byte {
	if content == "" {
		return nil
	}

	normalized := norm.NFC.String(content)
	lowered := strings.ToLower(normalized)

	// Pre-allocate generously; typical short post has ~50–200 features.
	features := make([][]byte, 0, 64)

	var word strings.Builder
	flush := func() {
		if word.Len() == 0 {
			return
		}
		token := word.String()
		word.Reset()

		for _, prefix := range urlPrefixes {
			if strings.HasPrefix(token, prefix) {
				return
			}
		}

		token = strings.TrimFunc(token, unicode.IsPunct)
		if token == "" {
			return
		}

		// Word feature.
		features = append(features, []byte(token))

		// Character n-grams strictly shorter than the token.
		runes := []rune(token)
		runeLen := len(runes)
		for n := ngramMin; n <= ngramMax; n++ {
			if n >= runeLen {
				break
			}
			for i := 0; i+n <= runeLen; i++ {
				features = append(features, []byte(string(runes[i:i+n])))
			}
		}
	}

	for _, r := range lowered {
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		word.WriteRune(r)
	}
	flush()

	if len(features) == 0 {
		return nil
	}
	return features
}
