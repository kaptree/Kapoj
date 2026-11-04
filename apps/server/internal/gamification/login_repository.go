package gamification

import (
	"context"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	loginDateLayout         = "2006-01-02"
	loginStreakLookbackDays = 400
)

// RecordLoginDay 记录用户在某天的登录；重复日期由唯一约束忽略。
func (r *Repository) RecordLoginDay(ctx context.Context, userID int64, date string) error {
	if _, err := time.ParseInLocation(loginDateLayout, date, time.Local); err != nil {
		return httpx.ErrValidation("登录日期格式错误")
	}
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO user_login_days (user_id, login_date, created_at)
		 VALUES (?, ?::date, now())
		 ON CONFLICT (user_id, login_date) DO NOTHING`,
		userID, date,
	).Error
}

// CurrentLoginStreak 返回截至 today 的连续登录天数；今天未登录时返回 0。
func (r *Repository) CurrentLoginStreak(ctx context.Context, userID int64, today string) (int64, error) {
	todayDate, err := time.ParseInLocation(loginDateLayout, today, time.Local)
	if err != nil {
		return 0, httpx.ErrValidation("登录日期格式错误")
	}
	var dates []string
	if err := r.db.WithContext(ctx).Raw(
		`SELECT to_char(login_date, 'YYYY-MM-DD') FROM user_login_days
		 WHERE user_id = ? ORDER BY login_date DESC LIMIT ?`,
		userID, loginStreakLookbackDays,
	).Scan(&dates).Error; err != nil {
		return 0, err
	}
	present := make(map[string]struct{}, len(dates))
	for _, date := range dates {
		present[date] = struct{}{}
	}
	var streak int64
	for day := todayDate; ; day = day.AddDate(0, 0, -1) {
		if _, ok := present[day.Format(loginDateLayout)]; !ok {
			break
		}
		streak++
	}
	return streak, nil
}
