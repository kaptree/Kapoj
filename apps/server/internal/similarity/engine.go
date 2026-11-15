package similarity

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

const (
	// MinTokens is the minimum number of normalized tokens a submission must
	// have before it participates in a comparison.
	MinTokens = 80
	// MinSharedFingerprints is the minimum number of shared fingerprints two
	// submissions must have to be reported as a pair.
	MinSharedFingerprints = 5
	// MaxSubmissionsPerProblem bounds the number of candidates a single
	// comparison run accepts.
	MaxSubmissionsPerProblem = 1000
)

// Candidate is one submission eligible for comparison.
type Candidate struct {
	ID        int64
	UserID    int64
	Username  string
	Nickname  string
	Language  submission.Language
	Code      string
	CreatedAt time.Time
}

// EngineRequest describes one comparison run.
type EngineRequest struct {
	Candidates       []Candidate
	ThresholdPercent float64
	BaseCode         string
	BaseLanguage     submission.Language
	IgnoreCommonCode bool
}

// Pair is a reported similarity match between an earlier (original) and a
// later (suspect) submission.
type Pair struct {
	OriginalSubmissionID int64
	SuspectSubmissionID  int64
	OriginalUserID       int64
	SuspectUserID        int64
	Language             submission.Language
	SimilarityPercent    float64
	DicePercent          float64
	SharedFingerprints   int
	OriginalFingerprints int
	SuspectFingerprints  int
	OriginalCreatedAt    time.Time
	SuspectCreatedAt     time.Time
}

// EngineResult is the outcome of a comparison run.
type EngineResult struct {
	Pairs           []Pair
	SubmissionCount int
	SkippedCount    int
}

// Engine compares submissions and attributes pairs.
type Engine interface {
	Compare(ctx context.Context, req EngineRequest) (EngineResult, error)
}

// WinnowingEngine is the pure in-memory winnowing similarity engine.
type WinnowingEngine struct{}

// NewWinnowingEngine returns a new engine instance.
func NewWinnowingEngine() *WinnowingEngine { return &WinnowingEngine{} }

// candidateFP pairs a candidate with its normalized fingerprint set.
type candidateFP struct {
	candidate    Candidate
	fingerprints []Fingerprint
}

// supportedLanguage reports whether lang can be tokenized for comparison.
// Objective "answer" submissions and any unknown language are excluded.
func supportedLanguage(lang submission.Language) bool {
	switch lang {
	case submission.LanguageC, submission.LanguageCPP, submission.LanguageJava,
		submission.LanguageGo, submission.LanguagePython3:
		return true
	default:
		return false
	}
}

// Compare runs the winnowing similarity algorithm over req.Candidates.
func (e *WinnowingEngine) Compare(ctx context.Context, req EngineRequest) (EngineResult, error) {
	if len(req.Candidates) > MaxSubmissionsPerProblem {
		return EngineResult{}, fmt.Errorf(
			"similarity: too many submissions (%d > %d)", len(req.Candidates), MaxSubmissionsPerProblem)
	}

	var result EngineResult

	// Group candidates by language, preserving the original language order for
	// deterministic processing. Skip unsupported languages, the objective
	// "answer" language, submissions too short to fingerprint, and candidates
	// whose tokenization fails.
	groups := make(map[submission.Language][]*candidateFP, 5)
	order := make([]submission.Language, 0, 5)
	for _, c := range req.Candidates {
		if err := ctx.Err(); err != nil {
			return EngineResult{}, err
		}
		if !supportedLanguage(c.Language) {
			result.SkippedCount++
			continue
		}
		tokens, err := Tokenize(c.Language, c.Code)
		if err != nil {
			result.SkippedCount++
			continue
		}
		if len(tokens) < MinTokens {
			result.SkippedCount++
			continue
		}
		fps := Fingerprints(tokens)
		if len(fps) == 0 {
			result.SkippedCount++
			continue
		}
		if _, ok := groups[c.Language]; !ok {
			order = append(order, c.Language)
		}
		groups[c.Language] = append(groups[c.Language], &candidateFP{candidate: c, fingerprints: fps})
		result.SubmissionCount++
	}

	// Remove base-template fingerprints from every candidate of the matching
	// language so shared boilerplate does not inflate similarity.
	if req.BaseCode != "" {
		if baseFP := fingerprintsFor(req.BaseLanguage, req.BaseCode); len(baseFP) > 0 {
			remove := make(map[Fingerprint]struct{}, len(baseFP))
			for _, fp := range baseFP {
				remove[fp] = struct{}{}
			}
			for _, cfp := range groups[req.BaseLanguage] {
				cfp.fingerprints = removeFingerprints(cfp.fingerprints, remove)
			}
		}
	}

	// Drop fingerprints that appear in more than half of a language's
	// candidates; they represent common code rather than copied code.
	if req.IgnoreCommonCode {
		for _, lang := range order {
			g := groups[lang]
			if len(g) < 5 {
				continue
			}
			docFreq := make(map[Fingerprint]int)
			for _, cfp := range g {
				for _, fp := range cfp.fingerprints {
					docFreq[fp]++
				}
			}
			var remove map[Fingerprint]struct{}
			for fp, count := range docFreq {
				if count*2 > len(g) {
					if remove == nil {
						remove = make(map[Fingerprint]struct{})
					}
					remove[fp] = struct{}{}
				}
			}
			if remove != nil {
				for _, cfp := range g {
					cfp.fingerprints = removeFingerprints(cfp.fingerprints, remove)
				}
			}
		}
	}

	// Build an inverted index (fingerprint -> candidates) per language and
	// count shared fingerprints for every candidate pair.
	pairs := make([]Pair, 0)
	for _, lang := range order {
		g := groups[lang]
		index := make(map[Fingerprint][]int)
		for i, cfp := range g {
			for _, fp := range cfp.fingerprints {
				index[fp] = append(index[fp], i)
			}
		}
		shared := make(map[[2]int]int)
		for _, idxs := range index {
			if len(idxs) < 2 {
				continue
			}
			for a := 0; a < len(idxs); a++ {
				for b := a + 1; b < len(idxs); b++ {
					i, j := idxs[a], idxs[b]
					if i > j {
						i, j = j, i
					}
					shared[[2]int{i, j}]++
				}
			}
		}

		for key, sh := range shared {
			if sh < MinSharedFingerprints {
				continue
			}
			a, b := g[key[0]], g[key[1]]
			if a.candidate.UserID == b.candidate.UserID {
				continue
			}
			lenA, lenB := len(a.fingerprints), len(b.fingerprints)
			similarity := math.Max(float64(sh)/float64(lenA), float64(sh)/float64(lenB)) * 100
			if similarity < req.ThresholdPercent {
				continue
			}
			dice := 200 * float64(sh) / float64(lenA+lenB)

			original, suspect := a, b
			if b.candidate.CreatedAt.Before(a.candidate.CreatedAt) ||
				(b.candidate.CreatedAt.Equal(a.candidate.CreatedAt) && b.candidate.ID < a.candidate.ID) {
				original, suspect = b, a
			}

			pairs = append(pairs, Pair{
				OriginalSubmissionID: original.candidate.ID,
				SuspectSubmissionID:  suspect.candidate.ID,
				OriginalUserID:       original.candidate.UserID,
				SuspectUserID:        suspect.candidate.UserID,
				Language:             original.candidate.Language,
				SimilarityPercent:    similarity,
				DicePercent:          dice,
				SharedFingerprints:   sh,
				OriginalFingerprints: len(original.fingerprints),
				SuspectFingerprints:  len(suspect.fingerprints),
				OriginalCreatedAt:    original.candidate.CreatedAt,
				SuspectCreatedAt:     suspect.candidate.CreatedAt,
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].SimilarityPercent != pairs[j].SimilarityPercent {
			return pairs[i].SimilarityPercent > pairs[j].SimilarityPercent
		}
		if pairs[i].OriginalSubmissionID != pairs[j].OriginalSubmissionID {
			return pairs[i].OriginalSubmissionID < pairs[j].OriginalSubmissionID
		}
		return pairs[i].SuspectSubmissionID < pairs[j].SuspectSubmissionID
	})

	result.Pairs = pairs
	return result, nil
}

// fingerprintsFor tokenizes source in lang and returns its fingerprints. Any
// failure (unsupported language, short input) yields an empty slice.
func fingerprintsFor(lang submission.Language, source string) []Fingerprint {
	if !supportedLanguage(lang) {
		return nil
	}
	tokens, err := Tokenize(lang, source)
	if err != nil {
		return nil
	}
	return Fingerprints(tokens)
}

// removeFingerprints returns a copy of set without any fingerprint present in
// remove.
func removeFingerprints(set []Fingerprint, remove map[Fingerprint]struct{}) []Fingerprint {
	out := make([]Fingerprint, 0, len(set))
	for _, fp := range set {
		if _, ok := remove[fp]; !ok {
			out = append(out, fp)
		}
	}
	return out
}
