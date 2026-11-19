package companion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

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
	repo         *Repository
	dialogue     DialogueGenerator
	aiStatus     AIStatus
	nameProvider CompanionNameProvider
}

// Option configures optional companion dependencies.
type Option func(*Service)

// WithDialogue wires the AI generator used by the dialogue endpoint.
func WithDialogue(g DialogueGenerator) Option {
	return func(s *Service) { s.dialogue = g }
}

// WithAIStatus wires the active-model check used by state and dialogue.
func WithAIStatus(status AIStatus) Option {
	return func(s *Service) { s.aiStatus = status }
}

// NewService returns a Service backed by repo.
func NewService(repo *Repository, opts ...Option) *Service {
	s := &Service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetDialogueGenerator wires AI dialogue after the service has been built.
// This is used by main because the companion service is constructed before the
// AI service, while game services need the companion service first.
func (s *Service) SetDialogueGenerator(g DialogueGenerator) { s.dialogue = g }

// SetAIStatus wires the active-model check after construction.
func (s *Service) SetAIStatus(status AIStatus) { s.aiStatus = status }

// SetCompanionNameProvider wires the site-configured display name.
func (s *Service) SetCompanionNameProvider(p CompanionNameProvider) { s.nameProvider = p }

const (
	aiDialogueDailyCap  = 30
	aiDialogueMaxTokens = 120
	aiDialogueCooldown  = 5 * time.Second
	aiDialogueTimeout   = 4 * time.Second
)

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

	aiReady := false
	aiRemaining := 0
	if s.aiStatus != nil {
		ready, statusErr := s.aiStatus.HasActiveModel(ctx, userID)
		if statusErr != nil {
			slog.Warn("companion: check AI status failed", "err", statusErr)
		} else {
			aiReady = ready
		}
	}
	if aiReady {
		aiCount := st.AIDailyCount
		if st.AIDailyDate != today {
			aiCount = 0
		}
		aiRemaining = aiDialogueDailyCap - aiCount
		if aiRemaining < 0 {
			aiRemaining = 0
		}
	}

	return &StateView{
		Bond:              st.Bond,
		Level:             lvl.Level,
		LevelName:         lvl.Name,
		NextThreshold:     lvl.Next,
		DailyGain:         dailyGain,
		DailyCap:          dailyCap,
		ActivityLevel:     st.ActivityLevel,
		DockEnabled:       st.DockEnabled,
		PublicProfile:     st.PublicProfile,
		EquippedSkin:      st.EquippedSkin,
		UnlockedSkins:     mergeUnlockedSkins([]string(st.UnlockedSkins), st.Bond),
		PersonalityPrompt: st.PersonalityPrompt,
		AIDialogueEnabled: st.AIDialogueEnabled,
		AIReady:           aiReady,
		AIRemainingToday:  aiRemaining,
		UnreadCount:       unread,
		TodayEvents:       eventViews(todayEvents),
	}, nil
}

// PublicState returns the privacy-filtered summary of another user's companion.
// A private profile is only readable by its owner; everyone else (including
// guests, whose viewerID is 0) gets httpx.ErrNotFound so the profile cannot tell
// a hidden companion from a user that never used one.
func (s *Service) PublicState(ctx context.Context, userID, viewerID int64) (*PublicStateView, error) {
	st, err := s.repo.GetState(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if !st.PublicProfile && viewerID != userID {
		return nil, httpx.ErrNotFound
	}
	lvl := LevelForBond(st.Bond)
	skins := mergeUnlockedSkins([]string(st.UnlockedSkins), st.Bond)
	// The favorite quote is a nice-to-have; a stats failure must not hide the
	// profile card.
	favorite, _ := s.repo.MostFrequentEventType(ctx, userID)
	return &PublicStateView{
		UserID:            st.UserID,
		Bond:              st.Bond,
		Level:             lvl.Level,
		LevelName:         lvl.Name,
		NextThreshold:     lvl.Next,
		EquippedSkin:      st.EquippedSkin,
		UnlockedSkins:     skins,
		PublicProfile:     st.PublicProfile,
		DaysTogether:      daysTogether(st.CreatedAt, time.Now()),
		FavoriteEventType: favorite,
	}, nil
}

// daysTogether counts local calendar days since createdAt, with the creation
// day itself counting as day 1. The local civil date is used (not elapsed UTC
// hours) so a summary read late in the day still reports one day together, and
// the difference is computed on UTC midnights to stay correct across DST.
func daysTogether(createdAt, now time.Time) int {
	cy, cm, cd := createdAt.In(time.Local).Date()
	ny, nm, nd := now.In(time.Local).Date()
	created := time.Date(cy, cm, cd, 0, 0, 0, 0, time.UTC)
	today := time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)
	days := int(today.Sub(created).Hours()/24) + 1
	if days < 1 {
		return 1
	}
	return days
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

const personalityMaxRunes = 500

func validatePersonality(prompt string) error {
	if utf8.RuneCountInString(prompt) > personalityMaxRunes {
		return httpx.ErrValidation("性格提示词不能超过 500 个字符")
	}
	return nil
}

func validateEquippedSkinAvailable(skin string, available []string) error {
	if skin == "" {
		return nil
	}
	if !skinAvailable(available, skin) {
		return httpx.ErrValidation("该换装尚未解锁")
	}
	return nil
}

// UpdateSettings persists the whole user-editable settings set and returns the
// fresh view. It validates through the same helpers as UpdateSettingsPatch.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in SettingsInput) (*StateView, error) {
	if err := validateActivityLevel(in.ActivityLevel); err != nil {
		return nil, err
	}
	if err := validatePersonality(in.PersonalityPrompt); err != nil {
		return nil, err
	}
	if err := validateEquippedSkin(in.EquippedSkin); err != nil {
		return nil, err
	}
	st, err := s.repo.EnsureState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := validateEquippedSkinAvailable(in.EquippedSkin, mergeUnlockedSkins([]string(st.UnlockedSkins), st.Bond)); err != nil {
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
	if patch.PersonalityPrompt != nil {
		if err := validatePersonality(*patch.PersonalityPrompt); err != nil {
			return nil, err
		}
	}
	if patch.EquippedSkin != nil {
		if err := validateEquippedSkin(*patch.EquippedSkin); err != nil {
			return nil, err
		}
		if *patch.EquippedSkin != "" {
			st, err := s.repo.EnsureState(ctx, userID)
			if err != nil {
				return nil, err
			}
			if err := validateEquippedSkinAvailable(*patch.EquippedSkin, mergeUnlockedSkins([]string(st.UnlockedSkins), st.Bond)); err != nil {
				return nil, err
			}
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

var validPetExpressions = map[string]struct{}{
	"neutral": {}, "happy": {}, "shy": {}, "pout": {},
	"surprised": {}, "sleepy": {}, "smug": {},
}

var validGreetingKeys = map[string]struct{}{
	"greet.morning": {}, "greet.afternoon": {}, "greet.night": {},
}

func normalizeRecentLines(lines []string) []string {
	out := make([]string, 0, 3)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > 80 {
			line = string([]rune(line)[:80])
		}
		out = append(out, line)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func defaultLineKey(req DialogueRequest) string {
	switch req.Trigger {
	case "pet":
		if _, ok := validPetExpressions[req.Expression]; ok {
			return "pet." + req.Expression
		}
		return "pet"
	case "greet":
		if _, ok := validGreetingKeys[req.GreetingKey]; ok {
			return req.GreetingKey
		}
		return "greeting"
	case "idle":
		return "idle.lively"
	default:
		return "greeting"
	}
}

func defaultDialogue(req DialogueRequest, reason string, remaining int) *DialogueResult {
	return &DialogueResult{
		Source:         "default",
		LineKey:        defaultLineKey(req),
		Reason:         reason,
		RemainingToday: remaining,
	}
}

func (s *Service) companionName(ctx context.Context) string {
	if s.nameProvider != nil {
		name, err := s.nameProvider.CompanionName(ctx)
		if err != nil {
			slog.Warn("companion: resolve display name failed", "err", err)
		} else if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return "Koj"
}

// Dialogue returns an AI-generated companion line or a static fallback key.
// AI failures and quota limits deliberately return a 200-friendly result; only
// storage errors are propagated.
func (s *Service) Dialogue(ctx context.Context, userID int64, req DialogueRequest) (*DialogueResult, error) {
	st, err := s.repo.EnsureState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !st.AIDialogueEnabled {
		return defaultDialogue(req, "disabled", 0), nil
	}
	if s.dialogue == nil || s.aiStatus == nil {
		return defaultDialogue(req, "no_ai", 0), nil
	}
	ready, err := s.aiStatus.HasActiveModel(ctx, userID)
	if err != nil {
		slog.Warn("companion: check AI status failed", "err", err)
		return defaultDialogue(req, "no_ai", 0), nil
	}
	if !ready {
		return defaultDialogue(req, "no_ai", 0), nil
	}

	now := time.Now()
	date := now.In(time.Local).Format("2006-01-02")
	allowed, remaining, err := s.repo.ReserveAIDialogue(ctx, userID, date, now, aiDialogueCooldown, aiDialogueDailyCap)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return defaultDialogue(req, "rate_limited", remaining), nil
	}

	level := LevelForBond(st.Bond)
	messages := BuildDialoguePrompt(PromptInput{
		Name:        s.companionName(ctx),
		Personality: st.PersonalityPrompt,
		Trigger:     req.Trigger,
		Expression:  req.Expression,
		GreetingKey: req.GreetingKey,
		Level:       level.Level,
		LevelName:   level.Name,
		RecentLines: normalizeRecentLines(req.RecentLines),
	})
	chatCtx, cancel := context.WithTimeout(ctx, aiDialogueTimeout)
	defer cancel()
	line, err := s.dialogue.Chat(chatCtx, userID, messages, aiDialogueMaxTokens)
	if err != nil {
		reason := "upstream_error"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "timeout"
		}
		slog.Warn("companion: AI dialogue failed", "reason", reason, "err", err)
		return defaultDialogue(req, reason, remaining), nil
	}
	clean, ok := CleanDialogueLine(line)
	if !ok {
		return defaultDialogue(req, "invalid_output", remaining), nil
	}
	return &DialogueResult{Source: "ai", Line: clean, RemainingToday: remaining}, nil
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
