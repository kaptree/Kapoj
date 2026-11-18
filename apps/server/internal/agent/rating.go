package agent

import (
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	difficultyMin = 1
	difficultyMax = 100
	// ratingK is the Elo K factor used by the Agent rating design.
	ratingK = 32.0
	// ratingScale is the Elo logistic scale.
	ratingScale = 400.0
	// difficultyRatingMin and difficultyRatingMax map difficulty 1..100 onto
	// the 800..2400 opponent-rating scale.
	difficultyRatingMin = 800.0
	difficultyRatingMax = 2400.0
)

// DifficultyRating maps a 1..100 problem difficulty onto the Elo opponent
// rating scale. Out-of-range values are clamped to 1 or 100.
func DifficultyRating(difficulty int) float64 {
	if difficulty < difficultyMin {
		difficulty = difficultyMin
	}
	if difficulty > difficultyMax {
		difficulty = difficultyMax
	}
	span := difficultyRatingMax - difficultyRatingMin
	return difficultyRatingMin +
		float64(difficulty-difficultyMin)*span/float64(difficultyMax-difficultyMin)
}

// ExpectedScore is the Elo expected score of an Agent with rating against a
// problem whose difficulty is mapped by DifficultyRating.
func ExpectedScore(rating, difficulty int) float64 {
	return 1 / (1 + math.Pow(10, (DifficultyRating(difficulty)-float64(rating))/ratingScale))
}

// SolveDelta is the positive rating change for a first accepted submission.
// It is rounded and clamped to 1..32.
func SolveDelta(rating, difficulty int) int {
	delta := int(math.Round(ratingK * (1 - ExpectedScore(rating, difficulty))))
	if delta < 1 {
		return 1
	}
	if delta > 32 {
		return 32
	}
	return delta
}

// FailureDelta is the negative rating change for abandoning, timing out or
// exhausting the attempts on a problem. It is rounded and clamped to -32..-1.
func FailureDelta(rating, difficulty int) int {
	delta := int(math.Round(ratingK * ExpectedScore(rating, difficulty) / 2))
	if delta < 1 {
		delta = 1
	}
	if delta > 32 {
		delta = 32
	}
	return -delta
}

// applySolve applies a first accepted submission. The caller must hold the
// agents row lock. The solve is recorded at most once per problem and updates
// passed_count plus the incremental average solve time.
func (s *Service) applySolve(
	tx *gorm.DB, a *Agent, attempt *AgentProblemAttempt, submissionID int64, now time.Time,
) error {
	difficulty, err := problemDifficultyTx(tx, attempt.ProblemID)
	if err != nil {
		return err
	}

	var exists bool
	if err := tx.Raw(`
		SELECT EXISTS(
			SELECT 1 FROM agent_rating_history
			WHERE agent_id = ? AND problem_id = ? AND kind = ?)`,
		a.ID, attempt.ProblemID, RatingSolve).Scan(&exists).Error; err != nil {
		return err
	}
	if exists {
		return nil
	}

	before := a.Rating
	delta := SolveDelta(before, difficulty)
	after := before + delta
	if after < 0 {
		after = 0
	}
	solveTime := attempt.SolveTimeMS
	if solveTime < 0 {
		solveTime = 0
	}
	subID := submissionID
	entry := AgentRatingHistory{
		AgentID:      a.ID,
		ProblemID:    attempt.ProblemID,
		SubmissionID: &subID,
		Kind:         RatingSolve,
		Difficulty:   difficulty,
		RatingBefore: before,
		RatingAfter:  after,
		Delta:        after - before,
		SolveTimeMS:  solveTime,
		CreatedAt:    now,
	}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}

	oldPassed := a.PassedCount
	newPassed := oldPassed + 1
	avg := int(math.Round(
		(float64(a.AvgSolveTimeMS)*float64(oldPassed) + float64(solveTime)) / float64(newPassed),
	))
	if err := tx.Model(&Agent{}).Where("id = ?", a.ID).Updates(map[string]any{
		"rating":            after,
		"passed_count":      newPassed,
		"avg_solve_time_ms": avg,
		"updated_at":        now,
	}).Error; err != nil {
		return err
	}
	a.Rating = after
	a.PassedCount = newPassed
	a.AvgSolveTimeMS = avg
	a.UpdatedAt = now
	return nil
}

// applyFailure applies the one-time failure penalty for abandoning, timing out
// or exhausting a problem. The caller must hold the agents row lock. It
// returns the actual stored delta, or nil when a failure for the problem was
// already recorded.
func (s *Service) applyFailure(
	tx *gorm.DB, a *Agent, attempt *AgentProblemAttempt, submissionID *int64,
	kind RatingKind, now time.Time,
) (*int, error) {
	difficulty, err := problemDifficultyTx(tx, attempt.ProblemID)
	if err != nil {
		return nil, err
	}

	before := a.Rating
	delta := FailureDelta(before, difficulty)
	after := before + delta
	if after < 0 {
		after = 0
	}
	actual := after - before

	var subID *int64
	if submissionID != nil {
		id := *submissionID
		subID = &id
	}
	entry := AgentRatingHistory{
		AgentID:      a.ID,
		ProblemID:    attempt.ProblemID,
		SubmissionID: subID,
		Kind:         kind,
		Difficulty:   difficulty,
		RatingBefore: before,
		RatingAfter:  after,
		Delta:        actual,
		CreatedAt:    now,
	}
	// uniq_agent_rating_fail covers abandon/timeout/exhausted across the whole
	// problem, so only the first failure can ever change the rating.
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}

	if err := tx.Model(&Agent{}).Where("id = ?", a.ID).Updates(map[string]any{
		"rating":     after,
		"updated_at": now,
	}).Error; err != nil {
		return nil, err
	}
	a.Rating = after
	a.UpdatedAt = now
	return &actual, nil
}

// expireAttemptTx marks a stale active attempt as expired and charges the
// timeout penalty at most once. The caller must hold the agents and attempt
// row locks.
func (s *Service) expireAttemptTx(
	tx *gorm.DB, a *Agent, attempt *AgentProblemAttempt, now time.Time,
) (*int, error) {
	res := tx.Model(&AgentProblemAttempt{}).
		Where("id = ? AND status = ?", attempt.ID, AttemptActive).
		Updates(map[string]any{
			"status":     AttemptExpired,
			"updated_at": now,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	attempt.Status = AttemptExpired
	attempt.UpdatedAt = now
	return s.applyFailure(tx, a, attempt, nil, RatingTimeout, now)
}

func problemDifficultyTx(tx *gorm.DB, problemID int64) (int, error) {
	var difficulty int
	res := tx.Raw(`SELECT difficulty FROM problems WHERE id = ?`, problemID).Scan(&difficulty)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return difficulty, nil
}
