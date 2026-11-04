package gamification

import (
	"context"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Service 是游戏化领域服务。
type Service struct {
	repo      *Repository
	publisher EventPublisher
}

func NewService(repo *Repository, publisher ...EventPublisher) *Service {
	var p EventPublisher
	if len(publisher) > 0 {
		p = publisher[0]
	}
	return &Service{repo: repo, publisher: p}
}

// ListCoinTransactions 返回用户的流水分页、总数与当前余额。
func (s *Service) ListCoinTransactions(ctx context.Context, userID int64, page, size int) (*CoinTransactionPage, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	// 极大页码会让 (page-1)*size 溢出为负数，导致数据库报错；把页码限制在
	// 能安全计算 offset 的最大值，超出后自然返回空页。
	if page-1 > math.MaxInt/size {
		page = math.MaxInt/size + 1
	}
	items, total, balance, err := s.repo.ListCoinTransactions(ctx, userID, (page-1)*size, size)
	if err != nil {
		return nil, err
	}
	pageItems := make([]CoinTransactionItem, 0, len(items))
	for _, tx := range items {
		pageItems = append(pageItems, toCoinTransactionItem(tx))
	}
	return &CoinTransactionPage{Balance: balance, Total: total, Items: pageItems}, nil
}

// AdminAdjustCoins 校验管理员参数并调整用户金币，返回调整后的余额。
func (s *Service) AdminAdjustCoins(ctx context.Context, v Viewer, userID, amount int64, note string) (int64, error) {
	if !v.IsAdmin {
		return 0, httpx.ErrForbidden
	}
	if amount == 0 {
		return 0, httpx.ErrValidation("金币数量不能为 0")
	}
	note = strings.TrimSpace(note)
	length := utf8.RuneCountInString(note)
	if length < 1 {
		return 0, httpx.ErrValidation("备注不能为空")
	}
	if length > 200 {
		return 0, httpx.ErrValidation("备注不能超过 200 个字符")
	}
	if err := s.repo.AdminAdjustCoins(ctx, userID, amount, note); err != nil {
		return 0, err
	}
	return s.repo.Balance(ctx, userID)
}
