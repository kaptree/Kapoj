package learning

import "time"

// Viewer 描述当前请求者。零值表示游客。
//
// CanManageOfficial 对应 learning_path.official.manage：可创建/管理官方路线、
// 管理任意路线的上下架与节点写入。
// CanViewAll 对应 learning_path.admin.read：只读查看全部路线、下架路线与完整
// 题目 ID，绝不参与任何写授权。
type Viewer struct {
	UserID            int64
	CanManageOfficial bool
	CanViewAll        bool
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

// NodeInput 是创建/更新学习路线节点的可编辑字段。RequiredCount 未提交时
// 默认等于题目数。
type NodeInput struct {
	Title         string  `json:"title"`
	DescriptionMD string  `json:"descriptionMd"`
	ParentID      *int64  `json:"parentId"`
	ProblemIDs    []int64 `json:"problemIds"`
	SortOrder     int     `json:"sortOrder"`
	RequiredCount *int    `json:"requiredCount"`
}

// ProblemSummary 是节点树中暴露的公开题目摘要，不含题干/题解。Solved 表示
// 当前查看者是否在该题目上有 AC；游客恒为 false。
type ProblemSummary struct {
	ProblemID  int64  `json:"problemId"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	Difficulty int    `json:"difficulty"`
	Solved     bool   `json:"solved"`
}

// NodeItem 是学习路线树中的一个节点。Problems 只包含公开题目的摘要；
// 公开树对非作者/非管理员只返回公开题目的 ProblemIDs，作者/管理员保留
// 节点引用的全部题目顺序（含已下线/缺失题目）供编辑。
type NodeItem struct {
	ID            int64            `json:"id"`
	ParentID      *int64           `json:"parentId"`
	Title         string           `json:"title"`
	DescriptionMD string           `json:"descriptionMd"`
	ProblemIDs    []int64          `json:"problemIds"`
	Problems      []ProblemSummary `json:"problems"`
	SortOrder     int              `json:"sortOrder"`
	RequiredCount int              `json:"requiredCount"`
	SolvedCount   int              `json:"solvedCount"`
	Status        string           `json:"status"`
	Children      []NodeItem       `json:"children"`
}

// Tree 是学习路线的结构与题单元信息。
type Tree struct {
	Path  PathItem   `json:"path"`
	Nodes []NodeItem `json:"nodes"`
}
