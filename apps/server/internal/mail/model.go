package mail

import "time"

// settingsID is the fixed primary key of the singleton mail_settings row.
const settingsID int16 = 1

// Provider identifies the SMTP preset shown in the admin UI. All providers use
// SMTP at runtime; the value only drives defaults and validation.
type Provider string

const (
	ProviderQQ         Provider = "qq"
	ProviderTencentSES Provider = "tencent_ses"
	ProviderAliyunDM   Provider = "aliyun_dm"
	ProviderCustom     Provider = "custom"
)

// TLSMode selects how the SMTP connection is protected.
type TLSMode string

const (
	TLSSSL      TLSMode = "ssl"
	TLSStartTLS TLSMode = "starttls"
	TLSNone     TLSMode = "none"
)

// PasswordState reports whether the stored SMTP password can be decrypted with
// the current mail key.
type PasswordState string

const (
	PasswordNone     PasswordState = "none"
	PasswordReady    PasswordState = "ready"
	PasswordKeyError PasswordState = "key_error"
)

// Settings is the singleton mail configuration row.
type Settings struct {
	ID                       int16     `gorm:"primaryKey"`
	Provider                 Provider  `gorm:"size:32;not null;default:'qq'"`
	SMTPHost                 string    `gorm:"size:255;not null;default:'smtp.qq.com'"`
	SMTPPort                 int       `gorm:"not null;default:465"`
	SMTPTLS                  TLSMode   `gorm:"size:16;not null;default:'ssl'"`
	SMTPUsername             string    `gorm:"size:255;not null;default:''"`
	SMTPPasswordEncrypted    string    `gorm:"type:text;not null;default:''"`
	FromEmail                string    `gorm:"size:254;not null;default:''"`
	FromName                 string    `gorm:"size:100;not null;default:'kapOJ'"`
	ReplyTo                  string    `gorm:"size:254;not null;default:''"`
	PublicBaseURL            string    `gorm:"size:500;not null;default:''"`
	SendingEnabled           bool      `gorm:"not null;default:false"`
	RequireEmailVerification bool      `gorm:"not null;default:true"`
	UpdatedBy                int64     `gorm:"not null;default:0"`
	UpdatedAt                time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the singleton table name stable.
func (Settings) TableName() string { return "mail_settings" }
