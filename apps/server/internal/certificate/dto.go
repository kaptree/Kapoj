package certificate

// TypeInput 是管理员创建或更新证书类型的输入。
// IsActive 使用指针区分「未提交」与显式 false：创建缺省为 true，更新缺省保留原值。
type TypeInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    *bool  `json:"isActive"`
	SortOrder   int    `json:"sortOrder"`
}

// TypeView 是证书类型的 API 表示。
type TypeView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"isActive"`
	SortOrder   int    `json:"sortOrder"`
}

func toTypeView(t CertificateType) TypeView {
	return TypeView{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		IsActive:    t.IsActive,
		SortOrder:   t.SortOrder,
	}
}
