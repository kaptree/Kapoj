package learning

import "time"

// Viewer 描述当前请求者。零值表示游客。
type Viewer struct {
	UserID    int64
	CanManage bool
}

// UserSummary 是路线作者的公开摘要。
type UserSummary struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// PathItem 是学习路线列表/详情的元信息，不含节点树。
type PathItem struct {
	ID               int64       `json:"id"`
	Title            string      `json:"title"`
	DescriptionMD    string      `json:"descriptionMd"`
	CoverURL         string      `json:"coverUrl"`
	IsOfficial       bool        `json:"isOfficial"`
	IsPublic         bool        `json:"isPublic"`
	IsActive         bool        `json:"isActive"`
	Author           UserSummary `json:"author"`
	NodeCount        int         `json:"nodeCount"`
	MyCompletedCount int         `json:"myCompletedCount"`
	SortOrder        int         `json:"sortOrder"`
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

// PathInput 是创建/更新学习路线的可编辑字段。布尔指针区分“未提交”与
// “显式设置为 false”，更新时未提交的官方/上下架字段保持原值。
type PathInput struct {
	Title         string `json:"title"`
	DescriptionMD string `json:"descriptionMd"`
	CoverURL      string `json:"coverUrl"`
	IsOfficial    *bool  `json:"isOfficial"`
	IsPublic      *bool  `json:"isPublic"`
	IsActive      *bool  `json:"isActive"`
	SortOrder     int    `json:"sortOrder"`
}
