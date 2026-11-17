package problem

import (
	"context"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	BulkActionPublish = "publish"
	BulkActionHide    = "hide"
	BulkActionDelete  = "delete"

	BulkStatusSuccess = "success"
	BulkStatusFailed  = "failed"
)

// BulkResult is the independent outcome of one problem in a bulk action.
type BulkResult struct {
	ID       int64  `json:"id"`
	Status   string `json:"status"`
	Offlined bool   `json:"offlined"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// SetVisibility publishes or hides one problem through the normal update path,
// so learning-path guards and publishability checks still apply. The returned
// message distinguishes a no-op from an actual visibility change.
func (s *Service) SetVisibility(ctx context.Context, id int64, public bool) (string, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return "", mapRepoErr(err)
	}
	if p.IsPublic == public {
		if public {
			return "已公开", nil
		}
		return "已隐藏", nil
	}
	in := inputFromProblem(p)
	in.IsPublic = public
	_, err = s.Update(ctx, id, in)
	return "", err
}

// BulkAction runs publish/hide/delete independently for every problem. A
// failure is reported in that problem's result and never rolls back the rest.
func (s *Service) BulkAction(ctx context.Context, action string, ids []int64) ([]BulkResult, error) {
	switch action {
	case BulkActionPublish, BulkActionHide, BulkActionDelete:
	default:
		return nil, httpx.ErrValidation("action 必须是 publish、hide 或 delete")
	}
	if len(ids) == 0 {
		return nil, httpx.ErrValidation("请选择至少 1 道题目")
	}

	results := make([]BulkResult, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		result := BulkResult{ID: id, Status: BulkStatusSuccess}
		var err error
		switch action {
		case BulkActionPublish:
			result.Message, err = s.SetVisibility(ctx, id, true)
		case BulkActionHide:
			result.Message, err = s.SetVisibility(ctx, id, false)
		case BulkActionDelete:
			result.Offlined, err = s.Delete(ctx, id)
		}
		if err != nil {
			result.Status = BulkStatusFailed
			result.Offlined = false
			result.Message = ""
			result.Error = packageErrorText(err)
		}
		results = append(results, result)
	}
	return results, nil
}
