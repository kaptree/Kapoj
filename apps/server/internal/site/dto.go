package site

import "time"

// Link is one editable home page card or CTA. Icon uses PrimeIcons class names
// such as "pi pi-book"; To is always an internal absolute path.
type Link struct {
	Icon        string `json:"icon"`
	IconPos     string `json:"iconPos"`
	Title       string `json:"title"`
	Description string `json:"description"`
	To          string `json:"to"`
}

// HomeContent is the structured, editable home page content stored in
// site_settings.home_json.
type HomeContent struct {
	Eyebrow          string `json:"eyebrow"`
	Title            string `json:"title"`
	TitleHighlight   string `json:"titleHighlight"`
	Description      string `json:"description"`
	PrimaryCTA       Link   `json:"primaryCta"`
	SecondaryCTA     Link   `json:"secondaryCta"`
	QuickTitle       string `json:"quickTitle"`
	QuickLinks       []Link `json:"quickLinks"`
	GrowthTitle      string `json:"growthTitle"`
	GrowthLinks      []Link `json:"growthLinks"`
	GuestTitle       string `json:"guestTitle"`
	GuestDescription string `json:"guestDescription"`
}

// FooterInput is the admin request body for updating the site footer.
type FooterInput struct {
	ICPNumber    string    `json:"icpNumber"`
	PoliceNumber string    `json:"policeNumber"`
	PoliceURL    string    `json:"policeUrl"`
	Copyright    string    `json:"copyright"`
	Text         string    `json:"text"`
	StartedAt    time.Time `json:"startedAt"`
	ShowVisits   bool      `json:"showVisits"`
}

// FooterView is the public read model of the footer; it never exposes the
// storage keys or the editor identity.
type FooterView struct {
	ICPNumber    string `json:"icpNumber"`
	PoliceNumber string `json:"policeNumber"`
	PoliceURL    string `json:"policeUrl"`
	Copyright    string `json:"copyright"`
	Text         string `json:"text"`
	ShowVisits   bool   `json:"showVisits"`
}

// PublicView is the public site configuration returned to visitors.
type PublicView struct {
	LogoURL     string      `json:"logoUrl"`
	FaviconURL  string      `json:"faviconUrl"`
	Home        HomeContent `json:"home"`
	Footer      FooterView  `json:"footer"`
	StartedAt   time.Time   `json:"startedAt"`
	RunningDays int         `json:"runningDays"`
	VisitCount  int64       `json:"visitCount"`
}

// AdminView is the complete admin read model. The admin page needs the same
// fields as the public view (footer + startedAt + visitCount).
type AdminView = PublicView
