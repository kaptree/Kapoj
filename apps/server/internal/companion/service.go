package companion

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Bond rules for each event type (companion design doc §8.2).
const (
	signinBond         = 3
	dailyACBond        = 2
	firstACBond        = 1
	firstACDailySubCap = 3
	taskBond           = 1
	taskDailySubCap    = 2
	achievementBond    = 1
	achievementSubCap  = 2
	contestBond        = 5
	streak7Bond        = 5
	streak30Bond       = 10
	streak100Bond      = 20
)

// streakMilestones lists the login-streak milestones in ascending order.
var streakMilestones = []struct {
	days int
	bond int
}{
	{7, streak7Bond},
	{30, streak30Bond},
	{100, streak100Bond},
}

// Service applies the bond event rules on top of the transactional repository.
// It is the shared recorder used by the API and the core-action hooks; every
// method returns an error so callers can decide whether a companion failure is
// fatal (async gamequeue) or best-effort (sign-in, submission, task, achievement).
type Service struct {
	repo *Repository
}

// NewService returns a Service backed by repo.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// State returns the user's companion state view, including the level title,
// next threshold, unread event count and today's gain breakdown.
func (s *Service) State(ctx context.Context, userID int64) (*StateView, error) {
	st, err := s.repo.EnsureState(ctx, userID)
	if err != nil {
		return nil, err
	}
	lvl := LevelForBond(st.Bond)
	unread, err := s.repo.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}
	today := time.Now().In(time.Local).Format("2006-01-02")
	todayEvents, err := s.repo.EventsByDate(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	// The stored counter only counts bond earned on st.DailyDate; a user whose
	// last gain landed on an earlier day must read zero until today's first
	// event, otherwise yesterday's gain leaks into today's meter.
	dailyGain := st.DailyGain
	if st.DailyDate != today {
		dailyGain = 0
	}
	return &StateView{
		Bond:          st.Bond,
		Level:         lvl.Level,
		LevelName:     lvl.Name,
		NextThreshold: lvl.Next,
		DailyGain:     dailyGain,
		DailyCap:      dailyCap,
		ActivityLevel: st.ActivityLevel,
		DockEnabled:   st.DockEnabled,
		PublicProfile: st.PublicProfile,
		EquippedSkin:  st.EquippedSkin,
		UnlockedSkins: []string(st.UnlockedSkins),
		UnreadCount:   unread,
		TodayEvents:   eventViews(todayEvents),
	}, nil
}

// equippedSkinMaxRunes matches the varchar(40) equipped_skin column so oversize
// input is reported as a validation error instead of a database error.
const equippedSkinMaxRunes = 40

// validActivityLevels mirrors the quiet | normal | lively choice in the
// companion design document.
var validActivityLevels = map[string]struct{}{
	"quiet":  {},
	"normal": {},
	"lively": {},
}

// validateActivityLevel enforces the settings activity enum. It is shared by
// the full and patch update paths so every caller is protected.
func validateActivityLevel(level string) error {
	if _, ok := validActivityLevels[level]; !ok {
		return httpx.ErrValidation("活动频率无效")
	}
	return nil
}

// validateEquippedSkin enforces the equipped-skin column width in runes;
// PostgreSQL varchar(40) also counts characters, not bytes.
func validateEquippedSkin(skin string) error {
	if utf8.RuneCountInString(skin) > equippedSkinMaxRunes {
		return httpx.ErrValidation("皮肤标识不能超过 40 个字符")
	}
	return nil
}

// UpdateSettings persists the whole user-editable settings set and returns the
// fresh view. It validates through the same helpers as UpdateSettingsPatch.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in SettingsInput) (*StateView, error) {
	if err := validateActivityLevel(in.ActivityLevel); err != nil {
		return nil, err
	}
	if err := validateEquippedSkin(in.EquippedSkin); err != nil {
		return nil, err
	}
	if _, err := s.repo.UpdateSettings(ctx, userID, in); err != nil {
		return nil, err
	}
	return s.State(ctx, userID)
}

// UpdateSettingsPatch validates and atomically applies a partial settings
// update: only the provided fields are written, under a state-row lock, so two
// concurrent patches touching different fields both persist.
func (s *Service) UpdateSettingsPatch(ctx context.Context, userID int64, patch SettingsPatch) (*StateView, error) {
	if patch.ActivityLevel != nil {
		if err := validateActivityLevel(*patch.ActivityLevel); err != nil {
			return nil, err
		}
	}
	if patch.EquippedSkin != nil {
		if err := validateEquippedSkin(*patch.EquippedSkin); err != nil {
			return nil, err
		}
	}
	if _, err := s.repo.UpdateSettingsPatch(ctx, userID, patch); err != nil {
		return nil, err
	}
	return s.State(ctx, userID)
}

// Pet records one daily pet interaction and reports whether bond was awarded
// for this date, plus the resulting bond and level.
func (s *Service) Pet(ctx context.Context, userID int64, date string) (*PetResult, error) {
	_, awarded, err := s.repo.Pet(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	st, err := s.repo.EnsureState(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &PetResult{Awarded: awarded, Bond: st.Bond, Level: st.Level}, nil
}

// RecordSignIn records the daily sign-in (+3) and then any streak milestones.
func (s *Service) RecordSignIn(ctx context.Context, userID int64, date string) error {
	if _, _, err := s.repo.ApplyEvent(ctx, EventInput{
		UserID:    userID,
		Key:       "signin:" + date,
		Type:      "signin",
		Bond:      signinBond,
		EventDate: date,
		Payload:   map[string]any{},
	}); err != nil {
		return err
	}
	return s.recordStreak(ctx, userID, date)
}

// recordStreak awards the 7/30/100-day login milestones once each and bypasses
// the natural daily cap.
func (s *Service) recordStreak(ctx context.Context, userID int64, date string) error {
	streak, err := s.repo.LoginStreak(ctx, userID, date)
	if err != nil {
		return err
	}
	for _, m := range streakMilestones {
		if streak < m.days {
			continue
		}
		if _, _, err := s.repo.ApplyEvent(ctx, EventInput{
			UserID:         userID,
			Key:            fmt.Sprintf("streak:%d", m.days),
			Type:           "streak",
			Bond:           m.bond,
			EventDate:      date,
			Payload:        map[string]any{"days": m.days},
			DailyCapBypass: true,
		}); err != nil {
			return err
		}
	}
	return nil
}

// RecordAC awards bond for an accepted submission: the daily first AC (+2) and,
// when this is the user's first accepted submission for the problem, the
// per-problem first AC (+1, capped at 3/day). The local date is derived from
// the submission's judged_at, never the processing time.
func (s *Service) RecordAC(ctx context.Context, userID, submissionID int64) error {
	info, err := s.repo.SubmissionInfo(ctx, submissionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if info.UserID != userID {
		// The event references a submission that does not belong to this user.
		return nil
	}
	if info.Status != "accepted" || info.JudgedAt == nil {
		return nil
	}
	date := info.JudgedAt.In(time.Local).Format("2006-01-02")

	if _, _, err := s.repo.ApplyEvent(ctx, EventInput{
		UserID:    userID,
		Key:       "ac:daily:" + date,
		Type:      "ac_daily",
		Bond:      dailyACBond,
		EventDate: date,
		Payload:   map[string]any{"submissionId": submissionID},
	}); err != nil {
		return err
	}

	earlier, err := s.repo.HasEarlierAccepted(ctx, info.UserID, info.ProblemID, info.CreatedAt, info.ID)
	if err != nil {
		return err
	}
	if earlier {
		return nil
	}
	_, _, err = s.repo.ApplyEvent(ctx, EventInput{
		UserID:    userID,
		Key:       fmt.Sprintf("ac:first:%d", info.ProblemID),
		Type:      "ac_first",
		Bond:      firstACBond,
		EventDate: date,
		Payload:   map[string]any{"problemId": info.ProblemID},
		SubCap:    firstACDailySubCap,
	})
	return err
}

// RecordTaskClaim awards +1 for claiming a task, capped at 2/day. The event
// date is derived from the caller-supplied at (the claim timestamp), not the
// processing time.
func (s *Service) RecordTaskClaim(ctx context.Context, userID, taskID int64, at time.Time) error {
	_, _, err := s.repo.ApplyEvent(ctx, EventInput{
		UserID:    userID,
		Key:       fmt.Sprintf("task:%d", taskID),
		Type:      "task",
		Bond:      taskBond,
		EventDate: at.In(time.Local).Format("2006-01-02"),
		Payload:   map[string]any{"taskId": taskID},
		SubCap:    taskDailySubCap,
	})
	return err
}

// RecordAchievement awards +1 for unlocking an achievement, capped at 2/day.
// The event date is derived from the caller-supplied at (the unlock timestamp).
func (s *Service) RecordAchievement(ctx context.Context, userID, achievementID int64, at time.Time) error {
	_, _, err := s.repo.ApplyEvent(ctx, EventInput{
		UserID:    userID,
		Key:       fmt.Sprintf("achievement:%d", achievementID),
		Type:      "achievement",
		Bond:      achievementBond,
		EventDate: at.In(time.Local).Format("2006-01-02"),
		Payload:   map[string]any{"achievementId": achievementID},
		SubCap:    achievementSubCap,
	})
	return err
}

// RecordContestSubmission awards +5 for the first submission in a contest,
// fired on submission creation. The event date is derived from the
// caller-supplied at (the submission's created_at). Non-positive contest ids
// are ignored.
func (s *Service) RecordContestSubmission(ctx context.Context, userID, contestID int64, at time.Time) error {
	if contestID <= 0 {
		return nil
	}
	_, _, err := s.repo.ApplyEvent(ctx, EventInput{
		UserID:    userID,
		Key:       fmt.Sprintf("contest:%d", contestID),
		Type:      "contest",
		Bond:      contestBond,
		EventDate: at.In(time.Local).Format("2006-01-02"),
		Payload:   map[string]any{"contestId": contestID},
	})
	return err
}

// HandleGameEvent reacts to the gamequeue contract. It only acts on accepted
// submissions and returns an error so Asynq retries on transient failures.
func (s *Service) HandleGameEvent(ctx context.Context, ev gamequeue.Event) error {
	switch ev.Type {
	case gamequeue.EventSubmissionAccepted:
		return s.RecordAC(ctx, ev.UserID, ev.SubmissionID)
	}
	return nil
}

// UnreadEvents returns the user's unread events as view models.
func (s *Service) UnreadEvents(ctx context.Context, userID int64, limit int) ([]EventView, error) {
	events, err := s.repo.UnreadEvents(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	return eventViews(events), nil
}

// MarkEventsRead marks the given event ids as read for the user.
func (s *Service) MarkEventsRead(ctx context.Context, userID int64, ids []int64) error {
	return s.repo.MarkRead(ctx, userID, ids)
}

func eventViews(events []Event) []EventView {
	out := make([]EventView, 0, len(events))
	for _, e := range events {
		out = append(out, EventView{
			ID:        e.ID,
			Type:      e.Type,
			Bond:      e.Bond,
			Payload:   e.Payload.Data,
			ReadAt:    e.ReadAt,
			CreatedAt: e.CreatedAt,
		})
	}
	return out
}
