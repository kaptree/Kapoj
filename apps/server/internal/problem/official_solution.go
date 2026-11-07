package problem

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	// Official solution access states.
	AccessFree      = "free"
	AccessAccepted  = "accepted"
	AccessPurchased = "purchased"
	AccessCreator   = "creator"
	AccessLocked    = "locked"
	AccessGuest     = "guest"

	maxSolutionMDChars = 50000
	maxSolutionCode    = 65536
	maxSolutionPrice   = 1_000_000
)

// ProblemSolutionUnlock 记录用户对某道题官方题解的永久解锁。价格变化不影响历史记录。
type ProblemSolutionUnlock struct {
	ID        int64     `gorm:"primaryKey"`
	UserID    int64     `gorm:"not null;uniqueIndex:uniq_problem_solution_unlocks,priority:1"`
	ProblemID int64     `gorm:"not null;uniqueIndex:uniq_problem_solution_unlocks,priority:2"`
	PricePaid int64     `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
}

func (ProblemSolutionUnlock) TableName() string { return "problem_solution_unlocks" }

// OfficialSolutionInput 是官方题解的唯一写入口。
type OfficialSolutionInput struct {
	SolutionMD   string `json:"solutionMd"`
	SolutionCode string `json:"solutionCode"`
	PriceCoins   int64  `json:"priceCoins"`
}

// OfficialSolutionView 是官方题解的访问视图；正文仅在允许的访问状态下返回。
type OfficialSolutionView struct {
	PriceCoins   int64   `json:"priceCoins"`
	Access       string  `json:"access"`
	CanEdit      bool    `json:"canEdit"`
	CanPurchase  bool    `json:"canPurchase"`
	SolutionMD   *string `json:"solutionMd,omitempty"`
	SolutionCode *string `json:"solutionCode,omitempty"`
}

// OfficialSolutionService 处理官方题解的访问、编辑与金币解锁。
type OfficialSolutionService struct {
	db       *gorm.DB
	problems Repository
}

func NewOfficialSolutionService(db *gorm.DB, problems Repository) *OfficialSolutionService {
	return &OfficialSolutionService{db: db, problems: problems}
}

// Get 返回用户对某题官方题解的访问视图。隐藏题要求 view_hidden 或 super。
func (s *OfficialSolutionService) Get(ctx context.Context, userID, problemID int64, v Viewer, isSuper bool) (*OfficialSolutionView, error) {
	p, err := s.problems.FindByID(ctx, problemID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if !p.IsPublic && !v.CanViewHidden && !isSuper {
		return nil, httpx.ErrNotFound
	}
	return s.view(ctx, userID, problemID, p, isSuper)
}

// Update 允许创建者或 super 保存官方题解的说明、代码与价格；不修改其他字段。
func (s *OfficialSolutionService) Update(ctx context.Context, userID, problemID int64, isSuper bool, in OfficialSolutionInput) (*OfficialSolutionView, error) {
	if runes(in.SolutionMD) > maxSolutionMDChars {
		return nil, httpx.ErrValidation("官方题解说明不能超过 50000 个字符")
	}
	if len(in.SolutionCode) > maxSolutionCode {
		return nil, httpx.ErrValidation("官方题解代码不能超过 65536 字节")
	}
	if in.PriceCoins < 0 || in.PriceCoins > maxSolutionPrice {
		return nil, httpx.ErrValidation("价格需为 0–1000000 金币")
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := lockProblem(tx, problemID)
		if err != nil {
			return err
		}
		if !isSuper && p.CreatedBy != userID {
			return httpx.ErrForbidden
		}
		res := tx.Model(&Problem{}).Where("id = ?", problemID).Updates(map[string]any{
			"solution_md":          in.SolutionMD,
			"solution_code":        in.SolutionCode,
			"solution_price_coins": in.PriceCoins,
			"updated_at":           time.Now(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	return s.Get(ctx, userID, problemID, Viewer{CanViewHidden: true}, isSuper)
}

// Purchase 在事务内解锁官方题解；价格 0、创建者、super、已通过或已解锁不扣费。
func (s *OfficialSolutionService) Purchase(ctx context.Context, userID, problemID int64, v Viewer, isSuper bool) (*OfficialSolutionView, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := lockProblem(tx, problemID)
		if err != nil {
			return err
		}
		if !p.IsPublic && !v.CanViewHidden && !isSuper {
			return httpx.ErrNotFound
		}

		// 免费情形：价格 0、创建者、super 或已通过均不扣费。
		if p.SolutionPriceCoins == 0 || isSuper || p.CreatedBy == userID {
			return nil
		}
		var acceptedCount int64
		if err := tx.WithContext(ctx).Table("submissions").
			Where("user_id = ? AND problem_id = ? AND status = ?", userID, problemID, "accepted").
			Count(&acceptedCount).Error; err != nil {
			return err
		}
		if acceptedCount > 0 {
			return nil
		}
		var unlocked int64
		if err := tx.Model(&ProblemSolutionUnlock{}).
			Where("user_id = ? AND problem_id = ?", userID, problemID).
			Count(&unlocked).Error; err != nil {
			return err
		}
		if unlocked > 0 {
			return nil
		}

		// 写解锁记录；冲突表示并发已解锁，直接返回不扣费。
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ProblemSolutionUnlock{
			UserID: userID, ProblemID: problemID, PricePaid: p.SolutionPriceCoins,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		// 扣金币并写流水；余额不足时整个事务回滚（含解锁记录）。
		return gamification.SpendCoinsTx(tx, userID, p.SolutionPriceCoins, "problem_solution", "problem_solution", problemID)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, problemID, v, isSuper)
}

// view 基于已加载的题目计算访问状态与内容可见性。
// 优先级：creator/super → accepted → purchased → free → guest → locked。
func (s *OfficialSolutionService) view(ctx context.Context, userID, problemID int64, p *Problem, isSuper bool) (*OfficialSolutionView, error) {
	v := &OfficialSolutionView{PriceCoins: p.SolutionPriceCoins}

	creator := isSuper || (userID > 0 && p.CreatedBy == userID)
	v.CanEdit = creator

	access := AccessLocked
	switch {
	case creator:
		access = AccessCreator
	case userID > 0:
		accepted, err := s.hasAccepted(ctx, userID, problemID)
		if err != nil {
			return nil, err
		}
		if accepted {
			access = AccessAccepted
			break
		}
		unlocked, err := s.hasUnlock(ctx, userID, problemID)
		if err != nil {
			return nil, err
		}
		if unlocked {
			access = AccessPurchased
			break
		}
		if p.SolutionPriceCoins == 0 {
			access = AccessFree
		}
	case p.SolutionPriceCoins == 0:
		access = AccessFree
	default:
		access = AccessGuest
	}
	v.Access = access

	switch access {
	case AccessCreator, AccessAccepted, AccessPurchased, AccessFree:
		v.SolutionMD = &p.SolutionMD
		v.SolutionCode = &p.SolutionCode
	}

	v.CanPurchase = userID > 0 && p.SolutionPriceCoins > 0 &&
		access != AccessCreator && access != AccessAccepted && access != AccessPurchased
	return v, nil
}

// hasAccepted 报告用户是否已通过该题。
func (s *OfficialSolutionService) hasAccepted(ctx context.Context, userID, problemID int64) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Table("submissions").
		Where("user_id = ? AND problem_id = ? AND status = ?", userID, problemID, "accepted").
		Count(&n).Error
	return n > 0, err
}

// hasUnlock 报告用户是否已解锁该题官方题解。
func (s *OfficialSolutionService) hasUnlock(ctx context.Context, userID, problemID int64) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&ProblemSolutionUnlock{}).
		Where("user_id = ? AND problem_id = ?", userID, problemID).
		Count(&n).Error
	return n > 0, err
}

// lockProblem 行锁加载题目；不存在返回 httpx.ErrNotFound。
func lockProblem(tx *gorm.DB, problemID int64) (*Problem, error) {
	var p Problem
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, problemID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
