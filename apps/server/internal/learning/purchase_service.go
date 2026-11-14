package learning

import (
	"context"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Purchase grants permanent access to a paid path. Free paths, authors and
// administrators are not charged and do not create purchase rows.
func (s *Service) Purchase(ctx context.Context, v Viewer, pathID int64) (*PurchaseResult, error) {
	if v.UserID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	p, err := s.visiblePath(ctx, v, pathID)
	if err != nil {
		return nil, err
	}
	// Authors and administrators already have access; never charge them.
	alreadyAllowed := v.CanManageOfficial || p.CreatedBy == v.UserID || p.PriceCoins == 0
	if !alreadyAllowed {
		if _, _, err := s.repo.Purchase(ctx, v.UserID, pathID, time.Now()); err != nil {
			return nil, mapServiceError(err)
		}
	}
	item, err := s.item(ctx, v, pathID)
	if err != nil {
		return nil, err
	}
	coins, err := s.repo.UserCoins(ctx, v.UserID)
	if err != nil {
		return nil, err
	}
	return &PurchaseResult{Path: *item, Coins: coins}, nil
}

// CanAccessPath reports whether the viewer may read the path tree contents.
func (s *Service) CanAccessPath(ctx context.Context, v Viewer, p *LearningPath) (bool, error) {
	if p.PriceCoins == 0 || v.CanManageOfficial || p.CreatedBy == v.UserID {
		return true, nil
	}
	if v.UserID <= 0 {
		return false, nil
	}
	return s.repo.HasPurchase(ctx, v.UserID, p.ID)
}

// Recommend adds one recommendation. Authors cannot recommend their own path,
// and paid paths require purchase (or author/admin access).
func (s *Service) Recommend(ctx context.Context, v Viewer, pathID int64) (*PathItem, error) {
	if v.UserID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	p, err := s.visiblePath(ctx, v, pathID)
	if err != nil {
		return nil, err
	}
	if p.CreatedBy == v.UserID {
		return nil, httpx.ErrValidation("不能推荐自己的路线")
	}
	allowed, err := s.CanAccessPath(ctx, v, p)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, httpx.ErrValidation("购买后才能推荐该路线")
	}
	if _, err := s.repo.SetRecommendation(ctx, v.UserID, pathID, true, time.Now()); err != nil {
		return nil, mapServiceError(err)
	}
	return s.item(ctx, v, pathID)
}

// Unrecommend removes the viewer's recommendation; removing an absent one is
// idempotent so a retried request succeeds.
func (s *Service) Unrecommend(ctx context.Context, v Viewer, pathID int64) (*PathItem, error) {
	if v.UserID <= 0 {
		return nil, httpx.ErrUnauthorized
	}
	if _, err := s.visiblePath(ctx, v, pathID); err != nil {
		return nil, err
	}
	if _, err := s.repo.SetRecommendation(ctx, v.UserID, pathID, false, time.Now()); err != nil {
		return nil, mapServiceError(err)
	}
	return s.item(ctx, v, pathID)
}
