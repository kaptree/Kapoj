package userimport

import (
	"context"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const (
	issueLimit  = 500
	sampleLimit = 20
	queryChunk  = 500
)

type Summary struct {
	Total              int `json:"total"`
	Valid              int `json:"valid"`
	Conflicts          int `json:"conflicts"`
	Invalid            int `json:"invalid"`
	GeneratedUsernames int `json:"generatedUsernames"`
	Imported           int `json:"imported"`
	Skipped            int `json:"skipped"`
	Failed             int `json:"failed"`
}

type Issue struct {
	Line     int    `json:"line"`
	LegacyID int64  `json:"legacyId"`
	Uname    string `json:"uname"`
	Email    string `json:"email"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

type Mapping struct {
	LegacyID int64  `json:"legacyId"`
	Uname    string `json:"uname"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type Sample struct {
	Line      int    `json:"line"`
	LegacyID  int64  `json:"legacyId"`
	Uname     string `json:"uname"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Generated bool   `json:"generated"`
}

type Result struct {
	DryRun          bool      `json:"dryRun"`
	Summary         Summary   `json:"summary"`
	Issues          []Issue   `json:"issues"`
	IssuesTruncated bool      `json:"issuesTruncated"`
	Mapping         []Mapping `json:"mapping"`
	Samples         []Sample  `json:"samples"`
}

type AuditFunc func(tx *gorm.DB, summary Summary) error

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Process parses one HydroOJ CSV. With dryRun it only previews; otherwise it
// inserts the importable rows in one transaction and invokes audit in that
// same transaction.
func (s *Service) Process(ctx context.Context, r io.Reader, dryRun bool, audit AuditFunc) (*Result, error) {
	rows, err := parseHydroCSV(r)
	if err != nil {
		return nil, err
	}

	existingNames, existingEmails, err := s.loadExisting(ctx, rows)
	if err != nil {
		return nil, err
	}

	taken := cloneSet(existingNames)
	applyUsernames(rows, taken)
	markConflicts(rows, existingNames, existingEmails)

	result := buildResult(rows, dryRun)
	if dryRun {
		return result, nil
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		result.Summary.Skipped = result.Summary.Conflicts
		result.Mapping = make([]Mapping, 0)
		result.Summary.GeneratedUsernames = 0

		for _, item := range rows {
			if item.status != rowValid {
				continue
			}
			entity := &user.User{
				Username:        item.username,
				Nickname:        item.uname,
				Email:           item.email,
				EmailVerifiedAt: &now,
				PasswordHash:    item.hash,
				Salt:            item.salt,
				Role:            user.RoleUser,
				Status:          user.StatusActive,
				Rating:          user.InitialRating,
				Coins:           0,
			}
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(entity)
			if created.Error != nil {
				return created.Error
			}
			if created.RowsAffected == 0 {
				result.Summary.Skipped++
				appendIssue(result, Issue{
					Line:     item.line,
					LegacyID: item.legacyID,
					Uname:    item.uname,
					Email:    item.email,
					Status:   string(rowConflict),
					Reason:   "导入时用户名或邮箱已被占用",
				})
				continue
			}
			result.Summary.Imported++
			if item.generated {
				result.Mapping = append(result.Mapping, Mapping{
					LegacyID: item.legacyID,
					Uname:    item.uname,
					Email:    item.email,
					Username: item.username,
				})
			}
		}
		result.Summary.GeneratedUsernames = len(result.Mapping)
		if audit != nil {
			return audit(tx, result.Summary)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) loadExisting(ctx context.Context, rows []*row) (map[string]struct{}, map[string]struct{}, error) {
	names := make([]string, 0, len(rows))
	emails := make([]string, 0, len(rows))
	for _, item := range rows {
		if item.status != rowValid {
			continue
		}
		normalized := user.Normalize(item.uname)
		if user.ValidateUsername(normalized) == nil {
			names = append(names, normalized)
		}
		if item.email != "" {
			emails = append(emails, item.email)
		}
	}

	existingNames := make(map[string]struct{}, len(names))
	if err := s.loadNames(ctx, names, existingNames); err != nil {
		return nil, nil, err
	}
	if err := s.loadHydroNames(ctx, existingNames); err != nil {
		return nil, nil, err
	}
	existingEmails := make(map[string]struct{}, len(emails))
	if err := s.loadEmails(ctx, emails, existingEmails); err != nil {
		return nil, nil, err
	}
	return existingNames, existingEmails, nil
}

func (s *Service) loadNames(ctx context.Context, names []string, out map[string]struct{}) error {
	for start := 0; start < len(names); start += queryChunk {
		end := start + queryChunk
		if end > len(names) {
			end = len(names)
		}
		var found []user.User
		if err := s.db.WithContext(ctx).
			Select("username").
			Where("LOWER(username) IN ?", names[start:end]).
			Find(&found).Error; err != nil {
			return err
		}
		for _, item := range found {
			out[strings.ToLower(item.Username)] = struct{}{}
		}
	}
	return nil
}

func (s *Service) loadHydroNames(ctx context.Context, out map[string]struct{}) error {
	var found []user.User
	if err := s.db.WithContext(ctx).
		Select("username").
		Where("LOWER(username) LIKE ?", "hydro%").
		Find(&found).Error; err != nil {
		return err
	}
	for _, item := range found {
		out[strings.ToLower(item.Username)] = struct{}{}
	}
	return nil
}

func (s *Service) loadEmails(ctx context.Context, emails []string, out map[string]struct{}) error {
	for start := 0; start < len(emails); start += queryChunk {
		end := start + queryChunk
		if end > len(emails) {
			end = len(emails)
		}
		var found []user.User
		if err := s.db.WithContext(ctx).
			Select("email").
			Where("LOWER(email) IN ?", emails[start:end]).
			Find(&found).Error; err != nil {
			return err
		}
		for _, item := range found {
			out[strings.ToLower(item.Email)] = struct{}{}
		}
	}
	return nil
}

func markConflicts(rows []*row, existingNames, existingEmails map[string]struct{}) {
	for _, item := range rows {
		if item.status != rowValid {
			continue
		}
		_, nameExists := existingNames[item.username]
		_, emailExists := existingEmails[item.email]
		switch {
		case nameExists && emailExists:
			item.status = rowConflict
			item.reason = "用户名和邮箱已存在"
		case nameExists:
			item.status = rowConflict
			item.reason = "用户名已存在"
		case emailExists:
			item.status = rowConflict
			item.reason = "邮箱已存在"
		}
	}
}

func buildResult(rows []*row, dryRun bool) *Result {
	result := &Result{
		DryRun:  dryRun,
		Issues:  make([]Issue, 0),
		Mapping: make([]Mapping, 0),
		Samples: make([]Sample, 0),
	}
	result.Summary.Total = len(rows)

	for _, item := range rows {
		switch item.status {
		case rowValid:
			result.Summary.Valid++
			if item.generated {
				result.Mapping = append(result.Mapping, Mapping{
					LegacyID: item.legacyID,
					Uname:    item.uname,
					Email:    item.email,
					Username: item.username,
				})
			}
			if len(result.Samples) < sampleLimit {
				result.Samples = append(result.Samples, Sample{
					Line:      item.line,
					LegacyID:  item.legacyID,
					Uname:     item.uname,
					Email:     item.email,
					Username:  item.username,
					Generated: item.generated,
				})
			}
		case rowConflict:
			result.Summary.Conflicts++
			appendIssue(result, Issue{
				Line: item.line, LegacyID: item.legacyID, Uname: item.uname,
				Email: item.email, Status: string(rowConflict), Reason: item.reason,
			})
		case rowInvalid:
			result.Summary.Invalid++
			appendIssue(result, Issue{
				Line: item.line, LegacyID: item.legacyID, Uname: item.uname,
				Email: item.email, Status: string(rowInvalid), Reason: item.reason,
			})
		}
	}
	result.Summary.GeneratedUsernames = len(result.Mapping)
	return result
}

func appendIssue(result *Result, issue Issue) {
	if len(result.Issues) >= issueLimit {
		result.IssuesTruncated = true
		return
	}
	result.Issues = append(result.Issues, issue)
}

func cloneSet(source map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(source))
	for key := range source {
		out[key] = struct{}{}
	}
	return out
}
