package user

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// memoryRepository is an in-memory Repository used by tests. It enforces the
// same unique and non-negative-coins constraints as the database.
type memoryRepository struct {
	mu     sync.Mutex
	nextID int64
	users  map[int64]User
}

func NewMemoryRepository() Repository {
	return &memoryRepository{users: map[int64]User{}}
}

func (r *memoryRepository) Create(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkUnique(u); err != nil {
		return err
	}
	if u.Rating == 0 {
		u.Rating = InitialRating
	}
	r.nextID++
	u.ID = r.nextID
	now := time.Now()
	u.CreatedAt, u.UpdatedAt = now, now
	r.users[u.ID] = *u
	return nil
}

func (r *memoryRepository) checkUnique(u *User) error {
	for id, existing := range r.users {
		if id == u.ID {
			continue
		}
		if existing.Username == u.Username {
			return ErrUsernameTaken
		}
		if existing.Email == u.Email {
			return ErrEmailTaken
		}
	}
	return nil
}

func (r *memoryRepository) FindByID(_ context.Context, id int64) (*User, error) {
	return r.find(func(u User) bool { return u.ID == id })
}

func (r *memoryRepository) FindByUsername(_ context.Context, username string) (*User, error) {
	return r.find(func(u User) bool { return u.Username == username })
}

func (r *memoryRepository) FindByEmail(_ context.Context, email string) (*User, error) {
	return r.find(func(u User) bool { return u.Email == email })
}

func (r *memoryRepository) find(match func(User) bool) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if match(u) {
			found := u
			return &found, nil
		}
	}
	return nil, ErrNotFound
}

func (r *memoryRepository) Update(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.ID]; !ok {
		return ErrNotFound
	}
	if u.Coins < 0 {
		return errors.New("coins must not be negative")
	}
	if err := r.checkUnique(u); err != nil {
		return err
	}
	u.UpdatedAt = time.Now()
	r.users[u.ID] = *u
	return nil
}

func (r *memoryRepository) List(_ context.Context, offset, limit int) ([]User, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]User, 0, len(r.users))
	for _, u := range r.users {
		all = append(all, u)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	total := int64(len(all))
	if offset >= len(all) {
		return []User{}, total, nil
	}
	end := min(offset+limit, len(all))
	return all[offset:end], total, nil
}
