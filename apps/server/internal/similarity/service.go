package similarity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

const (
	// engineName and engineVersion are recorded on every run so results remain
	// attributable to a specific algorithm version.
	engineName    = "winnowing-v1"
	engineVersion = "1"
)

const (
	// maxBaseCodeBytes caps the optional per-problem template size.
	maxBaseCodeBytes = 64 * 1024
	// maxErrorMessageRunes caps the persisted engine error message.
	maxErrorMessageRunes = 500
)

// ContestLookup reads contests and their problems for authorization checks.
type ContestLookup interface {
	FindByID(ctx context.Context, id int64) (*contest.Contest, error)
	FindProblems(ctx context.Context, contestID int64) ([]contest.ProblemRow, error)
}

// Enqueuer schedules a run for asynchronous processing.
type Enqueuer interface {
	Enqueue(ctx context.Context, runID int64) error
}

// Viewer is the acting user's identity and permission set.
type Viewer struct {
	UserID    int64
	CanManage bool
}

// Service implements similarity settings, runs and results.
type Service struct {
	repo     Repository
	contests ContestLookup
	engine   Engine
	queue    Enqueuer
}

// NewService returns a Service bound to the given dependencies.
func NewService(repo Repository, contests ContestLookup, engine Engine, queue Enqueuer) *Service {
	return &Service{repo: repo, contests: contests, engine: engine, queue: queue}
}

// requireManage rejects viewers without the contest management permission.
func requireManage(v Viewer) error {
	if !v.CanManage {
		return httpx.ErrForbidden
	}
	return nil
}

// requireEnded rejects contests that have not ended yet.
func requireEnded(c *contest.Contest) error {
	if contest.StatusAt(c.StartTime, c.EndTime, time.Now()) != contest.StatusEnded {
		return httpx.ErrValidation("比赛结束后才能查重")
	}
	return nil
}

// findEndedContest loads a contest and enforces the ended requirement.
func (s *Service) findEndedContest(ctx context.Context, id int64) (*contest.Contest, error) {
	c, err := s.contests.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, contest.ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if err := requireEnded(c); err != nil {
		return nil, err
	}
	return c, nil
}

// problemInContest reports whether problemID belongs to the contest.
func (s *Service) problemInContest(ctx context.Context, contestID, problemID int64) error {
	problems, err := s.contests.FindProblems(ctx, contestID)
	if err != nil {
		return err
	}
	for _, p := range problems {
		if p.ProblemID == problemID {
			return nil
		}
	}
	return httpx.ErrValidation("题目不在该比赛中")
}

// settingsOrDefault returns the saved settings or the defaults when the
// problem has none yet.
func (s *Service) settingsOrDefault(ctx context.Context, contestID, problemID int64) (*Settings, error) {
	st, err := s.repo.GetSettings(ctx, contestID, problemID)
	if err == nil {
		return st, nil
	}
	if errors.Is(err, ErrNotFound) {
		return &Settings{
			ContestID:        contestID,
			ProblemID:        problemID,
			ThresholdPercent: DefaultThresholdPercent,
			IgnoreCommonCode: true,
		}, nil
	}
	return nil, err
}

// Overview returns the per-problem settings and latest run for an ended
// contest.
func (s *Service) Overview(ctx context.Context, v Viewer, contestID int64) (*Overview, error) {
	if err := requireManage(v); err != nil {
		return nil, err
	}
	if _, err := s.findEndedContest(ctx, contestID); err != nil {
		return nil, err
	}
	problems, err := s.contests.FindProblems(ctx, contestID)
	if err != nil {
		return nil, err
	}
	settings, err := s.repo.ListSettings(ctx, contestID)
	if err != nil {
		return nil, err
	}
	runs, err := s.repo.LatestRuns(ctx, contestID)
	if err != nil {
		return nil, err
	}

	settingsByProblem := make(map[int64]Settings, len(settings))
	for _, st := range settings {
		settingsByProblem[st.ProblemID] = st
	}

	views := make([]ProblemView, 0, len(problems))
	for _, p := range problems {
		view := ProblemView{
			ProblemID:        p.ProblemID,
			DisplayID:        p.DisplayID,
			Title:            p.ProblemTitle,
			Slug:             p.ProblemSlug,
			ThresholdPercent: DefaultThresholdPercent,
			IgnoreCommonCode: true,
		}
		if st, ok := settingsByProblem[p.ProblemID]; ok {
			view.ThresholdPercent = st.ThresholdPercent
			view.IgnoreCommonCode = st.IgnoreCommonCode
			view.BaseLanguage = st.BaseLanguage
		}
		if run := runs[p.ProblemID]; run != nil {
			rv := toRunView(*run)
			view.LatestRun = &rv
			view.SubmissionCount = run.SubmissionCount
			view.MatchCount = run.MatchCount
		}
		views = append(views, view)
	}
	return &Overview{ContestID: contestID, Problems: views}, nil
}

// SaveSettings validates and upserts one problem's similarity settings.
func (s *Service) SaveSettings(ctx context.Context, v Viewer, contestID, problemID int64, in SettingsInput) (*Settings, error) {
	if err := requireManage(v); err != nil {
		return nil, err
	}
	if _, err := s.findEndedContest(ctx, contestID); err != nil {
		return nil, err
	}
	if err := s.problemInContest(ctx, contestID, problemID); err != nil {
		return nil, err
	}
	if in.ThresholdPercent < 1 || in.ThresholdPercent > 100 {
		return nil, httpx.ErrValidation("阈值必须在 1 到 100 之间")
	}

	baseCode := strings.TrimSpace(in.BaseCode)
	baseLanguage := strings.TrimSpace(in.BaseLanguage)
	if baseCode != "" {
		lang := submission.Language(baseLanguage)
		if !supportedLanguage(lang) {
			return nil, httpx.ErrValidation("模板语言无效")
		}
		if len(baseCode) > maxBaseCodeBytes {
			return nil, httpx.ErrValidation("模板代码不能超过 64 KiB")
		}
	} else {
		// No template means no template language either.
		baseLanguage = ""
	}

	settings := &Settings{
		ContestID:        contestID,
		ProblemID:        problemID,
		ThresholdPercent: in.ThresholdPercent,
		IgnoreCommonCode: in.IgnoreCommonCode,
		BaseLanguage:     baseLanguage,
		BaseCode:         baseCode,
		UpdatedBy:        v.UserID,
	}
	if err := s.repo.UpsertSettings(ctx, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

// StartRuns creates and enqueues a run for one problem or every problem.
// An existing queued/running run is returned instead of a duplicate.
func (s *Service) StartRuns(ctx context.Context, v Viewer, contestID, problemID int64) ([]RunView, error) {
	if err := requireManage(v); err != nil {
		return nil, err
	}
	if _, err := s.findEndedContest(ctx, contestID); err != nil {
		return nil, err
	}
	problems, err := s.contests.FindProblems(ctx, contestID)
	if err != nil {
		return nil, err
	}

	var target []contest.ProblemRow
	if problemID > 0 {
		for _, p := range problems {
			if p.ProblemID == problemID {
				target = []contest.ProblemRow{p}
				break
			}
		}
		if len(target) == 0 {
			return nil, httpx.ErrValidation("题目不在该比赛中")
		}
	} else {
		target = problems
	}

	views := make([]RunView, 0, len(target))
	for _, p := range target {
		active, err := s.repo.FindActiveRun(ctx, contestID, p.ProblemID)
		if err != nil {
			return nil, err
		}
		if active != nil {
			views = append(views, toRunView(*active))
			continue
		}
		st, err := s.settingsOrDefault(ctx, contestID, p.ProblemID)
		if err != nil {
			return nil, err
		}
		run := &Run{
			ContestID:        contestID,
			ProblemID:        p.ProblemID,
			Status:           RunStatusQueued,
			ThresholdPercent: st.ThresholdPercent,
			IgnoreCommonCode: st.IgnoreCommonCode,
			Engine:           engineName,
			EngineVersion:    engineVersion,
			RequestedBy:      v.UserID,
		}
		if err := s.repo.CreateRun(ctx, run); err != nil {
			return nil, err
		}
		if err := s.queue.Enqueue(ctx, run.ID); err != nil {
			return nil, err
		}
		views = append(views, toRunView(*run))
	}
	return views, nil
}

// Run returns a single run belonging to the contest.
func (s *Service) Run(ctx context.Context, v Viewer, contestID, runID int64) (*RunView, error) {
	if err := requireManage(v); err != nil {
		return nil, err
	}
	if _, err := s.findEndedContest(ctx, contestID); err != nil {
		return nil, err
	}
	run, err := s.repo.FindRun(ctx, runID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if run.ContestID != contestID {
		return nil, httpx.ErrNotFound
	}
	rv := toRunView(*run)
	return &rv, nil
}

// Matches returns a page of match results from the latest completed runs.
func (s *Service) Matches(ctx context.Context, v Viewer, contestID int64, problemID, runID int64, page, pageSize int) (MatchPage, error) {
	if err := requireManage(v); err != nil {
		return MatchPage{}, err
	}
	if _, err := s.findEndedContest(ctx, contestID); err != nil {
		return MatchPage{}, err
	}
	rows, total, err := s.repo.ListMatches(ctx, MatchFilter{
		ContestID: contestID, ProblemID: problemID, RunID: runID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		return MatchPage{}, err
	}
	items := make([]MatchView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toMatchView(row))
	}
	return MatchPage{Total: total, Items: items}, nil
}

// ExportCSV returns every match of the latest completed run per problem as a
// CSV document encoded as UTF-8 with a BOM.
func (s *Service) ExportCSV(ctx context.Context, v Viewer, contestID, problemID int64) ([]byte, error) {
	if err := requireManage(v); err != nil {
		return nil, err
	}
	c, err := s.findEndedContest(ctx, contestID)
	if err != nil {
		return nil, err
	}
	problems, err := s.contests.FindProblems(ctx, contestID)
	if err != nil {
		return nil, err
	}
	problemsByID := make(map[int64]contest.ProblemRow, len(problems))
	for _, p := range problems {
		problemsByID[p.ProblemID] = p
	}

	// ListMatches caps page sizes, so walk its pages to collect every row from
	// the latest completed runs.
	rows := make([]MatchRow, 0)
	for page := 1; ; page++ {
		batch, total, err := s.repo.ListMatches(ctx, MatchFilter{
			ContestID: contestID, ProblemID: problemID, Page: page, PageSize: csvPageSize,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, batch...)
		if len(batch) == 0 || int64(len(rows)) >= total {
			break
		}
	}
	return buildMatchesCSV(c, problemsByID, rows)
}

// MatchDetail returns both sides of a match, rejecting matches that belong to
// a different contest.
func (s *Service) MatchDetail(ctx context.Context, v Viewer, contestID, matchID int64) (*MatchDetail, error) {
	if err := requireManage(v); err != nil {
		return nil, err
	}
	if _, err := s.findEndedContest(ctx, contestID); err != nil {
		return nil, err
	}
	row, err := s.repo.FindMatch(ctx, matchID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if row.ContestID != contestID {
		return nil, httpx.ErrNotFound
	}
	mv := matchViewFromRow(row.Match, row.OriginalUsername, row.OriginalNickname, row.SuspectUsername, row.SuspectNickname)
	mv.ThresholdPercent = row.ThresholdPercent
	mv.FinishedAt = row.FinishedAt
	return &MatchDetail{
		MatchView:      mv,
		OriginalCode:   row.OriginalCode,
		SuspectCode:    row.SuspectCode,
		OriginalStatus: row.OriginalStatus,
		SuspectStatus:  row.SuspectStatus,
	}, nil
}

// ProcessRun runs one queued/running run to completion, or returns nil when
// the run is already finished (idempotent). Engine failures are persisted on
// the run and returned.
func (s *Service) ProcessRun(ctx context.Context, runID int64) error {
	run, err := s.repo.FindRun(ctx, runID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if run.Status != RunStatusQueued && run.Status != RunStatusRunning {
		return nil
	}

	started := time.Now().UTC()
	run.Status = RunStatusRunning
	run.StartedAt = &started
	if err := s.repo.UpdateRun(ctx, run); err != nil {
		return err
	}

	settings, err := s.settingsOrDefault(ctx, run.ContestID, run.ProblemID)
	if err != nil {
		return s.failRun(ctx, run, err)
	}
	rows, err := s.repo.ListProblemSubmissions(ctx, run.ContestID, run.ProblemID)
	if err != nil {
		return s.failRun(ctx, run, err)
	}
	candidates := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, Candidate{
			ID:        row.ID,
			UserID:    row.UserID,
			Username:  row.Username,
			Nickname:  row.Nickname,
			Language:  row.Language,
			Code:      row.Code,
			CreatedAt: row.CreatedAt,
		})
	}

	result, err := s.engine.Compare(ctx, EngineRequest{
		Candidates:       candidates,
		ThresholdPercent: float64(settings.ThresholdPercent),
		BaseCode:         settings.BaseCode,
		BaseLanguage:     submission.Language(settings.BaseLanguage),
		IgnoreCommonCode: settings.IgnoreCommonCode,
	})
	if err != nil {
		return s.failRun(ctx, run, err)
	}

	matches := make([]Match, 0, len(result.Pairs))
	for _, p := range result.Pairs {
		matches = append(matches, Match{
			ContestID:            run.ContestID,
			ProblemID:            run.ProblemID,
			OriginalSubmissionID: p.OriginalSubmissionID,
			SuspectSubmissionID:  p.SuspectSubmissionID,
			OriginalUserID:       p.OriginalUserID,
			SuspectUserID:        p.SuspectUserID,
			Language:             p.Language,
			SimilarityPercent:    p.SimilarityPercent,
			DicePercent:          p.DicePercent,
			SharedFingerprints:   p.SharedFingerprints,
			OriginalFingerprints: p.OriginalFingerprints,
			SuspectFingerprints:  p.SuspectFingerprints,
			OriginalCreatedAt:    p.OriginalCreatedAt,
			SuspectCreatedAt:     p.SuspectCreatedAt,
		})
	}
	if err := s.repo.ReplaceMatches(ctx, run.ID, matches); err != nil {
		return s.failRun(ctx, run, err)
	}

	finished := time.Now().UTC()
	run.Status = RunStatusCompleted
	run.SubmissionCount = result.SubmissionCount
	run.MatchCount = len(matches)
	run.ErrorMessage = ""
	run.FinishedAt = &finished
	return s.repo.UpdateRun(ctx, run)
}

// failRun marks the run failed with a truncated error message and returns the
// original error.
func (s *Service) failRun(ctx context.Context, run *Run, err error) error {
	finished := time.Now().UTC()
	run.Status = RunStatusFailed
	run.ErrorMessage = truncateMessage(err.Error())
	run.FinishedAt = &finished
	if updateErr := s.repo.UpdateRun(ctx, run); updateErr != nil {
		return updateErr
	}
	return err
}

func truncateMessage(msg string) string {
	runes := []rune(msg)
	if len(runes) > maxErrorMessageRunes {
		runes = runes[:maxErrorMessageRunes]
	}
	return string(runes)
}

func toRunView(r Run) RunView {
	return RunView{
		ID:               r.ID,
		ContestID:        r.ContestID,
		ProblemID:        r.ProblemID,
		Status:           r.Status,
		ThresholdPercent: r.ThresholdPercent,
		IgnoreCommonCode: r.IgnoreCommonCode,
		Engine:           r.Engine,
		EngineVersion:    r.EngineVersion,
		RequestedBy:      r.RequestedBy,
		SubmissionCount:  r.SubmissionCount,
		MatchCount:       r.MatchCount,
		ErrorMessage:     r.ErrorMessage,
		StartedAt:        r.StartedAt,
		FinishedAt:       r.FinishedAt,
		CreatedAt:        r.CreatedAt,
	}
}

func matchViewFromRow(m Match, originalUsername, originalNickname, suspectUsername, suspectNickname string) MatchView {
	return MatchView{
		ID:                   m.ID,
		RunID:                m.RunID,
		ContestID:            m.ContestID,
		ProblemID:            m.ProblemID,
		OriginalSubmissionID: m.OriginalSubmissionID,
		SuspectSubmissionID:  m.SuspectSubmissionID,
		OriginalUserID:       m.OriginalUserID,
		OriginalUsername:     originalUsername,
		OriginalNickname:     originalNickname,
		SuspectUserID:        m.SuspectUserID,
		SuspectUsername:      suspectUsername,
		SuspectNickname:      suspectNickname,
		Language:             m.Language,
		SimilarityPercent:    m.SimilarityPercent,
		DicePercent:          m.DicePercent,
		OriginalCreatedAt:    m.OriginalCreatedAt,
		SuspectCreatedAt:     m.SuspectCreatedAt,
	}
}

func toMatchView(row MatchRow) MatchView {
	mv := matchViewFromRow(row.Match, row.OriginalUsername, row.OriginalNickname, row.SuspectUsername, row.SuspectNickname)
	mv.ThresholdPercent = row.ThresholdPercent
	mv.FinishedAt = row.FinishedAt
	return mv
}
