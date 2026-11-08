package user

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	ErrNotFound      = errors.New("user not found")
	ErrUsernameTaken = errors.New("username taken")
	ErrEmailTaken    = errors.New("email taken")
)

type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id int64) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	// Update writes every column; use it only when the caller owns the whole row.
	Update(ctx context.Context, u *User) error
	// UpdateProfile writes only the provided profile fields, so concurrent
	// changes to other columns (status, coins, role) are never overwritten.
	UpdateProfile(ctx context.Context, id int64, changes ProfileChanges) error
	List(ctx context.Context, offset, limit int) ([]User, int64, error)
}

// ProfileChanges holds the user-editable profile fields; nil means unchanged.
type ProfileChanges struct {
	Nickname          *string
	Bio               *string
	AvatarURL         *string
	AvatarKey         *string
	AvatarContentType *string
}

func (c ProfileChanges) columns() map[string]any {
	cols := map[string]any{}
	if c.Nickname != nil {
		cols["nickname"] = *c.Nickname
	}
	if c.Bio != nil {
		cols["bio"] = *c.Bio
	}
	if c.AvatarURL != nil {
		cols["avatar_url"] = *c.AvatarURL
	}
	if c.AvatarKey != nil {
		cols["avatar_key"] = *c.AvatarKey
	}
	if c.AvatarContentType != nil {
		cols["avatar_content_type"] = *c.AvatarContentType
	}
	return cols
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, u *User) error {
	if u.Rating == 0 {
		u.Rating = InitialRating
	}
	return mapErr(r.db.WithContext(ctx).Create(u).Error)
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*User, error) {
	return r.findOne(ctx, "id = ?", id)
}

func (r *gormRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	return r.findOne(ctx, "username = ?", username)
}

func (r *gormRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	return r.findOne(ctx, "email = ?", email)
}

func (r *gormRepository) findOne(ctx context.Context, query string, arg any) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).Where(query, arg).First(&u).Error; err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *gormRepository) Update(ctx context.Context, u *User) error {
	return mapErr(r.db.WithContext(ctx).Save(u).Error)
}

func (r *gormRepository) UpdateProfile(ctx context.Context, id int64, changes ProfileChanges) error {
	cols := changes.columns()
	if len(cols) == 0 {
		_, err := r.FindByID(ctx, id)
		return err
	}
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return mapErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) List(ctx context.Context, offset, limit int) ([]User, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Model(&User{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []User
	if err := q.Order("id").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "uniq_users_username":
			return ErrUsernameTaken
		case "uniq_users_email":
			return ErrEmailTaken
		}
	}
	return err
}
