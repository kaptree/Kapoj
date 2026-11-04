package contest

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

type Viewer struct {
	UserID    int64
	CanManage bool
}

type ListQuery struct {
	Keyword       string
	Mode          Mode
	Status        Status
	IncludeHidden bool
}

func (q ListQuery) Normalize() (ListQuery, error) {
	q.Keyword = strings.TrimSpace(q.Keyword)
	if !utf8.ValidString(q.Keyword) {
		return q, invalid("keyword 包含无效字符")
	}
	if utf8.RuneCountInString(q.Keyword) > 100 {
		return q, invalid("keyword 不能超过 100 个字符")
	}
	for _, r := range q.Keyword {
		if unicode.IsControl(r) {
			return q, invalid("keyword 不能包含控制字符")
		}
	}
	if q.Mode != "" && !q.Mode.Valid() {
		return q, invalid("赛制必须是 icpc、ioi 或 oi")
	}
	if q.Status != "" && q.Status != StatusUpcoming && q.Status != StatusRunning && q.Status != StatusEnded {
		return q, invalid("status 必须是 upcoming、running 或 ended")
	}
	return q, nil
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, v Viewer, in Input) (*Contest, error) {
	if !v.CanManage {
		return nil, httpx.ErrForbidden
	}
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	if in.RegisterType == RegisterPassword && in.Password == "" {
		return nil, invalid("密码报名必须设置比赛密码")
	}
	if in.RegisterType == RegisterInvite && in.InviteCode == "" {
		return nil, invalid("邀请码报名必须设置邀请码")
	}
	c := &Contest{
		Title: in.Title, DescriptionMD: in.DescriptionMD,
		StartTime: in.StartTime, EndTime: in.EndTime, Mode: in.Mode,
		IsRated: in.IsRated, IsPublic: in.IsPublic, RegisterType: in.RegisterType,
		CreatedBy: v.UserID,
	}
	if c.PasswordHash, err = hash(in.Password); err != nil {
		return nil, err
	}
	if c.InviteCodeHash, err = hash(in.InviteCode); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, c, toProblems(in.Problems)); err != nil {
		return nil, mapRepoErr(err)
	}
	return c, nil
}

func (s *Service) Update(ctx context.Context, v Viewer, id int64, in Input) (*Contest, error) {
	if !v.CanManage {
		return nil, httpx.ErrForbidden
	}
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	in, err = in.Normalize()
	if err != nil {
		return nil, err
	}
	if in.RegisterType == RegisterPassword && in.Password == "" && existing.PasswordHash == "" {
		return nil, invalid("密码报名必须设置比赛密码")
	}
	if in.RegisterType == RegisterInvite && in.InviteCode == "" && existing.InviteCodeHash == "" {
		return nil, invalid("邀请码报名必须设置邀请码")
	}
	now := time.Now()
	if !now.Before(existing.StartTime) {
		if !in.StartTime.Equal(existing.StartTime) {
			return nil, invalid("比赛开始后不能修改开始时间")
		}
		if in.Mode != existing.Mode {
			return nil, invalid("比赛开始后不能修改赛制")
		}
		rows, err := s.repo.FindProblems(ctx, id)
		if err != nil {
			return nil, err
		}
		if !sameProblems(rows, in.Problems) {
			return nil, invalid("比赛开始后不能修改题目列表")
		}
	}
	c := *existing
	c.Title, c.DescriptionMD = in.Title, in.DescriptionMD
	c.StartTime, c.EndTime, c.Mode = in.StartTime, in.EndTime, in.Mode
	c.IsRated, c.IsPublic, c.RegisterType = in.IsRated, in.IsPublic, in.RegisterType
	if in.Password != "" {
		if c.PasswordHash, err = hash(in.Password); err != nil {
			return nil, err
		}
	}
	if in.InviteCode != "" {
		if c.InviteCodeHash, err = hash(in.InviteCode); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, &c, toProblems(in.Problems)); err != nil {
		return nil, mapRepoErr(err)
	}
	return &c, nil
}

func (s *Service) Detail(ctx context.Context, v Viewer, id int64) (*Detail, error) {
	c, err := s.findAccessible(ctx, v, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	rows, err := s.repo.FindProblems(ctx, id)
	if err != nil {
		return nil, err
	}
	problemCount := int64(len(rows))
	if !v.CanManage && now.Before(c.StartTime) {
		rows = nil
	}
	problems := make([]ProblemItem, len(rows))
	for i, row := range rows {
		problems[i] = toProblemItem(row)
	}
	registered := false
	if v.UserID != 0 {
		registered, err = s.repo.IsParticipant(ctx, id, v.UserID)
		if err != nil {
			return nil, err
		}
	}
	participants, err := s.repo.CountParticipants(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Detail{
		ListItem: toListItem(*c, problemCount, participants, now),
		Problems: problems, Registered: registered, CanManage: v.CanManage, ServerTime: now,
	}, nil
}

func (s *Service) List(ctx context.Context, v Viewer, q ListQuery, page, size int) (httpx.Page[ListItem], error) {
	q, err := q.Normalize()
	if err != nil {
		return httpx.Page[ListItem]{}, err
	}
	page = min(max(page, 1), 1_000_000)
	size = min(max(size, 1), 100)
	now := time.Now()
	rows, total, err := s.repo.List(ctx, ListFilter{
		Keyword: q.Keyword, Mode: q.Mode, Status: q.Status,
		IncludeHidden: q.IncludeHidden && v.CanManage, Now: now,
	}, (page-1)*size, size)
	if err != nil {
		return httpx.Page[ListItem]{}, err
	}
	items := make([]ListItem, len(rows))
	for i, row := range rows {
		items[i] = toListItem(row.Contest, row.ProblemCount, row.ParticipantCount, now)
	}
	return httpx.Page[ListItem]{Total: total, Items: items}, nil
}

func (s *Service) Delete(ctx context.Context, v Viewer, id int64) error {
	if !v.CanManage {
		return httpx.ErrForbidden
	}
	return mapRepoErr(s.repo.Delete(ctx, id))
}

func (s *Service) Clone(ctx context.Context, v Viewer, id int64, in Input) (*Contest, error) {
	if !v.CanManage {
		return nil, httpx.ErrForbidden
	}
	original, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		base := []rune(original.Title)
		if len(base) > 96 {
			base = base[:96]
		}
		title = string(base) + "（副本）"
	}
	if n := utf8.RuneCountInString(title); n < 1 || n > 100 {
		return nil, invalid("比赛标题长度需为 1–100 个字符")
	}
	start, end := original.StartTime, original.EndTime
	if !in.StartTime.IsZero() {
		start = in.StartTime.UTC()
	}
	if !in.EndTime.IsZero() {
		end = in.EndTime.UTC()
	}
	if !start.Before(end) {
		return nil, invalid("比赛结束时间必须晚于开始时间")
	}
	c := *original
	c.ID, c.Title, c.StartTime, c.EndTime = 0, title, start, end
	c.CreatedBy = v.UserID
	c.CreatedAt, c.UpdatedAt = time.Time{}, time.Time{}
	rows, err := s.repo.FindProblems(ctx, id)
	if err != nil {
		return nil, err
	}
	problems := make([]ContestProblem, len(rows))
	for i, row := range rows {
		problems[i] = ContestProblem{
			ProblemID: row.ProblemID, DisplayID: row.DisplayID,
			SortOrder: i + 1, Score: row.Score,
		}
	}
	if err := s.repo.Create(ctx, &c, problems); err != nil {
		return nil, mapRepoErr(err)
	}
	return &c, nil
}

func (s *Service) Register(ctx context.Context, userID, id int64, password, inviteCode string) error {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapRepoErr(err)
	}
	if !time.Now().Before(c.EndTime) {
		return invalid("比赛已结束，无法报名")
	}
	registered, err := s.repo.IsParticipant(ctx, id, userID)
	if err != nil {
		return err
	}
	if registered {
		return nil
	}
	switch c.RegisterType {
	case RegisterPassword:
		if bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(strings.TrimSpace(password))) != nil {
			return invalid("比赛密码错误")
		}
	case RegisterInvite:
		if bcrypt.CompareHashAndPassword([]byte(c.InviteCodeHash), []byte(strings.TrimSpace(inviteCode))) != nil {
			return invalid("邀请码错误")
		}
	}
	return s.repo.AddParticipant(ctx, id, userID)
}

func (s *Service) Participants(ctx context.Context, v Viewer, id int64) ([]ParticipantItem, error) {
	if !v.CanManage {
		return nil, httpx.ErrForbidden
	}
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, mapRepoErr(err)
	}
	rows, err := s.repo.ListParticipants(ctx, id)
	if err != nil {
		return nil, err
	}
	items := make([]ParticipantItem, len(rows))
	for i, row := range rows {
		items[i] = ParticipantItem(row)
	}
	return items, nil
}

func (s *Service) Announcements(ctx context.Context, v Viewer, id int64) ([]AnnouncementItem, error) {
	if _, err := s.findAccessible(ctx, v, id); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListAnnouncements(ctx, id)
	if err != nil {
		return nil, err
	}
	items := make([]AnnouncementItem, len(rows))
	for i, a := range rows {
		items[i] = toAnnouncementItem(a)
	}
	return items, nil
}

func (s *Service) CreateAnnouncement(ctx context.Context, v Viewer, id int64, title, content string) (*AnnouncementItem, error) {
	if !v.CanManage {
		return nil, httpx.ErrForbidden
	}
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, mapRepoErr(err)
	}
	title, err := normalizeAnnouncement(title, content)
	if err != nil {
		return nil, err
	}
	a := &Announcement{ContestID: id, Title: title, ContentMD: content, CreatedBy: v.UserID}
	if err := s.repo.AddAnnouncement(ctx, a); err != nil {
		return nil, err
	}
	item := toAnnouncementItem(*a)
	return &item, nil
}

func (s *Service) UpdateAnnouncement(ctx context.Context, v Viewer, id, aid int64, title, content string) error {
	if !v.CanManage {
		return httpx.ErrForbidden
	}
	title, err := normalizeAnnouncement(title, content)
	if err != nil {
		return err
	}
	return mapAnnouncementErr(s.repo.UpdateAnnouncement(ctx, id, aid, title, content))
}

func (s *Service) DeleteAnnouncement(ctx context.Context, v Viewer, id, aid int64) error {
	if !v.CanManage {
		return httpx.ErrForbidden
	}
	return mapAnnouncementErr(s.repo.DeleteAnnouncement(ctx, id, aid))
}

func (s *Service) findAccessible(ctx context.Context, v Viewer, id int64) (*Contest, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if c.IsPublic || v.CanManage {
		return c, nil
	}
	if v.UserID != 0 {
		ok, err := s.repo.IsParticipant(ctx, id, v.UserID)
		if err != nil {
			return nil, err
		}
		if ok {
			return c, nil
		}
	}
	return nil, httpx.ErrNotFound
}

func sameProblems(rows []ProblemRow, inputs []ProblemInput) bool {
	if len(rows) != len(inputs) {
		return false
	}
	for i, row := range rows {
		if row.ProblemID != inputs[i].ProblemID ||
			row.DisplayID != inputs[i].DisplayID ||
			row.Score != inputs[i].Score {
			return false
		}
	}
	return true
}

func toProblems(inputs []ProblemInput) []ContestProblem {
	out := make([]ContestProblem, len(inputs))
	for i, p := range inputs {
		out[i] = ContestProblem{ProblemID: p.ProblemID, DisplayID: p.DisplayID, SortOrder: i + 1, Score: p.Score}
	}
	return out
}

func hash(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	h, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	return string(h), err
}

func normalizeAnnouncement(title, content string) (string, error) {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n < 1 || n > 100 {
		return "", invalid("公告标题长度需为 1–100 个字符")
	}
	if utf8.RuneCountInString(content) > 50000 {
		return "", invalid("公告内容不能超过 50000 个字符")
	}
	return title, nil
}

func toAnnouncementItem(a Announcement) AnnouncementItem {
	return AnnouncementItem{
		ID: a.ID, ContestID: a.ContestID, Title: a.Title,
		ContentMD: a.ContentMD, CreatedAt: a.CreatedAt,
	}
}

func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrAnnouncementNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, ErrDuplicateDisplay):
		return invalid("同一比赛的题目编号不能重复")
	case errors.Is(err, ErrDuplicateProblem):
		return invalid("同一比赛不能重复添加题目")
	case errors.Is(err, ErrProblemNotFound):
		return invalid("题目不存在")
	case errors.Is(err, ErrProblemNotInContest):
		return invalid("题目不在该比赛中")
	case errors.Is(err, ErrHasSubmissions):
		return invalid("比赛已有提交，无法删除")
	case errors.Is(err, ErrStartFrozen):
		return invalid("比赛开始后不能修改开始时间")
	case errors.Is(err, ErrModeFrozen):
		return invalid("比赛开始后不能修改赛制")
	}
	return err
}

func mapAnnouncementErr(err error) error {
	if errors.Is(err, ErrAnnouncementNotFound) {
		return httpx.ErrNotFound
	}
	return err
}
