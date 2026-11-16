package site

import (
	"context"
	"fmt"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	defaultBadgeSize = 22
	minBadgeSize     = 16
	maxBadgeSize     = 48
)

// Service implements site configuration reads and writes.
type Service struct {
	repo  Repository
	store storage.Storage
	now   func() time.Time
}

// NewService constructs a site service backed by repo and store.
func NewService(repo Repository, store storage.Storage) *Service {
	return &Service{repo: repo, store: store, now: time.Now}
}

// Public returns the public site configuration.
func (s *Service) Public(ctx context.Context) (*PublicView, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.VisitCount(ctx)
	if err != nil {
		return nil, err
	}
	return s.view(settings, count), nil
}

// Admin returns the complete admin view: the public view plus full deployment
// settings. The public subset (CDN/assets domains) is still present in
// PublicView.Deployment but is shadowed by the full field in JSON.
func (s *Service) Admin(ctx context.Context) (*AdminView, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.VisitCount(ctx)
	if err != nil {
		return nil, err
	}
	return &AdminView{
		PublicView: *s.view(settings, count),
		Deployment: settings.Deployment.Data,
	}, nil
}

// UpdateHome validates and stores the whole home page content.
func (s *Service) UpdateHome(ctx context.Context, actorID int64, in HomeContent) (*AdminView, error) {
	if err := validateHome(&in); err != nil {
		return nil, err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	settings.Home = dbtypes.JSON[HomeContent]{Data: in}
	settings.UpdatedBy = actorID
	if err := s.repo.Update(ctx, settings); err != nil {
		return nil, err
	}
	return s.Admin(ctx)
}

// UpdateFooter validates and stores the footer fields and running start date.
func (s *Service) UpdateFooter(ctx context.Context, actorID int64, in FooterInput) (*AdminView, error) {
	if err := validateFooter(&in); err != nil {
		return nil, err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	settings.FooterICP = in.ICPNumber
	settings.FooterPolice = in.PoliceNumber
	settings.FooterPoliceURL = in.PoliceURL
	settings.FooterCopyright = in.Copyright
	settings.FooterText = in.Text
	settings.StartedAt = in.StartedAt
	settings.ShowVisits = in.ShowVisits
	settings.UpdatedBy = actorID
	if err := s.repo.Update(ctx, settings); err != nil {
		return nil, err
	}
	return s.Admin(ctx)
}

// UpdateAppearance validates and stores visual settings shared by public
// identity components.
func (s *Service) UpdateAppearance(ctx context.Context, actorID int64, in Appearance) (*AdminView, error) {
	if in.BadgeSize < minBadgeSize || in.BadgeSize > maxBadgeSize {
		return nil, httpx.ErrValidation(fmt.Sprintf("徽章大小需为 %d–%d px", minBadgeSize, maxBadgeSize))
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	settings.BadgeSize = in.BadgeSize
	settings.UpdatedBy = actorID
	if err := s.repo.Update(ctx, settings); err != nil {
		return nil, err
	}
	return s.Admin(ctx)
}

// UpdateCompanion validates and stores the site-wide companion defaults.
func (s *Service) UpdateCompanion(ctx context.Context, actorID int64, in CompanionSettings) (*AdminView, error) {
	if err := validateCompanion(&in); err != nil {
		return nil, err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	settings.Companion = dbtypes.JSON[CompanionSettings]{Data: in}
	settings.UpdatedBy = actorID
	if err := s.repo.Update(ctx, settings); err != nil {
		return nil, err
	}
	return s.Admin(ctx)
}

// UpdateDeployment validates and stores the deployment metadata.
func (s *Service) UpdateDeployment(ctx context.Context, actorID int64, in Deployment) (*AdminView, error) {
	if err := validateDeployment(&in); err != nil {
		return nil, err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	settings.Deployment = dbtypes.JSON[Deployment]{Data: in}
	settings.UpdatedBy = actorID
	if err := s.repo.Update(ctx, settings); err != nil {
		return nil, err
	}
	return s.Admin(ctx)
}

// RecordVisit atomically increments the total visit counter and returns the new
// value. showVisits only controls footer display, never counting.
func (s *Service) RecordVisit(ctx context.Context) (int64, error) {
	return s.repo.IncrementVisit(ctx)
}

func (s *Service) view(settings *Settings, visitCount int64) *PublicView {
	now := s.now().UTC()
	startedAt := settings.StartedAt.UTC()
	runningDays := 0
	if !startedAt.IsZero() {
		runningDays = int(now.Sub(startedAt) / (24 * time.Hour))
		if runningDays < 0 {
			runningDays = 0
		}
	}

	home := settings.Home.Data
	if isZeroHome(home) {
		home = DefaultHomeContent()
	}

	badgeSize := settings.BadgeSize
	if badgeSize < minBadgeSize || badgeSize > maxBadgeSize {
		badgeSize = defaultBadgeSize
	}

	view := &PublicView{
		Home: home,
		Footer: FooterView{
			ICPNumber:    settings.FooterICP,
			PoliceNumber: settings.FooterPolice,
			PoliceURL:    settings.FooterPoliceURL,
			Copyright:    settings.FooterCopyright,
			Text:         settings.FooterText,
			ShowVisits:   settings.ShowVisits,
		},
		Appearance: Appearance{BadgeSize: badgeSize},
		Companion:  normalizeCompanion(settings.Companion.Data),
		Deployment: PublicDeployment{
			CDNDomain:    settings.Deployment.Data.CDNDomain,
			AssetsDomain: settings.Deployment.Data.AssetsDomain,
		},
		StartedAt:   startedAt,
		RunningDays: runningDays,
		VisitCount:  visitCount,
	}
	// The timestamp version lets a replaced image bypass long-lived caches.
	if settings.LogoKey != "" {
		view.LogoURL = fmt.Sprintf("/api/site/logo?v=%d", settings.UpdatedAt.UnixNano())
	}
	if settings.FaviconKey != "" {
		view.FaviconURL = fmt.Sprintf("/api/site/favicon?v=%d", settings.UpdatedAt.UnixNano())
	}
	return view
}

func isZeroHome(home HomeContent) bool {
	return home.Eyebrow == "" &&
		home.Title == "" &&
		home.TitleHighlight == "" &&
		home.Tagline == "" &&
		home.Description == "" &&
		home.PrimaryCTA == (Link{}) &&
		home.SecondaryCTA == (Link{}) &&
		home.QuickTitle == "" &&
		len(home.QuickLinks) == 0 &&
		home.GrowthTitle == "" &&
		len(home.GrowthLinks) == 0 &&
		home.GuestTitle == "" &&
		home.GuestDescription == ""
}
