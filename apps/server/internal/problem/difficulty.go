package problem

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Values of problems.difficulty_source.
const (
	DifficultySourceDefault   = "default"
	DifficultySourceAI        = "ai"
	DifficultySourcePassRate  = "pass_rate"
	DifficultySourceManual    = "manual"
	DifficultySourceMigration = "migration"
	DifficultySourceImport    = "import"
)

// Values of problem_difficulty_history.source. They distinguish the first AI
// assessment from a later admin-triggered reassessment.
const (
	DifficultySourceAIInitial  = "ai_initial"
	DifficultySourceAIReassess = "ai_reassess"
)

// Values of problems.difficulty_prior_source.
const (
	DifficultyPriorSourceAI       = "ai"
	DifficultyPriorSourceDefault  = "default"
	DifficultyPriorSourceMigrated = "migrated"
	DifficultyPriorSourceManual   = "manual"
	DifficultyPriorSourceImport   = "import"
)

// DifficultyHistory is one immutable difficulty change for a problem. OldScore
// is 0 when the problem was created. RunID is set only for scheduled recalcs.
type DifficultyHistory struct {
	ID               int64     `gorm:"primaryKey"`
	ProblemID        int64     `gorm:"not null;index:idx_problem_difficulty_history_problem,priority:1"`
	OldScore         int       `gorm:"not null"`
	NewScore         int       `gorm:"not null"`
	Prior            int       `gorm:"not null"`
	Source           string    `gorm:"size:20;not null"`
	SampleUsers      int       `gorm:"not null;default:0"`
	AcceptedUsers    int       `gorm:"not null;default:0"`
	PassRate         float64   `gorm:"type:numeric(6,5);not null;default:0"`
	SmoothedPassRate float64   `gorm:"type:numeric(6,5);not null;default:0"`
	TargetScore      int       `gorm:"not null;default:0"`
	Confidence       float64   `gorm:"type:numeric(4,3);not null;default:0"`
	Reason           string    `gorm:"type:text;not null;default:''"`
	RunID            *int64    `gorm:"index:idx_problem_difficulty_history_run"`
	CreatedBy        int64     `gorm:"not null;default:0"`
	CreatedAt        time.Time `gorm:"type:timestamptz;not null;index:idx_problem_difficulty_history_problem,priority:2,sort:desc"`
}

// TableName keeps the table name from the design document.
func (DifficultyHistory) TableName() string { return "problem_difficulty_history" }

// DifficultyDecision is the final difficulty metadata chosen by the caller: an
// AI assessment, a manual edit, an import or the legacy Create/Update wrappers.
// Score is the resulting score and Prior is the anchor used by the daily
// recalculation. ActorID is recorded as history.created_by.
type DifficultyDecision struct {
	Score, Prior int
	// PriorSource and Source describe where the prior and the score came from;
	// empty values fall back to manual.
	PriorSource, Source string
	Auto                bool
	Reason              string
	Confidence          float64
	ActorID             int64
	// forceHistory makes an update log a row even when the score is
	// unchanged. Imports use it so each imported problem has an import entry;
	// follow-up content updates do not.
	forceHistory bool
}

// validateDifficultyScore enforces the 1–100 scale shared by problems and
// their history rows.
func validateDifficultyScore(score int) error {
	if score < 1 || score > 100 {
		return httpx.ErrValidation("难度需为 1–100")
	}
	return nil
}

// normalized fills defaults that only depend on the decision itself: a missing
// prior anchors at the score and missing sources mean a manual edit.
func (d DifficultyDecision) normalized() DifficultyDecision {
	if d.Prior == 0 {
		d.Prior = d.Score
	}
	if d.PriorSource == "" {
		d.PriorSource = DifficultyPriorSourceManual
	}
	if d.Source == "" {
		d.Source = DifficultySourceManual
	}
	return d
}

// applyDifficultyDecision writes the decision onto p. DifficultyUpdatedAt only
// moves when a difficulty field actually changes, so unrelated edits keep it.
func applyDifficultyDecision(p *Problem, d DifficultyDecision) {
	d = d.normalized()
	changed := p.Difficulty != d.Score ||
		p.DifficultyPrior != d.Prior ||
		p.DifficultyPriorSource != d.PriorSource ||
		p.DifficultyAuto != d.Auto ||
		p.DifficultySource != d.Source ||
		p.DifficultyReason != d.Reason
	p.Difficulty = d.Score
	p.DifficultyPrior = d.Prior
	p.DifficultyPriorSource = d.PriorSource
	p.DifficultyAuto = d.Auto
	p.DifficultySource = d.Source
	p.DifficultyReason = d.Reason
	if changed {
		p.DifficultyUpdatedAt = time.Now()
	}
}

// newDifficultyHistory builds the history row for a move from oldScore to the
// decision's score.
func newDifficultyHistory(oldScore int, d DifficultyDecision) *DifficultyHistory {
	d = d.normalized()
	return &DifficultyHistory{
		OldScore: oldScore,
		NewScore: d.Score,
		Prior:    d.Prior,
		Source:   d.Source,
		// Populated by the scheduled recalculation, not by edits.
		Confidence: d.Confidence,
		Reason:     d.Reason,
		CreatedBy:  d.ActorID,
	}
}
