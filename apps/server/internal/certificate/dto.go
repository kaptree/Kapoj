package certificate

import (
	"fmt"
	"time"
)

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

// CertificateView 是证书的 API 表示。ImageURL 恒为鉴权图片接口地址，仅本人与
// 管理员可通过带 Bearer 的请求读取；Type 使用提交时快照的类型名；VerifyCode 仅
// 在 approved 时非空。
type CertificateView struct {
	ID           int64      `json:"id"`
	Type         string     `json:"type"`
	Level        string     `json:"level"`
	Status       Status     `json:"status"`
	ImageURL     string     `json:"imageUrl"`
	RejectReason string     `json:"rejectReason"`
	VerifyCode   string     `json:"verifyCode"`
	SubmittedAt  time.Time  `json:"submittedAt"`
	ReviewedAt   *time.Time `json:"reviewedAt"`
}

// PublicCertificateView 是公开核验结果。仅当 Valid 为 true 时携带类型/等级/
// 昵称/通过时间；绝不返回图片、邮箱、用户 ID、审核人、驳回原因等敏感信息。
type PublicCertificateView struct {
	Valid      bool       `json:"valid"`
	Type       string     `json:"type,omitempty"`
	Level      string     `json:"level,omitempty"`
	Nickname   string     `json:"nickname,omitempty"`
	ApprovedAt *time.Time `json:"approvedAt,omitempty"`
}

// ProfileCertificateView 是用户公开主页上的证书摘要。仅包含已通过证书的
// 非敏感字段：不返回图片、状态、驳回原因、审核人、用户 ID 或邮箱。
type ProfileCertificateView struct {
	ID         int64      `json:"id"`
	Type       string     `json:"type"`
	Level      string     `json:"level"`
	VerifyCode string     `json:"verifyCode"`
	ApprovedAt *time.Time `json:"approvedAt"`
}

// Image 是证书图片的原始字节与内容类型（内容类型取自数据库，而非重新探测）。
type Image struct {
	Data        []byte
	ContentType string
}

func toCertificateView(c Certificate) CertificateView {
	v := CertificateView{
		ID:           c.ID,
		Type:         c.CertificateTypeName,
		Level:        c.Level,
		Status:       c.Status,
		ImageURL:     fmt.Sprintf("/api/certificates/%d/image", c.ID),
		RejectReason: c.RejectReason,
		SubmittedAt:  c.SubmittedAt,
		ReviewedAt:   c.ReviewedAt,
	}
	if c.VerifyCode != nil {
		v.VerifyCode = *c.VerifyCode
	}
	return v
}

// AdminFilter 是管理员审核列表的筛选条件。Status 为空表示不筛选（返回全部状态）；
// 分页的 offset/limit 由仓储层独立接收，不并入筛选结构体。
type AdminFilter struct {
	Status Status
}

// CertificateUserView 是管理员列表中证书持有者的摘要，仅包含非敏感字段。
type CertificateUserView struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// AdminCertificateView 是管理员列表项：内嵌证书视图，附持有者摘要。
type AdminCertificateView struct {
	CertificateView
	User CertificateUserView `json:"user"`
}

// ReviewInput 是审核输入。Action 取 approve/reject；reject 时 Reason 必填 1–500 字，
// approve 忽略 Reason。
type ReviewInput struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}
