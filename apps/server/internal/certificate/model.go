package certificate

import "time"

// CertificateType 是管理员维护的证书类型。name 唯一；历史证书保存提交时快照的
// 类型名，类型改名/停用/删除不回溯。IsActive 不设置数据库默认值，创建时由服务
// 显式赋值，否则显式停用的类型无法正确持久化。
type CertificateType struct {
	ID          int64     `gorm:"primaryKey"`
	Name        string    `gorm:"size:50;not null;uniqueIndex:uniq_certificate_types_name"`
	Description string    `gorm:"size:200;not null;default:''"`
	IsActive    bool      `gorm:"not null"`
	SortOrder   int       `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

// TableName 固定证书类型表名。
func (CertificateType) TableName() string { return "certificate_types" }

// Status 是证书认证的持久化状态。
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// Certificate 是用户提交的证书认证申请。image_key 仅在服务端使用；通过审核后
// 生成唯一 verify_code；pending/rejected 的 verify_code 为 NULL。
type Certificate struct {
	ID                  int64      `gorm:"primaryKey"`
	UserID              int64      `gorm:"not null;index:idx_certificates_user_submitted,priority:1"`
	CertificateTypeID   int64      `gorm:"column:certificate_type_id;not null;index:idx_certificates_type"`
	CertificateTypeName string     `gorm:"column:certificate_type_name;size:50;not null"`
	Level               string     `gorm:"size:50;not null"`
	ImageKey            string     `gorm:"column:image_key;size:255;not null"`
	ImageContentType    string     `gorm:"column:image_content_type;size:100;not null"`
	Status              Status     `gorm:"size:20;not null;default:pending;index:idx_certificates_status_submitted,priority:1"`
	RejectReason        string     `gorm:"column:reject_reason;size:500;not null;default:''"`
	VerifyCode          *string    `gorm:"column:verify_code;size:32;uniqueIndex:uniq_certificates_verify_code"`
	SubmittedAt         time.Time  `gorm:"column:submitted_at;type:timestamptz;not null;index:idx_certificates_user_submitted,priority:2;index:idx_certificates_status_submitted,priority:2"`
	ReviewedAt          *time.Time `gorm:"column:reviewed_at;type:timestamptz"`
	ReviewedBy          *int64     `gorm:"column:reviewed_by"`
	CreatedAt           time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt           time.Time  `gorm:"type:timestamptz;not null"`
}

// TableName 固定证书表名。
func (Certificate) TableName() string { return "certificates" }
