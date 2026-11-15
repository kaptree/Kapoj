// Package storageaudit detects database rows that point at uploaded files
// which are no longer present in StorageDir. Database metadata and blobs have
// separate lifecycles: a missing or recreated storage volume can leave valid
// rows with dangling image/file references. Auditing at startup turns silent
// 404s into an actionable warning instead of hiding the problem.
package storageaudit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

// Reference is one stored object referenced by a database row.
type Reference struct {
	Kind string
	ID   int64
	Key  string
}

// Missing describes a reference whose file is not present in storage.
type Missing struct {
	Kind string
	ID   int64
	Key  string
}

// Scan collects all user-facing media references and checks that each blob
// exists. It does not modify the database; callers decide whether a missing
// file should be logged, repaired, or treated as a startup failure.
func Scan(ctx context.Context, db *gorm.DB, store storage.ObjectStore) ([]Missing, error) {
	refs, err := Collect(ctx, db)
	if err != nil {
		return nil, err
	}
	return MissingReferences(ctx, store, refs)
}

// Collect returns all database references to business objects: user-facing
// media plus problem testcases, checkers, attachments, and generated cases.
func Collect(ctx context.Context, db *gorm.DB) ([]Reference, error) {
	refs := make([]Reference, 0)

	var badges []struct {
		ID       int64
		ImageURL string
	}
	if err := db.WithContext(ctx).Table("badges").Select("id, image_url").Scan(&badges).Error; err != nil {
		return nil, fmt.Errorf("audit badges: %w", err)
	}
	for _, badge := range badges {
		if key, ok := BadgeKey(badge.ImageURL); ok {
			refs = append(refs, Reference{Kind: "badge", ID: badge.ID, Key: key})
		}
	}

	var users []struct {
		ID        int64
		AvatarKey string
	}
	if err := db.WithContext(ctx).Table("users").
		Select("id, avatar_key").Where("avatar_key <> ''").Scan(&users).Error; err != nil {
		return nil, fmt.Errorf("audit avatars: %w", err)
	}
	for _, user := range users {
		refs = append(refs, Reference{Kind: "avatar", ID: user.ID, Key: user.AvatarKey})
	}

	var certificates []struct {
		ID       int64
		ImageKey string
	}
	if err := db.WithContext(ctx).Table("certificates").
		Select("id, image_key").Where("image_key <> ''").Scan(&certificates).Error; err != nil {
		return nil, fmt.Errorf("audit certificates: %w", err)
	}
	for _, certificate := range certificates {
		refs = append(refs, Reference{
			Kind: "certificate", ID: certificate.ID, Key: certificate.ImageKey,
		})
	}

	var settings []struct {
		ID         int64
		LogoKey    string
		FaviconKey string
	}
	if err := db.WithContext(ctx).Table("site_settings").
		Select("id, logo_key, favicon_key").Scan(&settings).Error; err != nil {
		return nil, fmt.Errorf("audit site assets: %w", err)
	}
	for _, row := range settings {
		if row.LogoKey != "" {
			refs = append(refs, Reference{Kind: "site-logo", ID: row.ID, Key: row.LogoKey})
		}
		if row.FaviconKey != "" {
			refs = append(refs, Reference{Kind: "site-favicon", ID: row.ID, Key: row.FaviconKey})
		}
	}

	var checkers []struct {
		ID          int64
		CheckerPath string
	}
	if err := db.WithContext(ctx).Table("problems").
		Select("id, checker_path").Where("checker_path <> ''").Scan(&checkers).Error; err != nil {
		return nil, fmt.Errorf("audit problem checkers: %w", err)
	}
	for _, row := range checkers {
		refs = append(refs, Reference{Kind: "problem-checker", ID: row.ID, Key: row.CheckerPath})
	}

	var attachments []struct {
		ID         int64
		StorageKey string
	}
	if err := db.WithContext(ctx).Table("problem_attachments").
		Select("id, storage_key").Where("storage_key <> ''").Scan(&attachments).Error; err != nil {
		return nil, fmt.Errorf("audit problem attachments: %w", err)
	}
	for _, row := range attachments {
		refs = append(refs, Reference{Kind: "problem-attachment", ID: row.ID, Key: row.StorageKey})
	}

	var testcases []struct {
		ID         int64
		InputFile  string
		OutputFile string
	}
	if err := db.WithContext(ctx).Table("problem_testcases").
		Select("id, input_file, output_file").Scan(&testcases).Error; err != nil {
		return nil, fmt.Errorf("audit problem testcases: %w", err)
	}
	for _, row := range testcases {
		if row.InputFile != "" {
			refs = append(refs, Reference{Kind: "problem-testcase-input", ID: row.ID, Key: row.InputFile})
		}
		if row.OutputFile != "" {
			refs = append(refs, Reference{Kind: "problem-testcase-output", ID: row.ID, Key: row.OutputFile})
		}
	}

	var generated []struct {
		ID         int64
		InputFile  string
		OutputFile string
	}
	if err := db.WithContext(ctx).Table("testcase_generation_cases").
		Select("id, input_file, output_file").Scan(&generated).Error; err != nil {
		return nil, fmt.Errorf("audit generated testcases: %w", err)
	}
	for _, row := range generated {
		if row.InputFile != "" {
			refs = append(refs, Reference{Kind: "generated-input", ID: row.ID, Key: row.InputFile})
		}
		if row.OutputFile != "" {
			refs = append(refs, Reference{Kind: "generated-output", ID: row.ID, Key: row.OutputFile})
		}
	}
	return refs, nil
}

// MissingReferences lists the store once and checks each unique reference
// against that set. Errors abort the audit so a broken store is not
// misreported as hundreds of missing files.
func MissingReferences(
	ctx context.Context,
	store storage.ObjectStore,
	refs []Reference,
) ([]Missing, error) {
	if store == nil {
		return nil, errors.New("storage audit: nil storage")
	}
	objects, err := store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		existing[object.Key] = struct{}{}
	}
	missing := make([]Missing, 0)
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if ref.Key == "" {
			continue
		}
		if _, ok := seen[ref.Key]; ok {
			continue
		}
		seen[ref.Key] = struct{}{}
		if _, ok := existing[ref.Key]; ok {
			continue
		}
		missing = append(missing, Missing{Kind: ref.Kind, ID: ref.ID, Key: ref.Key})
	}
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].Kind != missing[j].Kind {
			return missing[i].Kind < missing[j].Kind
		}
		if missing[i].ID != missing[j].ID {
			return missing[i].ID < missing[j].ID
		}
		return missing[i].Key < missing[j].Key
	})
	return missing, nil
}

// BadgeKey converts the public internal badge URL into its storage key.
// External URLs and malformed values return false.
func BadgeKey(imageURL string) (string, bool) {
	const prefix = "/api/badges/"
	const suffix = "/image"
	if !strings.HasPrefix(imageURL, prefix) || !strings.HasSuffix(imageURL, suffix) {
		return "", false
	}
	rawID := strings.TrimSuffix(strings.TrimPrefix(imageURL, prefix), suffix)
	if rawID == "" || strings.ContainsAny(rawID, `/\`) {
		return "", false
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != rawID {
		return "", false
	}
	return "badges/" + rawID, true
}
