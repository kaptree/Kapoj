package problem

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	maxPostTitleChars    = 100
	maxPostContentChars  = 50000
	maxReplyContentChars = 10000
	maxRepliesPerPost    = 500
	postExcerptChars     = 200
)

// AuthorSummary 是帖子/回复作者的公开摘要；仅含 ID/username/nickname，绝不包含邮箱。
type AuthorSummary struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// SolutionPostInput 是发布/编辑帖子的输入。
type SolutionPostInput struct {
	Title     string `json:"title"`
	ContentMD string `json:"contentMd"`
}

// SolutionReplyInput 是发布/编辑回复的输入。
type SolutionReplyInput struct {
	ContentMD string `json:"contentMd"`
}

// SolutionPostItem 是帖子列表项；正文不返回，只返回摘要。
type SolutionPostItem struct {
	ID         int64         `json:"id"`
	ProblemID  int64         `json:"problemId"`
	UserID     int64         `json:"userId"`
	Title      string        `json:"title"`
	Excerpt    string        `json:"excerpt"`
	Author     AuthorSummary `json:"author"`
	ReplyCount int64         `json:"replyCount"`
	LikeCount  int64         `json:"likeCount"`
	LikedByMe  bool          `json:"likedByMe"`
	CreatedAt  time.Time     `json:"createdAt"`
	UpdatedAt  time.Time     `json:"updatedAt"`
	CanEdit    bool          `json:"canEdit"`
	CanDelete  bool          `json:"canDelete"`
}

// SolutionPostDetail 是帖子详情：内嵌列表项 + 正文 + 回复。
type SolutionPostDetail struct {
	SolutionPostItem
	ContentMD string              `json:"contentMd"`
	Replies   []SolutionReplyItem `json:"replies"`
}

// SolutionReplyItem 是回复。
type SolutionReplyItem struct {
	ID        int64         `json:"id"`
	PostID    int64         `json:"postId"`
	UserID    int64         `json:"userId"`
	ContentMD string        `json:"contentMd"`
	Author    AuthorSummary `json:"author"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
	CanEdit   bool          `json:"canEdit"`
	CanDelete bool          `json:"canDelete"`
}

// DiscussionService 处理解题思路的帖子与回复。
type DiscussionService struct {
	db       *gorm.DB
	problems Repository
}

func NewDiscussionService(db *gorm.DB, problems Repository) *DiscussionService {
	return &DiscussionService{db: db, problems: problems}
}

// loadVisibleProblem 加载题目并套用隐藏题可见性规则；隐藏题无 view_hidden/super
// 时视为不存在（404），不沿用比赛参与者 fallback。
func (s *DiscussionService) loadVisibleProblem(ctx context.Context, problemID int64, v Viewer, isSuper bool) (*Problem, error) {
	p, err := s.problems.FindByID(ctx, problemID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if !p.IsPublic && !v.CanViewHidden && !isSuper {
		return nil, httpx.ErrNotFound
	}
	return p, nil
}

// ListPosts 返回帖子分页列表，按 created_at DESC, id DESC；正文只返回摘要。
func (s *DiscussionService) ListPosts(ctx context.Context, userID, problemID int64, v Viewer, isSuper, isModerator bool, page, size int) (httpx.Page[SolutionPostItem], error) {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return httpx.Page[SolutionPostItem]{}, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}

	q := s.db.WithContext(ctx).Model(&SolutionPost{}).Where("problem_id = ?", problemID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return httpx.Page[SolutionPostItem]{}, err
	}

	var posts []SolutionPost
	if err := q.Order("created_at DESC, id DESC").
		Offset((page - 1) * size).Limit(size).Find(&posts).Error; err != nil {
		return httpx.Page[SolutionPostItem]{}, err
	}

	items, err := s.postItems(ctx, p, userID, isModerator, posts)
	if err != nil {
		return httpx.Page[SolutionPostItem]{}, err
	}
	return httpx.Page[SolutionPostItem]{Total: total, Items: items}, nil
}

// CreatePost 发布帖子。
func (s *DiscussionService) CreatePost(ctx context.Context, userID, problemID int64, v Viewer, isSuper bool, in SolutionPostInput) (*SolutionPostDetail, error) {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return nil, err
	}
	title, err := validatePostInput(in)
	if err != nil {
		return nil, err
	}
	post := &SolutionPost{
		ProblemID: problemID, UserID: userID, Title: title, ContentMD: in.ContentMD,
	}
	if err := s.db.WithContext(ctx).Create(post).Error; err != nil {
		return nil, err
	}
	return s.postDetail(ctx, p, userID, false, post)
}

// GetPost 返回帖子详情与回复；不存在或已删除返回 404。
func (s *DiscussionService) GetPost(ctx context.Context, userID, problemID, postID int64, v Viewer, isSuper, isModerator bool) (*SolutionPostDetail, error) {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return nil, err
	}
	post, err := s.findPost(ctx, problemID, postID)
	if err != nil {
		return nil, err
	}
	return s.postDetail(ctx, p, userID, isModerator, post)
}

// UpdatePost 仅作者可编辑自己的帖子。
func (s *DiscussionService) UpdatePost(ctx context.Context, userID, problemID, postID int64, v Viewer, isSuper, isModerator bool, in SolutionPostInput) (*SolutionPostDetail, error) {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return nil, err
	}
	title, err := validatePostInput(in)
	if err != nil {
		return nil, err
	}
	post, err := s.findPost(ctx, problemID, postID)
	if err != nil {
		return nil, err
	}
	if post.UserID != userID {
		return nil, httpx.ErrForbidden
	}
	res := s.db.WithContext(ctx).Model(&SolutionPost{}).
		Where("id = ?", postID).
		Updates(map[string]any{"title": title, "content_md": in.ContentMD, "updated_at": time.Now()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, httpx.ErrNotFound
	}
	post.Title = title
	post.ContentMD = in.ContentMD
	return s.postDetail(ctx, p, userID, isModerator, post)
}

// DeletePost 软删除帖子；作者、题目创建者或 problem.update 管理员可删除。
func (s *DiscussionService) DeletePost(ctx context.Context, userID, problemID, postID int64, v Viewer, isSuper, isModerator bool) error {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return err
	}
	post, err := s.findPost(ctx, problemID, postID)
	if err != nil {
		return err
	}
	if !canDelete(userID, post.UserID, p, isModerator) {
		return httpx.ErrForbidden
	}
	res := s.db.WithContext(ctx).Delete(&SolutionPost{}, postID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// CreateReply 发布回复；单帖上限 500 条。
func (s *DiscussionService) CreateReply(ctx context.Context, userID, problemID, postID int64, v Viewer, isSuper, isModerator bool, in SolutionReplyInput) (*SolutionReplyItem, error) {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return nil, err
	}
	if err := validateReplyInput(in); err != nil {
		return nil, err
	}
	if _, err := s.findPost(ctx, problemID, postID); err != nil {
		return nil, err
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&SolutionReply{}).Where("post_id = ?", postID).Count(&n).Error; err != nil {
		return nil, err
	}
	if n >= maxRepliesPerPost {
		return nil, httpx.ErrValidation("单个帖子的回复不能超过 500 条")
	}
	reply := &SolutionReply{PostID: postID, UserID: userID, ContentMD: in.ContentMD}
	if err := s.db.WithContext(ctx).Create(reply).Error; err != nil {
		return nil, err
	}
	return s.replyItem(ctx, p, userID, isModerator, reply)
}

// UpdateReply 仅回复作者可编辑。
func (s *DiscussionService) UpdateReply(ctx context.Context, userID, problemID, postID, replyID int64, v Viewer, isSuper, isModerator bool, in SolutionReplyInput) (*SolutionReplyItem, error) {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return nil, err
	}
	if err := validateReplyInput(in); err != nil {
		return nil, err
	}
	if _, err := s.findPost(ctx, problemID, postID); err != nil {
		return nil, err
	}
	reply, err := s.findReply(ctx, postID, replyID)
	if err != nil {
		return nil, err
	}
	if reply.UserID != userID {
		return nil, httpx.ErrForbidden
	}
	res := s.db.WithContext(ctx).Model(&SolutionReply{}).
		Where("id = ?", replyID).
		Updates(map[string]any{"content_md": in.ContentMD, "updated_at": time.Now()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, httpx.ErrNotFound
	}
	reply.ContentMD = in.ContentMD
	return s.replyItem(ctx, p, userID, isModerator, reply)
}

// DeleteReply 软删除回复；作者、题目创建者或 problem.update 管理员可删除。
func (s *DiscussionService) DeleteReply(ctx context.Context, userID, problemID, postID, replyID int64, v Viewer, isSuper, isModerator bool) error {
	p, err := s.loadVisibleProblem(ctx, problemID, v, isSuper)
	if err != nil {
		return err
	}
	if _, err := s.findPost(ctx, problemID, postID); err != nil {
		return err
	}
	reply, err := s.findReply(ctx, postID, replyID)
	if err != nil {
		return err
	}
	if !canDelete(userID, reply.UserID, p, isModerator) {
		return httpx.ErrForbidden
	}
	res := s.db.WithContext(ctx).Delete(&SolutionReply{}, replyID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (s *DiscussionService) findPost(ctx context.Context, problemID, postID int64) (*SolutionPost, error) {
	var post SolutionPost
	err := s.db.WithContext(ctx).Where("id = ? AND problem_id = ?", postID, problemID).First(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func (s *DiscussionService) findReply(ctx context.Context, postID, replyID int64) (*SolutionReply, error) {
	var reply SolutionReply
	err := s.db.WithContext(ctx).Where("id = ? AND post_id = ?", replyID, postID).First(&reply).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

// postDetail 组装帖子详情：作者摘要、回复数与回复列表。
func (s *DiscussionService) postDetail(ctx context.Context, p *Problem, userID int64, isModerator bool, post *SolutionPost) (*SolutionPostDetail, error) {
	item := SolutionPostItem{
		ID: post.ID, ProblemID: post.ProblemID, UserID: post.UserID,
		Title: post.Title, Excerpt: excerpt(post.ContentMD),
		CreatedAt: post.CreatedAt.UTC(), UpdatedAt: post.UpdatedAt.UTC(),
		CanEdit: canEdit(userID, post.UserID), CanDelete: canDelete(userID, post.UserID, p, isModerator),
	}

	var replies []SolutionReply
	if err := s.db.WithContext(ctx).Model(&SolutionReply{}).
		Where("post_id = ?", post.ID).
		Order("created_at ASC, id ASC").
		Limit(maxRepliesPerPost).
		Find(&replies).Error; err != nil {
		return nil, err
	}

	authorIDs := make([]int64, 0, len(replies)+1)
	authorIDs = append(authorIDs, post.UserID)
	for _, r := range replies {
		authorIDs = append(authorIDs, r.UserID)
	}
	authors, err := s.authors(ctx, authorIDs)
	if err != nil {
		return nil, err
	}

	likeCounts, err := s.likeCounts(ctx, []int64{post.ID})
	if err != nil {
		return nil, err
	}
	likedByMe, err := s.likedByMe(ctx, userID, []int64{post.ID})
	if err != nil {
		return nil, err
	}

	item.Author = authors[post.UserID]
	item.ReplyCount = int64(len(replies))
	item.LikeCount = likeCounts[post.ID]
	item.LikedByMe = likedByMe[post.ID]
	replyItems := make([]SolutionReplyItem, 0, len(replies))
	for _, r := range replies {
		replyItems = append(replyItems, SolutionReplyItem{
			ID: r.ID, PostID: r.PostID, UserID: r.UserID, ContentMD: r.ContentMD,
			Author:    authors[r.UserID],
			CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
			CanEdit: canEdit(userID, r.UserID), CanDelete: canDelete(userID, r.UserID, p, isModerator),
		})
	}
	return &SolutionPostDetail{SolutionPostItem: item, ContentMD: post.ContentMD, Replies: replyItems}, nil
}

func (s *DiscussionService) replyItem(ctx context.Context, p *Problem, userID int64, isModerator bool, reply *SolutionReply) (*SolutionReplyItem, error) {
	authors, err := s.authors(ctx, []int64{reply.UserID})
	if err != nil {
		return nil, err
	}
	return &SolutionReplyItem{
		ID: reply.ID, PostID: reply.PostID, UserID: reply.UserID, ContentMD: reply.ContentMD,
		Author:    authors[reply.UserID],
		CreatedAt: reply.CreatedAt.UTC(), UpdatedAt: reply.UpdatedAt.UTC(),
		CanEdit: canEdit(userID, reply.UserID), CanDelete: canDelete(userID, reply.UserID, p, isModerator),
	}, nil
}

// postItems 批量组装列表项，避免 N+1：一次查询回复数与作者摘要。
func (s *DiscussionService) postItems(ctx context.Context, p *Problem, userID int64, isModerator bool, posts []SolutionPost) ([]SolutionPostItem, error) {
	items := make([]SolutionPostItem, 0, len(posts))
	if len(posts) == 0 {
		return items, nil
	}
	ids := make([]int64, len(posts))
	authorIDs := make([]int64, 0, len(posts))
	seen := map[int64]struct{}{}
	for i := range posts {
		ids[i] = posts[i].ID
		if _, ok := seen[posts[i].UserID]; !ok {
			seen[posts[i].UserID] = struct{}{}
			authorIDs = append(authorIDs, posts[i].UserID)
		}
	}

	replyCounts, err := s.replyCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	authors, err := s.authors(ctx, authorIDs)
	if err != nil {
		return nil, err
	}
	likeCounts, err := s.likeCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	likedByMe, err := s.likedByMe(ctx, userID, ids)
	if err != nil {
		return nil, err
	}

	for _, post := range posts {
		items = append(items, SolutionPostItem{
			ID: post.ID, ProblemID: post.ProblemID, UserID: post.UserID,
			Title: post.Title, Excerpt: excerpt(post.ContentMD),
			Author: authors[post.UserID], ReplyCount: replyCounts[post.ID],
			LikeCount: likeCounts[post.ID], LikedByMe: likedByMe[post.ID],
			CreatedAt: post.CreatedAt.UTC(), UpdatedAt: post.UpdatedAt.UTC(),
			CanEdit: canEdit(userID, post.UserID), CanDelete: canDelete(userID, post.UserID, p, isModerator),
		})
	}
	return items, nil
}

func (s *DiscussionService) likeCounts(ctx context.Context, postIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(postIDs))
	if len(postIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		PostID int64
		N      int64
	}
	if err := s.db.WithContext(ctx).Model(&SolutionPostLike{}).
		Select("post_id, COUNT(*) AS n").
		Where("post_id IN ?", postIDs).
		Group("post_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PostID] = r.N
	}
	return out, nil
}

func (s *DiscussionService) likedByMe(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(postIDs))
	if userID <= 0 || len(postIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		PostID int64
	}
	if err := s.db.WithContext(ctx).Model(&SolutionPostLike{}).
		Select("post_id").
		Where("user_id = ? AND post_id IN ?", userID, postIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PostID] = true
	}
	return out, nil
}

func (s *DiscussionService) replyCounts(ctx context.Context, postIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(postIDs))
	if len(postIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		PostID int64
		N      int64
	}
	if err := s.db.WithContext(ctx).Model(&SolutionReply{}).
		Select("post_id, COUNT(*) AS n").
		Where("post_id IN ?", postIDs).
		Group("post_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PostID] = r.N
	}
	return out, nil
}

func (s *DiscussionService) authors(ctx context.Context, userIDs []int64) (map[int64]AuthorSummary, error) {
	out := make(map[int64]AuthorSummary, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ID       int64
		Username string
		Nickname string
	}
	if err := s.db.WithContext(ctx).Table("users").
		Select("id, username, nickname").
		Where("id IN ?", userIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = AuthorSummary{ID: r.ID, Username: r.Username, Nickname: r.Nickname}
	}
	return out, nil
}

func canEdit(userID, authorID int64) bool {
	return userID > 0 && userID == authorID
}

func canDelete(userID, authorID int64, p *Problem, isModerator bool) bool {
	if canEdit(userID, authorID) {
		return true
	}
	if isModerator {
		return true
	}
	return userID > 0 && p.CreatedBy == userID
}

func validatePostInput(in SolutionPostInput) (string, error) {
	title := strings.TrimSpace(in.Title)
	if n := runes(title); n < 1 || n > maxPostTitleChars {
		return "", httpx.ErrValidation("标题长度需为 1–100 个字符")
	}
	if strings.TrimSpace(in.ContentMD) == "" {
		return "", httpx.ErrValidation("帖子正文不能为空")
	}
	if n := runes(in.ContentMD); n > maxPostContentChars {
		return "", httpx.ErrValidation("帖子正文不能超过 50000 个字符")
	}
	return title, nil
}

func validateReplyInput(in SolutionReplyInput) error {
	if strings.TrimSpace(in.ContentMD) == "" {
		return httpx.ErrValidation("回复不能为空")
	}
	if n := runes(in.ContentMD); n > maxReplyContentChars {
		return httpx.ErrValidation("回复不能超过 10000 个字符")
	}
	return nil
}

// excerpt 截取前 postExcerptChars 个字符（按 rune）。
func excerpt(s string) string {
	r := []rune(s)
	if len(r) <= postExcerptChars {
		return s
	}
	return string(r[:postExcerptChars])
}
