package similarity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repeatedPunct builds n identical punctuation tokens, the simplest input long
// enough to produce k-grams.
func repeatedPunct(n int) []Token {
	tokens := make([]Token, n)
	for i := range tokens {
		tokens[i] = Token{Kind: TokenPunct, Text: ";"}
	}
	return tokens
}

func TestFingerprintsDeterministic(t *testing.T) {
	tokens := []Token{{Kind: TokenKeyword, Text: "int"}, {Kind: TokenIdentifier, Text: "ID"}}
	require.Equal(t, Fingerprints(tokens), Fingerprints(tokens))
}

func TestFingerprintsIgnoreShortInput(t *testing.T) {
	assert.Empty(t, Fingerprints([]Token{{Kind: TokenKeyword, Text: "int"}}))
}

func TestFingerprintsChangeWhenCodeChanges(t *testing.T) {
	base := make([]Token, 0, 80)
	for i := 0; i < 80; i++ {
		base = append(base, Token{Kind: TokenPunct, Text: ";"})
	}
	changed := append([]Token{}, base...)
	changed[40].Text = "{}"
	assert.NotEqual(t, Fingerprints(base), Fingerprints(changed))
}

func TestWinnowSelectsRightmostMinimum(t *testing.T) {
	got := winnow([]uint64{5, 1, 1, 9}, 2)
	assert.Equal(t, []Fingerprint{1, 1}, got)
}

func TestWinnowTreatsOversizedWindowAsOneWindow(t *testing.T) {
	got := winnow([]uint64{4, 2, 8, 2, 9}, 10)
	assert.Equal(t, []Fingerprint{2}, got)
}

func TestWinnowEmptyInput(t *testing.T) {
	assert.Empty(t, winnow(nil, WindowSize))
}

// TestFingerprintsExactlyOneKGram checks the boundary: KGramSize tokens are
// enough for exactly one k-gram and therefore one fingerprint.
func TestFingerprintsExactlyOneKGram(t *testing.T) {
	assert.Len(t, Fingerprints(repeatedPunct(KGramSize)), 1)
}

func TestFingerprintsBelowKGramSizeAreEmpty(t *testing.T) {
	assert.Empty(t, Fingerprints(repeatedPunct(KGramSize-1)))
}

func TestFingerprintsAreUnique(t *testing.T) {
	tokens := make([]Token, 0, 120)
	for i := 0; i < 120; i++ {
		switch i % 3 {
		case 1:
			tokens = append(tokens, Token{Kind: TokenKeyword, Text: "if"})
		case 2:
			tokens = append(tokens, Token{Kind: TokenIdentifier, Text: "ID"})
		default:
			tokens = append(tokens, Token{Kind: TokenPunct, Text: ";"})
		}
	}

	got := Fingerprints(tokens)
	require.NotEmpty(t, got)
	seen := make(map[Fingerprint]struct{}, len(got))
	for _, fp := range got {
		require.NotContains(t, seen, fp)
		seen[fp] = struct{}{}
	}
}

func TestFingerprintsIgnoreLineNumbers(t *testing.T) {
	base := repeatedPunct(80)
	renumbered := append([]Token{}, base...)
	for i := range renumbered {
		renumbered[i].Line = i + 1
	}
	assert.Equal(t, Fingerprints(base), Fingerprints(renumbered))
}
