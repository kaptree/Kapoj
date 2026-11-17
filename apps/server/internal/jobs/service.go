package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	// enqueueTimeout bounds the detached enqueue call so a canceled HTTP
	// request cannot leave a created run unqueued.
	enqueueTimeout = 5 * time.Second
	// finishTimeout bounds terminal-state writes after the handler returns.
	finishTimeout = 5 * time.Second
	// enqueueFailureMessage is persisted when a run cannot be queued.
	enqueueFailureMessage = "任务入队失败，请稍后重试"
	// staleRunMessage is persisted when a run is reconciled as stale.
	staleRunMessage = "任务执行超时，已标记为失败"
)

// DefaultStaleAfter bounds how long a run may go without a heartbeat before
// ReconcileStale fails it. Handlers refresh the heartbeat through Reporter,
// and the Asynq worker timeout is 30 minutes, so 15 minutes of silence means
// the worker is gone rather than the batch still being productive.
const DefaultStaleAfter = 15 * time.Minute

// Enqueuer schedules a run for asynchronous execution.
type Enqueuer interface {
	Enqueue(ctx context.Context, runID int64) error
}

// Service owns scheduled job definitions, their persisted state and runs.
type Service struct {
	db    *gorm.DB
	queue Enqueuer

	mu   sync.RWMutex
	defs map[string]Definition
}

// NewService returns a Service backed by db that enqueues runs through queue.
func NewService(db *gorm.DB, enqueuer Enqueuer) *Service {
	return &Service{db: db, queue: enqueuer, defs: make(map[string]Definition)}
}

// Register adds or replaces a job definition. Registration normally happens at
// startup before Ensure and RunScheduler are called.
func (s *Service) Register(def Definition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.defs == nil {
		s.defs = make(map[string]Definition)
	}
	s.defs[def.Key] = def
}

// NextRun parses a standard five-field cron expression in timezone and returns
// the first occurrence strictly after the given time.
func NextRun(cronExpr, timezone string, after time.Time) (time.Time, error) {
	tz := strings.TrimSpace(timezone)
	if tz == "" {
		return time.Time{}, httpx.ErrValidation("时区不能为空")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, httpx.ErrValidation("时区无效")
	}
	expr := strings.TrimSpace(cronExpr)
	if expr == "" {
		return time.Time{}, httpx.ErrValidation("cron 表达式不能为空")
	}
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return time.Time{}, httpx.ErrValidation("cron 表达式无效")
	}
	// ParseStandard evaluates in time.Local; pin the parsed schedule to the
	// job's configured location instead.
	if spec, ok := schedule.(*cron.SpecSchedule); ok {
		spec.Location = loc
	}
	next := schedule.Next(after.In(loc))
	if next.IsZero() {
		return time.Time{}, httpx.ErrValidation("cron 表达式没有可执行时间")
	}
	return next, nil
}

// Ensure inserts registered definitions once and fills in missing next_run_at
// values. Existing rows keep their admin-edited cron, timezone and enabled
// flag.
func (s *Service) Ensure(ctx context.Context) error {
	defs := s.definitions()
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, def := range defs {
			key := strings.TrimSpace(def.Key)
			if key == "" {
				return errors.New("jobs: definition key is empty")
			}
			name := strings.TrimSpace(def.Name)
			if name == "" {
				name = key
			}
			tz := strings.TrimSpace(def.Timezone)
			if tz == "" {
				tz = DefaultTimezone
			}
			next, err := NextRun(def.DefaultCron, tz, now)
			if err != nil {
				return fmt.Errorf("jobs: definition %q: %w", key, err)
			}
			row := Job{
				Key:         key,
				Name:        name,
				Description: def.Description,
				CronExpr:    strings.TrimSpace(def.DefaultCron),
				Timezone:    tz,
				Enabled:     true,
				NextRunAt:   &next,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoNothing: true,
			}).Create(&row).Error; err != nil {
				return fmt.Errorf("jobs: seed %q: %w", key, err)
			}
		}

		var pending []Job
		if err := tx.Where("enabled = true AND next_run_at IS NULL").Find(&pending).Error; err != nil {
			return err
		}
		for i := range pending {
			next, err := NextRun(pending[i].CronExpr, pending[i].Timezone, now)
			if err != nil {
				return fmt.Errorf("jobs: job %q: %w", pending[i].Key, err)
			}
			if err := tx.Model(&Job{}).Where("id = ?", pending[i].ID).
				Update("next_run_at", next).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ClaimDue atomically claims due jobs, advances their schedule and creates one
// queued run per claimed job. Jobs with an active run are rescheduled instead
// of creating a second run. The caller enqueues the returned runs after the
// transaction commits.
func (s *Service) ClaimDue(ctx context.Context, now time.Time) ([]Run, error) {
	runs := make([]Run, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var due []Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("enabled = true AND next_run_at IS NOT NULL AND next_run_at <= ?", now).
			Order("next_run_at, id").Find(&due).Error; err != nil {
			return err
		}
		if len(due) == 0 {
			return nil
		}

		keys := make([]string, len(due))
		for i := range due {
			keys[i] = due[i].Key
		}
		var active []Run
		if err := tx.Where("job_key IN ? AND status IN ?", keys, activeStatuses).Find(&active).Error; err != nil {
			return err
		}
		activeKeys := make(map[string]struct{}, len(active))
		for i := range active {
			activeKeys[active[i].JobKey] = struct{}{}
		}

		for i := range due {
			job := &due[i]
			next, err := NextRun(job.CronExpr, job.Timezone, now)
			if err != nil {
				return fmt.Errorf("jobs: job %q: %w", job.Key, err)
			}
			if _, busy := activeKeys[job.Key]; busy {
				if err := tx.Model(&Job{}).Where("id = ?", job.ID).Updates(map[string]any{
					"next_run_at": next,
					"updated_at":  now,
				}).Error; err != nil {
					return err
				}
				continue
			}
			run := Run{
				JobKey:    job.Key,
				Trigger:   TriggerSchedule,
				Status:    StatusQueued,
				StatsJSON: map[string]any{},
				CreatedAt: now,
			}
			if err := tx.Create(&run).Error; err != nil {
				return err
			}
			if err := tx.Model(&Job{}).Where("id = ?", job.ID).Updates(map[string]any{
				"next_run_at": next,
				"last_run_at": now,
				"last_status": StatusQueued,
				"last_error":  "",
				"updated_at":  now,
			}).Error; err != nil {
				return err
			}
			runs = append(runs, run)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

// List returns every registered job with its active run, ordered by key.
func (s *Service) List(ctx context.Context) ([]JobView, error) {
	var rows []Job
	if err := s.db.WithContext(ctx).Order("key").Find(&rows).Error; err != nil {
		return nil, err
	}
	var active []Run
	if len(rows) > 0 {
		if err := s.db.WithContext(ctx).Where("status IN ?", activeStatuses).
			Order("id").Find(&active).Error; err != nil {
			return nil, err
		}
	}
	activeByKey := make(map[string]*Run, len(active))
	for i := range active {
		activeByKey[active[i].JobKey] = &active[i]
	}
	views := make([]JobView, 0, len(rows))
	for i := range rows {
		views = append(views, jobView(rows[i], activeByKey[rows[i].Key]))
	}
	return views, nil
}

// Update validates and stores the admin-editable schedule of one job. The
// audit callback runs inside the same transaction, so an audit failure rolls
// the update back.
func (s *Service) Update(ctx context.Context, actorID int64, key string, in JobInput, audit func(*gorm.DB) error) (*JobView, error) {
	cronExpr := strings.TrimSpace(in.CronExpr)
	tz := strings.TrimSpace(in.Timezone)
	now := time.Now().UTC()

	// Validate cron and timezone even when disabling the job so a broken
	// schedule cannot be saved.
	next, err := NextRun(cronExpr, tz, now)
	if err != nil {
		return nil, err
	}
	var nextRunAt *time.Time
	if in.Enabled {
		nextRunAt = &next
	}

	var job Job
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("key = ?", key).First(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}
		if err := tx.Model(&Job{}).Where("id = ?", job.ID).Updates(map[string]any{
			"enabled":     in.Enabled,
			"cron_expr":   cronExpr,
			"timezone":    tz,
			"next_run_at": nextRunAt,
			"updated_by":  actorID,
			"updated_at":  now,
		}).Error; err != nil {
			return fmt.Errorf("jobs: update %q: %w", key, err)
		}
		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}
		var updated Job
		if err := tx.First(&updated, job.ID).Error; err != nil {
			return err
		}
		job = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	active, err := s.activeRun(ctx, job.Key)
	if err != nil {
		return nil, err
	}
	view := jobView(job, active)
	return &view, nil
}

// CreateManualRun creates a queued manual run for one job and enqueues it. A
// job that already has a queued or running run rejects the request with
// httpx.ErrJobRunning.
func (s *Service) CreateManualRun(ctx context.Context, actorID int64, key string, audit func(*gorm.DB) error) (*RunView, error) {
	var run Run
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("key = ?", key).First(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}
		var active int64
		if err := tx.Model(&Run{}).
			Where("job_key = ? AND status IN ?", key, activeStatuses).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return httpx.ErrJobRunning
		}
		run = Run{
			JobKey:    key,
			Trigger:   TriggerManual,
			Status:    StatusQueued,
			StatsJSON: map[string]any{},
			CreatedBy: actorID,
			CreatedAt: time.Now().UTC(),
		}
		if err := tx.Create(&run).Error; err != nil {
			if isActiveRunConflict(err) {
				return httpx.ErrJobRunning
			}
			return fmt.Errorf("jobs: create manual run: %w", err)
		}
		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.enqueue(ctx, &run); err != nil {
		return nil, err
	}
	view := runView(run)
	return &view, nil
}

// ListRuns returns a page of runs, newest first.
func (s *Service) ListRuns(ctx context.Context, f RunFilter, page, pageSize int) (httpx.Page[RunView], error) {
	page, pageSize = normalizePage(page, pageSize)
	query := s.db.WithContext(ctx).Model(&Run{})
	if f.JobKey != "" {
		query = query.Where("job_key = ?", f.JobKey)
	}
	if f.Status != "" {
		query = query.Where("status = ?", f.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return httpx.Page[RunView]{}, err
	}
	var rows []Run
	if err := query.Order("created_at DESC").Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return httpx.Page[RunView]{}, err
	}
	items := make([]RunView, 0, len(rows))
	for i := range rows {
		items = append(items, runView(rows[i]))
	}
	return httpx.Page[RunView]{Total: total, Items: items}, nil
}

// GetRun returns one run by ID.
func (s *Service) GetRun(ctx context.Context, id int64) (*RunView, error) {
	var run Run
	if err := s.db.WithContext(ctx).First(&run, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	view := runView(run)
	return &view, nil
}

// Execute runs one queued run. Terminal or already-running runs are ignored so
// duplicate queue deliveries cannot execute a handler twice.
func (s *Service) Execute(ctx context.Context, runID int64) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).Model(&Run{}).
		Where("id = ? AND status = ?", runID, StatusQueued).
		Updates(map[string]any{
			"status":       StatusRunning,
			"started_at":   now,
			"heartbeat_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var existing Run
		if err := s.db.WithContext(ctx).First(&existing, runID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("jobs: run %d not found", runID)
			}
			return err
		}
		return nil
	}

	var run Run
	if err := s.db.WithContext(ctx).First(&run, runID).Error; err != nil {
		return err
	}
	run.Status = StatusRunning
	run.StartedAt = &now
	if err := s.db.WithContext(ctx).Model(&Job{}).Where("key = ?", run.JobKey).Updates(map[string]any{
		"last_run_at": now,
		"last_status": StatusRunning,
		"last_error":  "",
		"updated_at":  now,
	}).Error; err != nil {
		slog.Warn("jobs: mark job running failed", "job", run.JobKey, "err", err)
	}

	def, ok := s.definition(run.JobKey)
	if !ok || def.Handler == nil {
		msg := "任务处理器未注册"
		_ = s.finishRun(ctx, &run, StatusFailed, msg)
		return errors.New(msg)
	}

	reporter := &runReporter{db: s.db, runID: run.ID}
	execErr := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("任务执行异常: %v", r)
			}
		}()
		return def.Handler(ctx, &run, reporter)
	}()
	if execErr != nil {
		_ = s.finishRun(ctx, &run, StatusFailed, execErr.Error())
		return execErr
	}
	return s.finishRun(ctx, &run, StatusSucceeded, "")
}

// ReconcileStale marks abandoned runs as failed and frees their job for the
// next run. Queued runs are stale once their queue wait exceeds olderThan; a
// running run is stale only after its heartbeat (falling back to started_at,
// then created_at for rows written before heartbeats existed) expires.
func (s *Service) ReconcileStale(ctx context.Context, olderThan time.Duration) error {
	if olderThan <= 0 {
		olderThan = DefaultStaleAfter
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stale []Run
		if err := tx.Where(
			"(status = ? AND created_at < ?) OR "+
				"(status = ? AND COALESCE(heartbeat_at, started_at, created_at) < ?)",
			StatusQueued, cutoff, StatusRunning, cutoff,
		).Find(&stale).Error; err != nil {
			return err
		}
		if len(stale) == 0 {
			return nil
		}
		ids := make([]int64, len(stale))
		keys := make([]string, 0, len(stale))
		seen := make(map[string]struct{}, len(stale))
		for i := range stale {
			ids[i] = stale[i].ID
			if _, ok := seen[stale[i].JobKey]; !ok {
				seen[stale[i].JobKey] = struct{}{}
				keys = append(keys, stale[i].JobKey)
			}
		}
		now := time.Now().UTC()
		// Only rows that are still active are failed. A run that finished
		// between the scan and this update must keep its terminal status.
		res := tx.Model(&Run{}).
			Where("id IN ? AND status IN ?", ids, activeStatuses).
			Updates(map[string]any{
				"status":        StatusFailed,
				"error_message": staleRunMessage,
				"finished_at":   now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&Job{}).
			Where("key IN ? AND last_status IN ?", keys, activeStatuses).
			Updates(map[string]any{
				"last_status": StatusFailed,
				"last_error":  staleRunMessage,
				"updated_at":  now,
			}).Error
	})
}

func (s *Service) definitions() []Definition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	defs := make([]Definition, 0, len(s.defs))
	for _, def := range s.defs {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Key < defs[j].Key })
	return defs
}

func (s *Service) definition(key string) (Definition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	def, ok := s.defs[key]
	return def, ok
}

func (s *Service) activeRun(ctx context.Context, key string) (*Run, error) {
	var run Run
	if err := s.db.WithContext(ctx).
		Where("job_key = ? AND status IN ?", key, activeStatuses).
		Order("id").First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (s *Service) enqueue(ctx context.Context, run *Run) error {
	if s.queue == nil {
		msg := "任务队列未配置"
		_ = s.failQueuedRun(ctx, run, msg)
		return errors.New("jobs: queue is not configured")
	}
	enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), enqueueTimeout)
	defer cancel()
	if err := s.queue.Enqueue(enqueueCtx, run.ID); err != nil {
		_ = s.failQueuedRun(ctx, run, enqueueFailureMessage)
		return err
	}
	return nil
}

// failQueuedRun marks a run that never reached a worker as failed. Only a
// still-queued row may transition, so a late enqueue failure cannot overwrite
// a run that another worker already started or the reconciler already failed.
func (s *Service) failQueuedRun(ctx context.Context, run *Run, errMsg string) error {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()
	now := time.Now().UTC()
	return s.db.WithContext(finishCtx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Run{}).Where("id = ? AND status = ?", run.ID, StatusQueued).
			Updates(map[string]any{
				"status":        StatusFailed,
				"error_message": errMsg,
				"finished_at":   now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&Job{}).Where("key = ?", run.JobKey).Updates(map[string]any{
			"last_status": StatusFailed,
			"last_error":  errMsg,
			"updated_at":  now,
		}).Error
	})
}

// finishRun records the terminal state of a run that is still running. A run
// reconciled as stale is already failed, so a late handler completion is a
// no-op and must not flip the status or rewrite the job summary.
func (s *Service) finishRun(ctx context.Context, run *Run, status, errMsg string) error {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()
	now := time.Now().UTC()
	return s.db.WithContext(finishCtx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Run{}).Where("id = ? AND status = ?", run.ID, StatusRunning).
			Updates(map[string]any{
				"status":        status,
				"error_message": errMsg,
				"finished_at":   now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&Job{}).Where("key = ?", run.JobKey).Updates(map[string]any{
			"last_status": status,
			"last_error":  errMsg,
			"updated_at":  now,
		}).Error
	})
}

// runReporter writes progress and stats as the handler reports them.
type runReporter struct {
	db    *gorm.DB
	runID int64
	mu    sync.Mutex
}

func (r *runReporter) SetProgress(current, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.db.Model(&Run{}).Where("id = ?", r.runID).Updates(map[string]any{
		"progress_current": current,
		"progress_total":   total,
		"heartbeat_at":     time.Now().UTC(),
	}).Error; err != nil {
		slog.Warn("jobs: update run progress failed", "run", r.runID, "err", err)
	}
}

func (r *runReporter) SetStats(stats map[string]any) {
	copied := make(map[string]any, len(stats))
	for k, v := range stats {
		copied[k] = v
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.db.Model(&Run{}).Where("id = ?", r.runID).
		Select("stats_json", "heartbeat_at").
		Updates(&Run{StatsJSON: copied, HeartbeatAt: &now}).Error; err != nil {
		slog.Warn("jobs: update run stats failed", "run", r.runID, "err", err)
	}
}

var activeStatuses = []string{StatusQueued, StatusRunning}

func jobView(job Job, active *Run) JobView {
	view := JobView{
		Key:         job.Key,
		Name:        job.Name,
		Description: job.Description,
		CronExpr:    job.CronExpr,
		Timezone:    job.Timezone,
		Enabled:     job.Enabled,
		NextRunAt:   utcPtr(job.NextRunAt),
		LastRunAt:   utcPtr(job.LastRunAt),
		LastStatus:  job.LastStatus,
		LastError:   job.LastError,
		UpdatedAt:   job.UpdatedAt.UTC(),
	}
	if active != nil {
		rv := runView(*active)
		view.ActiveRun = &rv
	}
	return view
}

func runView(run Run) RunView {
	stats := run.StatsJSON
	if stats == nil {
		stats = map[string]any{}
	}
	return RunView{
		ID:              run.ID,
		JobKey:          run.JobKey,
		Trigger:         run.Trigger,
		Status:          run.Status,
		ProgressCurrent: run.ProgressCurrent,
		ProgressTotal:   run.ProgressTotal,
		Stats:           stats,
		ErrorMessage:    run.ErrorMessage,
		CreatedBy:       run.CreatedBy,
		CreatedAt:       run.CreatedAt.UTC(),
		StartedAt:       utcPtr(run.StartedAt),
		FinishedAt:      utcPtr(run.FinishedAt),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	return page, min(pageSize, maxPageSize)
}

func isActiveRunConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "uniq_scheduled_job_runs_active"
}
