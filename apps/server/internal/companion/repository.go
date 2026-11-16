package companion

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
)

// dailyCap is the natural bond gain a user may earn in a single local day.
// Streak milestones bypass it via EventInput.DailyCapBypass.
const dailyCap = 15

// loginStreakLookbackDays bounds the login-day scan for a streak. It matches
// the existing gamification streak implementation.
const loginStreakLookbackDays = 400

// ErrNotFound is returned when a looked-up row does not exist.
var ErrNotFound = errors.New("companion: not found")

// Repository is the transactional companion repository. All bond writes go
// through ApplyEvent so the daily cap, sub-caps and idempotency are enforced in
// one state-row transaction.
type Repository struct {
	db *gorm.DB
}

// NewGormRepository returns a Repository backed by db.
func NewGormRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

const insertStateSQL = `INSERT INTO companion_states (user_id, created_at, updated_at)
	VALUES (?, now(), now()) ON CONFLICT (user_id) DO NOTHING`

// EnsureState lazily creates the user's state row and returns it. It is
// idempotent: a second call returns the same row.
func (r *Repository) EnsureState(ctx context.Context, userID int64) (*State, error) {
	if err := r.db.WithContext(ctx).Exec(insertStateSQL, userID).Error; err != nil {
		return nil, err
	}
	var st State
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

// UpdateSettings persists the user-editable settings and returns the resulting
// state. It ensures the state row exists first, so a fresh user can save
// settings before any event has fired.
func (r *Repository) UpdateSettings(ctx context.Context, userID int64, in SettingsInput) (*State, error) {
	if _, err := r.EnsureState(ctx, userID); err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&State{}).Where("user_id = ?", userID).Updates(map[string]any{
		"activity_level": in.ActivityLevel,
		"dock_enabled":   in.DockEnabled,
		"public_profile": in.PublicProfile,
		"equipped_skin":  in.EquippedSkin,
	}).Error; err != nil {
		return nil, err
	}
	var st State
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

// ApplyEvent records one bond event inside a single transaction that locks the
// user's state row. It enforces the daily natural cap, the per-type sub-cap and
// the (user_id, key) uniqueness. It returns the event and whether it was newly
// created; a duplicate returns the existing event with created=false and never
// awards bond twice.
func (r *Repository) ApplyEvent(ctx context.Context, in EventInput) (*Event, bool, error) {
	var out Event
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ev, c, err := r.applyEventTx(tx, in)
		if err != nil {
			return err
		}
		out = ev
		created = c
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &out, created, nil
}

// applyEventTx applies one bond event inside an already-open transaction. It is
// split out so Pet can record last_pet_date in the same transaction.
func (r *Repository) applyEventTx(tx *gorm.DB, in EventInput) (Event, bool, error) {
	var out Event
	if err := tx.Exec(insertStateSQL, in.UserID).Error; err != nil {
		return out, false, err
	}
	var st State
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", in.UserID).First(&st).Error; err != nil {
		return out, false, err
	}

	// A new local date resets the daily counter.
	dailyGain := st.DailyGain
	if st.DailyDate != in.EventDate {
		dailyGain = 0
	}

	bond := in.Bond
	if bond < 0 {
		bond = 0
	}
	if !in.DailyCapBypass {
		if rem := dailyCap - dailyGain; bond > rem {
			bond = rem
		}
		if bond < 0 {
			bond = 0
		}
		if in.SubCap > 0 {
			// Sum the bond already awarded for this type/date, not the row
			// count, so zero-bond rows do not consume sub-cap slots.
			var awarded int64
			if err := tx.Model(&Event{}).
				Where("user_id = ? AND type = ? AND event_date = ?", in.UserID, in.Type, in.EventDate).
				Select("COALESCE(SUM(bond), 0)").
				Scan(&awarded).Error; err != nil {
				return out, false, err
			}
			if rem := in.SubCap - int(awarded); bond > rem {
				bond = rem
			}
			if bond < 0 {
				bond = 0
			}
		}
	}

	payload := in.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	ev := Event{
		UserID:    in.UserID,
		Key:       in.Key,
		Type:      in.Type,
		Bond:      bond,
		EventDate: in.EventDate,
		Payload:   dbtypes.JSON[map[string]any]{Data: payload},
		CreatedAt: time.Now(),
	}
	res := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "key"}},
		DoNothing: true,
	}).Create(&ev)
	if res.Error != nil {
		return out, false, res.Error
	}
	if res.RowsAffected == 0 {
		// Duplicate key: return the existing event without touching state.
		if err := tx.Where("user_id = ? AND key = ?", in.UserID, in.Key).First(&out).Error; err != nil {
			return out, false, err
		}
		return out, false, nil
	}

	newBond := st.Bond + bond
	lvl := LevelForBond(newBond)
	updates := map[string]any{
		"bond":  newBond,
		"level": lvl.Level,
	}
	if !in.DailyCapBypass {
		updates["daily_gain"] = dailyGain + bond
		updates["daily_date"] = in.EventDate
	}
	if err := tx.Model(&State{}).Where("user_id = ?", in.UserID).Updates(updates).Error; err != nil {
		return out, false, err
	}
	return ev, true, nil
}

// Pet records one daily pet interaction (key pet:{date}, +1). It returns the
// event and whether it was awarded today. last_pet_date is set in the same
// transaction as the event; it is also set when the key already exists, so a
// retry after a failed earlier write repairs the date.
func (r *Repository) Pet(ctx context.Context, userID int64, date string) (*Event, bool, error) {
	var out Event
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ev, c, err := r.applyEventTx(tx, EventInput{
			UserID:    userID,
			Key:       "pet:" + date,
			Type:      "pet",
			Bond:      1,
			EventDate: date,
			Payload:   map[string]any{},
		})
		if err != nil {
			return err
		}
		if err := tx.Model(&State{}).Where("user_id = ?", userID).
			Update("last_pet_date", date).Error; err != nil {
			return err
		}
		out = ev
		created = c
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &out, created, nil
}

// UnreadEvents returns the user's unread events ordered by id, limited to
// limit rows. It returns an empty slice when there are none.
func (r *Repository) UnreadEvents(ctx context.Context, userID int64, limit int) ([]Event, error) {
	events := []Event{}
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND read_at IS NULL", userID).
		Order("id").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// CountUnread returns the number of the user's unread events.
func (r *Repository) CountUnread(ctx context.Context, userID int64) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&Event{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// EventsByDate returns the user's events for a local date ordered by id. It
// returns an empty slice when there are none.
func (r *Repository) EventsByDate(ctx context.Context, userID int64, date string) ([]Event, error) {
	events := []Event{}
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND event_date = ?", userID, date).
		Order("id").
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// MarkRead marks the given event ids as read for the user. Ids belonging to
// other users or already-read events are ignored.
func (r *Repository) MarkRead(ctx context.Context, userID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Event{}).
		Where("user_id = ? AND id IN ? AND read_at IS NULL", userID, ids).
		Update("read_at", time.Now()).Error
}

// SubmissionInfo returns the companion-relevant fields of a submission.
func (r *Repository) SubmissionInfo(ctx context.Context, submissionID int64) (*SubmissionInfo, error) {
	var info SubmissionInfo
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, problem_id, status, created_at, judged_at
		   FROM submissions WHERE id = ?`, submissionID,
	).Scan(&info).Error; err != nil {
		return nil, err
	}
	if info.ID == 0 {
		return nil, ErrNotFound
	}
	return &info, nil
}

// HasEarlierAccepted reports whether the user has an accepted submission for
// problemID that comes before the submission identified by submissionID, using
// (created_at, id) ordering. The submission itself is excluded, and only
// accepted submissions count.
func (r *Repository) HasEarlierAccepted(ctx context.Context, userID, problemID int64, createdAt time.Time, submissionID int64) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(
		`SELECT EXISTS(
			SELECT 1 FROM submissions
			WHERE user_id = ? AND problem_id = ? AND status = 'accepted'
			  AND id <> ?
			  AND (created_at < ? OR (created_at = ? AND id < ?))
		)`, userID, problemID, submissionID, createdAt, createdAt, submissionID,
	).Scan(&ok).Error
	return ok, err
}

// LoginStreak counts consecutive local login days ending at date. A missing
// day (or no login on date itself) breaks the streak.
func (r *Repository) LoginStreak(ctx context.Context, userID int64, date string) (int, error) {
	today, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return 0, err
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
	for _, d := range dates {
		present[d] = struct{}{}
	}
	streak := 0
	for day := today; ; day = day.AddDate(0, 0, -1) {
		if _, ok := present[day.Format("2006-01-02")]; !ok {
			break
		}
		streak++
	}
	return streak, nil
}
