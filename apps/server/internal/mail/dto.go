package mail

import "time"

// AdminView is the read model returned by the admin mail settings API. It never
// contains the SMTP password or its ciphertext.
type AdminView struct {
	Provider                 Provider      `json:"provider"`
	SMTPHost                 string        `json:"smtpHost"`
	SMTPPort                 int           `json:"smtpPort"`
	SMTPTLS                  TLSMode       `json:"smtpTls"`
	SMTPUsername             string        `json:"smtpUsername"`
	PasswordState            PasswordState `json:"passwordState"`
	FromEmail                string        `json:"fromEmail"`
	FromName                 string        `json:"fromName"`
	ReplyTo                  string        `json:"replyTo"`
	PublicBaseURL            string        `json:"publicBaseUrl"`
	SendingEnabled           bool          `json:"sendingEnabled"`
	RequireEmailVerification bool          `json:"requireEmailVerification"`
	Ready                    bool          `json:"ready"`
	UpdatedBy                int64         `json:"updatedBy"`
	UpdatedAt                time.Time     `json:"updatedAt"`
}

// SettingsInput is the admin write model. SMTPPassword nil keeps the existing
// password, a non-empty value replaces it, and an empty value clears it.
type SettingsInput struct {
	Provider                 Provider `json:"provider"`
	SMTPHost                 string   `json:"smtpHost"`
	SMTPPort                 int      `json:"smtpPort"`
	SMTPTLS                  TLSMode  `json:"smtpTls"`
	SMTPUsername             string   `json:"smtpUsername"`
	SMTPPassword             *string  `json:"smtpPassword"`
	FromEmail                string   `json:"fromEmail"`
	FromName                 string   `json:"fromName"`
	ReplyTo                  string   `json:"replyTo"`
	PublicBaseURL            string   `json:"publicBaseUrl"`
	SendingEnabled           bool     `json:"sendingEnabled"`
	RequireEmailVerification bool     `json:"requireEmailVerification"`
}

// TestInput is the admin test-send request.
type TestInput struct {
	To string `json:"to"`
}
