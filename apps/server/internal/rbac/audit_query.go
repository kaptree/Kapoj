package rbac

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// AuditFilter 是审计查询的筛选条件。From/To 为闭区间（含边界），由 handler 负责
// 把日期过滤（RFC3339 或 YYYY-MM-DD）解析成 *time.Time。
type AuditFilter struct {
	ActorID    int64
	Action     string
	TargetType string
	From, To   *time.Time
}

// AuditItem 是审计列表中的一项。只暴露审计表已存的字段，不含密码、token、
// 代码等敏感信息；Summary 沿用写入时的中文摘要。
type AuditItem struct {
	ID         int64     `json:"id"`
	ActorID    int64     `json:"actorId"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   *int64    `json:"targetId"`
	Summary    string    `json:"summary"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ListAudit 按筛选条件与分页返回审计日志，按 created_at DESC, id DESC 稳定排序，
// 并返回匹配总数。
func (s *Service) ListAudit(ctx context.Context, filter AuditFilter, page, size int) (httpx.Page[AuditItem], error) {
	db := s.db.WithContext(ctx)
	q := db.Model(&AdminAuditLog{})

	if filter.ActorID > 0 {
		q = q.Where("actor_id = ?", filter.ActorID)
	}
	if action := strings.TrimSpace(filter.Action); action != "" {
		q = q.Where("action = ?", action)
	}
	if targetType := strings.TrimSpace(filter.TargetType); targetType != "" {
		q = q.Where("target_type = ?", targetType)
	}
	if filter.From != nil {
		q = q.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("created_at <= ?", *filter.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return httpx.Page[AuditItem]{}, fmt.Errorf("rbac: count audit logs: %w", err)
	}

	var rows []AdminAuditLog
	if err := q.Order("created_at DESC, id DESC").
		Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return httpx.Page[AuditItem]{}, fmt.Errorf("rbac: list audit logs: %w", err)
	}

	items := make([]AuditItem, len(rows))
	for i, row := range rows {
		items[i] = AuditItem(row)
	}
	return httpx.Page[AuditItem]{Total: total, Items: items}, nil
}
