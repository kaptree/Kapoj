package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
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

// SaveUpload stores an uploaded restore ZIP in the backup bucket and returns
// its object key. The restore worker downloads it only when the job runs.
func (s *Service) SaveUpload(ctx context.Context, name string, r io.Reader) (string, error) {
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.NewReplacer("\r", "_", "\n", "_", `"`, "_").Replace(base)
	if base == "." || base == "/" || base == "" {
		base = "upload.zip"
	}
	if runes := []rune(base); len(runes) > 80 {
		base = string(runes[len(runes)-80:])
	}
	key := fmt.Sprintf("uploads/%d-%s", time.Now().UnixNano(), base)
	if _, err := s.packages.Put(ctx, key, r, s.maxUploadBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return "", httpx.ErrValidation("备份包超过上传大小限制")
		}
		return "", err
	}
	return key, nil
}

// DeleteUpload removes a temporary uploaded restore ZIP after validation or
// restore failure.
func (s *Service) DeleteUpload(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	return s.packages.Delete(ctx, key)
}

// DownloadPackage opens a ready backup package for streaming.
func (s *Service) DownloadPackage(ctx context.Context, id int64) (io.ReadCloser, string, error) {
	row, err := s.repo.FindPackage(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if row.Status != PackageReady || row.Filename == "" {
		return nil, "", ErrNotFound
	}
	reader, err := s.packages.Open(ctx, row.Filename)
	if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrInvalidKey) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return reader, path.Base(row.Filename), nil
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
