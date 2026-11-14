package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

type Queue interface {
	EnqueueCreate(ctx context.Context, packageID int64) error
	EnqueueRestore(ctx context.Context, jobID int64) error
}

type Service struct {
	db       *gorm.DB
	repo     *Repository
	database *Database

	storageDir      string
	backupDir       string
	maxUploadBytes  int64
	maxUncompressed int64
	maxEntries      int

	queue Queue

	restartCh   chan struct{}
	restartOnce sync.Once
	restoreMu   sync.Mutex
	stateMu     sync.Mutex
	restoring   bool
}

type Options struct {
	StorageDir           string
	BackupDir            string
	MaxUploadBytes       int64
	MaxUncompressedBytes int64
	MaxEntries           int
}

func NewService(db *gorm.DB, dsn string, opts Options) (*Service, error) {
	if opts.StorageDir == "" {
		opts.StorageDir = "data"
	}
	if opts.BackupDir == "" {
		opts.BackupDir = filepath.Join(opts.StorageDir, "backups")
	}
	if opts.MaxUploadBytes <= 0 {
		opts.MaxUploadBytes = 2 << 30
	}
	if opts.MaxUncompressedBytes <= 0 {
		opts.MaxUncompressedBytes = 20 << 30
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 200_000
	}
	if err := os.MkdirAll(opts.BackupDir, 0o700); err != nil {
		return nil, err
	}
	return &Service{
		db: db, repo: NewRepository(db), database: NewDatabase(dsn),
		storageDir: opts.StorageDir, backupDir: opts.BackupDir,
		maxUploadBytes: opts.MaxUploadBytes, maxUncompressed: opts.MaxUncompressedBytes,
		maxEntries: opts.MaxEntries, restartCh: make(chan struct{}, 1),
	}, nil
}

func (s *Service) SetQueue(q Queue) { s.queue = q }

func (s *Service) Ensure(ctx context.Context) error { return s.repo.EnsureSchedules(ctx) }

func (s *Service) Overview(ctx context.Context) (*Overview, error) { return s.repo.Overview(ctx) }

func (s *Service) MaxUploadBytes() int64 { return s.maxUploadBytes }

func (s *Service) RestartRequested() <-chan struct{} { return s.restartCh }

// StartCreate creates a package row and runs/queues its generation.
func (s *Service) StartCreate(ctx context.Context, kind Kind, userID *int64, actorID int64) (*BackupPackage, error) {
	s.stateMu.Lock()
	restoring := s.restoring
	s.stateMu.Unlock()
	if restoring {
		return nil, httpx.ErrValidation("恢复任务执行中，暂不能创建备份")
	}
	if kind != KindPlatform && kind != KindAccounts && kind != KindAccount {
		return nil, httpx.ErrValidation("无效的备份类型")
	}
	if kind == KindAccount && (userID == nil || *userID <= 0) {
		return nil, httpx.ErrValidation("账号备份需要指定用户")
	}
	if kind != KindAccount {
		userID = nil
	}
	row := &BackupPackage{
		Kind: kind, UserID: userID, Status: PackageRunning,
		CreatedBy: actorID, CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreatePackage(ctx, row); err != nil {
		return nil, err
	}
	if s.queue != nil {
		if err := s.queue.EnqueueCreate(ctx, row.ID); err != nil {
			_ = s.repo.FinishPackage(ctx, row.ID, PackageFailed, "", 0, "任务入队失败", "")
			return nil, err
		}
	} else {
		if err := s.RunPackage(ctx, row.ID); err != nil {
			return nil, err
		}
	}
	return s.repo.FindPackage(ctx, row.ID)
}

// RunPackage generates one queued backup package.
func (s *Service) RunPackage(ctx context.Context, packageID int64) error {
	row, err := s.repo.FindPackage(ctx, packageID)
	if err != nil {
		return err
	}
	if row.Status != PackageRunning {
		return nil
	}
	if err := s.waitRestoreDone(ctx); err != nil {
		return err
	}
	var (
		filename string
		size     int64
		hash     string
	)
	fail := func(err error) error {
		_ = s.repo.FinishPackage(ctx, row.ID, PackageFailed, "", 0, err.Error(), "")
		if row.CreatedBy == 0 {
			s.setScheduleResult(ctx, row.Kind, "failed", err.Error())
		}
		return err
	}
	switch row.Kind {
	case KindPlatform:
		filename = filepath.Join(s.backupDir, "platform", fmt.Sprintf("platform-%d.zip", time.Now().UnixNano()))
		manifest, err := s.createPlatformFile(ctx, filename, KindPlatform)
		if err != nil {
			return fail(err)
		}
		size, hash = fileSize(filename), manifest.SchemaHash
	case KindAccounts, KindAccount:
		filename = filepath.Join(s.backupDir, "accounts",
			fmt.Sprintf("%s-%d.zip", row.Kind, time.Now().UnixNano()))
		if row.Kind == KindAccount {
			filename = filepath.Join(s.backupDir, "accounts",
				fmt.Sprintf("account-%d-%d.zip", *row.UserID, time.Now().UnixNano()))
		}
		manifest, err := s.createAccountFile(ctx, filename, row.Kind, row.UserID)
		if err != nil {
			return fail(err)
		}
		size, hash = fileSize(filename), manifest.SchemaHash
	default:
		return fail(httpx.ErrValidation("未知备份类型"))
	}
	if err := s.repo.FinishPackage(ctx, row.ID, PackageReady, filename, size, "", hash); err != nil {
		return err
	}
	if row.Kind == KindAccount {
		s.cleanupOld(ctx, row.Kind, row.UserID, row.ID)
	} else {
		s.cleanupOld(ctx, row.Kind, nil, row.ID)
	}
	if row.CreatedBy == 0 {
		s.setScheduleResult(ctx, row.Kind, "success", "")
	}
	return nil
}

func fileSize(filename string) int64 {
	info, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (s *Service) cleanupOld(ctx context.Context, kind Kind, userID *int64, keepID int64) {
	files, err := s.repo.RemoveOlderPackages(ctx, kind, userID, keepID)
	if err != nil {
		return
	}
	for _, name := range files {
		if name != "" {
			_ = os.Remove(name)
		}
	}
}

func (s *Service) createPlatformFile(ctx context.Context, filename string, kind Kind) (*Manifest, error) {
	tmp := filename + ".tmp"
	writer, err := NewWriter(tmp, s.maxEntries)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			writer.Abort()
		}
	}()
	manifest, err := s.database.Dump(ctx, writer, kind)
	if err != nil {
		return nil, err
	}
	manifest.Kind = kind
	if err := addStorageTree(writer, s.storageDir, s.backupDir); err != nil {
		return nil, err
	}
	if err := writer.Close(*manifest); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, filename); err != nil {
		return nil, err
	}
	ok = true
	return manifest, nil
}

func (s *Service) StartRestore(ctx context.Context, kind Kind, actorID int64, packageID *int64, uploadPath string, targetUserID *int64) (*BackupRestoreJob, error) {
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	running, err := s.repo.AnyRunningRestore(ctx)
	if err != nil {
		return nil, err
	}
	if running {
		return nil, httpx.ErrValidation("已有恢复任务正在执行")
	}
	backingUp, err := s.repo.AnyRunningPackage(ctx)
	if err != nil {
		return nil, err
	}
	if backingUp {
		return nil, httpx.ErrValidation("备份任务执行中，请稍后再恢复")
	}
	if kind != KindPlatform && kind != KindAccount {
		return nil, httpx.ErrValidation("无效的恢复类型")
	}
	if kind == KindAccount && (targetUserID == nil || *targetUserID <= 0) {
		return nil, httpx.ErrValidation("账号恢复需要指定目标用户")
	}
	row := &BackupRestoreJob{
		Kind: kind, PackageID: packageID, TargetUserID: targetUserID,
		Status: RestoreRunning, CreatedBy: actorID, CreatedAt: time.Now().UTC(),
	}
	if uploadPath != "" {
		row.UploadedFile = uploadPath
	}
	if err := s.repo.CreateRestoreJob(ctx, row); err != nil {
		return nil, err
	}
	if s.queue != nil {
		if err := s.queue.EnqueueRestore(ctx, row.ID); err != nil {
			_ = s.repo.FinishRestoreJob(ctx, row.ID, RestoreFailed, "任务入队失败", false)
			return nil, err
		}
	} else {
		_ = s.RunRestore(ctx, row.ID)
	}
	return s.repo.FindRestoreJob(ctx, row.ID)
}

func (s *Service) RunRestore(ctx context.Context, jobID int64) error {
	row, err := s.repo.FindRestoreJob(ctx, jobID)
	if err != nil {
		return err
	}
	if row.Status != RestoreRunning {
		return nil
	}
	s.stateMu.Lock()
	s.restoring = true
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		s.restoring = false
		s.stateMu.Unlock()
	}()
	fail := func(err error) error {
		_ = s.repo.FinishRestoreJob(ctx, row.ID, RestoreFailed, err.Error(), false)
		return err
	}
	zipPath := row.UploadedFile
	if row.PackageID != nil {
		pkg, err := s.repo.FindPackage(ctx, *row.PackageID)
		if err != nil {
			return fail(err)
		}
		zipPath = pkg.Filename
	}
	if zipPath == "" {
		return fail(httpx.ErrValidation("备份包不存在"))
	}
	switch row.Kind {
	case KindPlatform:
		if err := s.restorePlatform(ctx, zipPath, row.CreatedBy); err != nil {
			return fail(err)
		}
		if err := s.repo.FinishRestoreJob(ctx, row.ID, RestoreSuccess, "", true); err != nil {
			return err
		}
		s.scheduleRestart()
		return nil
	case KindAccount:
		if err := s.restoreAccount(ctx, zipPath, *row.TargetUserID); err != nil {
			return fail(err)
		}
		return s.repo.FinishRestoreJob(ctx, row.ID, RestoreSuccess, "", false)
	}
	return fail(httpx.ErrValidation("未知恢复类型"))
}

func (s *Service) waitRestoreDone(ctx context.Context) error {
	for {
		s.stateMu.Lock()
		restoring := s.restoring
		s.stateMu.Unlock()
		if !restoring {
			return nil
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Service) scheduleRestart() {
	s.restartOnce.Do(func() {
		time.AfterFunc(3*time.Second, func() {
			select {
			case s.restartCh <- struct{}{}:
			default:
			}
		})
	})
}

func (s *Service) setScheduleResult(ctx context.Context, kind Kind, status, errMsg string) {
	if kind == KindPlatform || kind == KindAccounts {
		_ = s.repo.SetScheduleResult(ctx, kind, status, errMsg)
	}
}
