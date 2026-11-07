package learning

import (
	"context"
	"time"
)

// evalResult is the outcome of evaluating one node for a user.
type evalResult struct {
	status    string
	solved    int
	completed bool
	write     bool
}

// EvaluateUser recomputes and persists the user's progress across every active
// learning path visible to them. It is idempotent and safe to call both on
// reads and on accepted-submission events.
//
// Rules:
//   - solved = the number of distinct problems in the node that the user has
//     an accepted submission for; completed when solved >= required_count.
//   - the root node is active; a child is active only when its parent is
//     completed, otherwise locked.
//   - once a node is completed it stays completed with the solved_count
//     recorded at completion, even if the node's problems or required_count
//     later change; uncompleted nodes are recomputed against current config.
//   - only nodes of active paths visible to the user are touched; nodes of
//     inactive or invisible paths are left unchanged.
//   - guests (userID <= 0) are a no-op.
func (s *Service) EvaluateUser(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	pathIDs, err := s.repo.ListActivePathIDsForUser(ctx, userID)
	if err != nil {
		return err
	}
	if len(pathIDs) == 0 {
		return nil
	}
	nodes, err := s.repo.ListNodesByPathIDs(ctx, pathIDs)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}
	existing, err := s.repo.ListProgressForPaths(ctx, userID, pathIDs)
	if err != nil {
		return err
	}
	accepted, err := s.repo.AcceptedProblemIDs(ctx, userID)
	if err != nil {
		return err
	}

	byID := make(map[int64]LearningPathNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	now := time.Now()
	memo := make(map[int64]evalResult, len(nodes))
	visiting := make(map[int64]bool)

	var eval func(nodeID int64) evalResult
	eval = func(nodeID int64) evalResult {
		if res, ok := memo[nodeID]; ok {
			return res
		}
		if visiting[nodeID] {
			// Defensive cycle guard: treat as locked without writing a row.
			return evalResult{status: ProgressLocked}
		}
		n, ok := byID[nodeID]
		if !ok {
			return evalResult{status: ProgressLocked}
		}
		// Completed latch: keep completed and the recorded solved count, no upsert.
		if p, ok := existing[nodeID]; ok && p.Status == ProgressCompleted {
			res := evalResult{status: ProgressCompleted, solved: p.SolvedCount}
			memo[nodeID] = res
			return res
		}

		visiting[nodeID] = true
		defer delete(visiting, nodeID)

		// Count distinct accepted problems within the node's problem set.
		solved := 0
		seen := make(map[int64]struct{}, len(n.ProblemIDs))
		for _, pid := range n.ProblemIDs {
			if _, accepted := accepted[pid]; !accepted {
				continue
			}
			if _, dup := seen[pid]; dup {
				continue
			}
			seen[pid] = struct{}{}
			solved++
		}

		var res evalResult
		if solved >= n.RequiredCount {
			res = evalResult{status: ProgressCompleted, solved: solved, completed: true, write: true}
		} else {
			status := ProgressActive
			if n.ParentID != nil {
				if _, ok := byID[*n.ParentID]; !ok {
					// Missing parent: treat as a root (active).
					status = ProgressActive
				} else if eval(*n.ParentID).status != ProgressCompleted {
					status = ProgressLocked
				}
			}
			res = evalResult{status: status, solved: solved, write: true}
		}
		memo[nodeID] = res
		return res
	}

	var rows []UserLearningProgress
	for _, n := range nodes {
		res := eval(n.ID)
		if !res.write {
			continue
		}
		row := UserLearningProgress{
			UserID:      userID,
			PathID:      n.PathID,
			NodeID:      n.ID,
			Status:      res.status,
			SolvedCount: res.solved,
			UpdatedAt:   now,
		}
		if res.completed {
			row.CompletedAt = &now
		}
		rows = append(rows, row)
	}
	return s.repo.UpsertProgress(ctx, rows)
}
