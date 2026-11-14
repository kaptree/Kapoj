package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

func (s *Service) SaveSchedule(ctx context.Context, kind Kind, in ScheduleInput, actorID int64) (*BackupSchedule, error) {
	if kind != KindPlatform && kind != KindAccounts {
		return nil, httpx.ErrValidation("无效的定时任务类型")
	}
	if in.Frequency != FrequencyDaily && in.Frequency != FrequencyWeekly && in.Frequency != FrequencyMonthly {
		return nil, httpx.ErrValidation("频率必须是 daily、weekly 或 monthly")
	}
	if in.RunHour < 0 || in.RunHour > 23 || in.RunMinute < 0 || in.RunMinute > 59 {
		return nil, httpx.ErrValidation("执行时间不合法")
	}
	if in.Weekday < 0 || in.Weekday > 6 {
		return nil, httpx.ErrValidation("weekday 必须为 0–6")
	}
	if in.DayOfMonth < 1 || in.DayOfMonth > 31 {
		return nil, httpx.ErrValidation("dayOfMonth 必须为 1–31")
	}
	schedule := BackupSchedule{
		Kind: kind, Enabled: in.Enabled, Frequency: in.Frequency,
		RunHour: in.RunHour, RunMinute: in.RunMinute,
		Weekday: in.Weekday, DayOfMonth: in.DayOfMonth,
	}
	if schedule.Enabled {
		next := NextRun(schedule, time.Now())
		schedule.NextRunAt = &next
	}
	return s.repo.SaveSchedule(ctx, kind, in, schedule.NextRunAt, actorID)
}

// SaveUpload stores an uploaded restore ZIP inside the backup directory.
func (s *Service) SaveUpload(_ context.Context, name string, r io.Reader) (string, error) {
	dir := filepath.Join(s.backupDir, "uploads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := filepath.Base(name)
	if base == "." || base == "/" || base == "" {
		base = "upload.zip"
	}
	filename := filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), base))
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(f, io.LimitReader(r, s.maxUploadBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(filename)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(filename)
		return "", closeErr
	}
	if n > s.maxUploadBytes {
		_ = os.Remove(filename)
		return "", httpx.ErrValidation("备份包超过上传大小限制")
	}
	return filename, nil
}

func (s *Service) DownloadPath(ctx context.Context, id int64) (string, error) {
	row, err := s.repo.FindPackage(ctx, id)
	if err != nil {
		return "", err
	}
	if row.Status != PackageReady || row.Filename == "" {
		return "", ErrNotFound
	}
	if _, err := os.Stat(row.Filename); err != nil {
		return "", ErrNotFound
	}
	return row.Filename, nil
}

func (s *Service) RestoreJob(ctx context.Context, id int64) (*BackupRestoreJob, error) {
	return s.repo.FindRestoreJob(ctx, id)
}

// RequestRestart triggers the same controlled restart used after a platform
// restore. The API response has time to reach the client first.
func (s *Service) RequestRestart() {
	time.AfterFunc(time.Second, func() {
		select {
		case s.restartCh <- struct{}{}:
		default:
		}
	})
}
