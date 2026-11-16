package similarity

import (
	"hash/fnv"
	"sync"
)

const (
	// KGramSize is the number of consecutive tokens hashed into one k-gram.
	KGramSize = 20
	// WindowSize is the number of consecutive k-gram hashes winnowed together.
	WindowSize = 10
)

// Fingerprint is one selected k-gram hash. Two submissions are compared by the
// overlap of their fingerprint sets, so values must be stable across runs and
// independent of token position.
type Fingerprint uint64

// rollingBase is the multiplier of the polynomial k-gram hash. Overflow is
// intentional: the hash is defined modulo 2^64.
const rollingBase uint64 = 1000003

// tokenHashes caches the FNV-1a hash of each normalized token text. Token
// streams repeat heavily, so the cache keeps rehashing cheap; sync.Map makes it
// safe for concurrent analysis of multiple submissions.
var tokenHashes sync.Map // map[string]uint64

// tokenHash returns the stable FNV-1a hash of a normalized token text.
func tokenHash(text string) uint64 {
	if cached, ok := tokenHashes.Load(text); ok {
		return cached.(uint64)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	sum := h.Sum64()
	tokenHashes.Store(text, sum)
	return sum
}

// kGramHashes hashes every window of KGramSize tokens, in order. Each k-gram is
// hashed independently so identical token sequences produce identical hashes
// regardless of where they appear in the file.
func kGramHashes(tokens []Token) []uint64 {
	if len(tokens) < KGramSize {
		return nil
	}
	hashes := make([]uint64, 0, len(tokens)-KGramSize+1)
	for start := 0; start+KGramSize <= len(tokens); start++ {
		var hash uint64
		for _, tok := range tokens[start : start+KGramSize] {
			hash = hash*rollingBase + tokenHash(tok.Text)
		}
		hashes = append(hashes, hash)
	}
	return hashes
}

// winnow selects the minimum hash of every window of w consecutive hashes. On
// ties it keeps the rightmost position, matching the classic Winnowing
// algorithm, so shifting a window by one rarely changes the selection. A
// position chosen by adjacent windows is emitted only once; separate positions
// with equal hashes are both emitted. A window wider than the input is treated
// as a single window over the whole input.
func winnow(hashes []uint64, w int) []Fingerprint {
	if len(hashes) == 0 || w <= 0 {
		return nil
	}
	if w > len(hashes) {
		w = len(hashes)
	}

	selected := make([]Fingerprint, 0, len(hashes))
	last := -1
	for start := 0; start+w <= len(hashes); start++ {
		minIdx := start
		for i := start + 1; i < start+w; i++ {
			if hashes[i] <= hashes[minIdx] {
				minIdx = i
			}
		}
		if minIdx != last {
			selected = append(selected, Fingerprint(hashes[minIdx]))
			last = minIdx
		}
	}
	return selected
}

// Fingerprints returns the winnowed fingerprints of a token stream: unique
// values in the order they were first selected. Streams shorter than one k-gram
// produce no fingerprints.
func Fingerprints(tokens []Token) []Fingerprint {
	if len(tokens) < KGramSize {
		return []Fingerprint{}
	}

	selected := winnow(kGramHashes(tokens), WindowSize)
	unique := make([]Fingerprint, 0, len(selected))
	seen := make(map[Fingerprint]struct{}, len(selected))
	for _, fp := range selected {
		if _, ok := seen[fp]; ok {
			continue
		}
		seen[fp] = struct{}{}
		unique = append(unique, fp)
	}
	return unique
}
