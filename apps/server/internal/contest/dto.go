package contest

import "time"

type ListItem struct {
	ID               int64        `json:"id"`
	Title            string       `json:"title"`
	DescriptionMD    string       `json:"descriptionMd"`
	StartTime        time.Time    `json:"startTime"`
	EndTime          time.Time    `json:"endTime"`
	Mode             Mode         `json:"mode"`
	IsRated          bool         `json:"isRated"`
	IsPublic         bool         `json:"isPublic"`
	DisableAI        bool         `json:"disableAi"`
	RegisterType     RegisterType `json:"registerType"`
	Status           Status       `json:"status"`
	ProblemCount     int64        `json:"problemCount"`
	ParticipantCount int64        `json:"participantCount"`
	Registered       bool         `json:"registered"`
	CreatedAt        time.Time    `json:"createdAt"`
}

type ProblemItem struct {
	ProblemID     int64  `json:"problemId"`
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	DisplayID     string `json:"displayId"`
	SortOrder     int    `json:"sortOrder"`
	Score         int    `json:"score"`
	TimeLimitMS   int    `json:"timeLimitMs"`
	MemoryLimitMB int    `json:"memoryLimitMb"`
	IsPublic      bool   `json:"isPublic"`
	Locked        bool   `json:"locked"`
	SubmitCount   int    `json:"submitCount"`
	AcceptCount   int    `json:"acceptCount"`
	MyStatus      string `json:"myStatus"`
	MyAttempts    int    `json:"myAttempts"`
}

type Detail struct {
	ListItem
	Problems   []ProblemItem `json:"problems"`
	Registered bool          `json:"registered"`
	CanManage  bool          `json:"canManage"`
	ServerTime time.Time     `json:"serverTime"`
}

type ParticipantItem struct {
	UserID       int64     `json:"userId"`
	Username     string    `json:"username"`
	Nickname     string    `json:"nickname"`
	RegisterTime time.Time `json:"registerTime"`
}

type AnnouncementItem struct {
	ID        int64     `json:"id"`
	ContestID int64     `json:"contestId"`
	Title     string    `json:"title"`
	ContentMD string    `json:"contentMd"`
	CreatedAt time.Time `json:"createdAt"`
}

func toListItem(c Contest, problems, participants int64, registered bool, now time.Time) ListItem {
	return ListItem{
		ID: c.ID, Title: c.Title, DescriptionMD: c.DescriptionMD,
		StartTime: c.StartTime, EndTime: c.EndTime, Mode: c.Mode,
		IsRated: c.IsRated, IsPublic: c.IsPublic, DisableAI: c.DisableAI,
		RegisterType: c.RegisterType,
		Status:       StatusAt(c.StartTime, c.EndTime, now),
		ProblemCount: problems, ParticipantCount: participants, Registered: registered, CreatedAt: c.CreatedAt,
	}
}

func toProblemItem(row ProblemRow) ProblemItem {
	return ProblemItem{
		ProblemID: row.ProblemID, Title: row.ProblemTitle, Slug: row.ProblemSlug,
		DisplayID: row.DisplayID, SortOrder: row.SortOrder, Score: row.Score,
		TimeLimitMS: row.ProblemTimeLimitMS, MemoryLimitMB: row.ProblemMemoryLimitMB,
		IsPublic: row.ProblemIsPublic,
	}
}
