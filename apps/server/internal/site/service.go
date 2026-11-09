package site

import (
	"context"
	"fmt"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
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

// Admin returns the complete admin view. It is the same shape as the public
// view; the admin page reads the raw footer fields and counters from it.
func (s *Service) Admin(ctx context.Context) (*AdminView, error) {
	return s.Public(ctx)
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
