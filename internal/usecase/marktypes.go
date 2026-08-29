package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"unicode/utf8"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

// Limits of the mark type attributes.
const (
	MaxMarkTypeNameLen = 40 // types_marks.name is VARCHAR(40)
	MaxMarkTypeIconLen = 64
	MaxSLAHours        = 24 * 365
	MaxMarkTypeSort    = 10_000
)

var (
	markTypeCodeRe  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	markTypeColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// MarkTypesRepository manages the mark type dictionary.
type MarkTypesRepository interface {
	GetAllMarkTypes(ctx context.Context, lang models.Lang) ([]models.MarkType, error)
	GetMarkTypeById(ctx context.Context, id int, lang models.Lang) (models.MarkType, error)
	AddMarkType(ctx context.Context, t models.MarkTypeCreate) (int64, error)
	UpdateMarkType(ctx context.Context, id int, upd models.MarkTypeUpdate) error
}

// DictionaryCache drops cached HTTP responses of a dictionary by key
// prefix (see middleware/cache.Key); it is optional and fails open.
type DictionaryCache interface {
	DeleteByPrefix(ctx context.Context, prefix string) error
}

// MarkTypes is the admin service of the mark type dictionary. Every change
// invalidates the cached public dictionary (GET /marks/types).
type MarkTypes struct {
	log       *slog.Logger
	trManager trm.Manager
	repo      MarkTypesRepository
	cache     DictionaryCache
	prefixes  []string
}

func NewMarkTypes(log *slog.Logger, trManager trm.Manager, repo MarkTypesRepository) *MarkTypes {
	return &MarkTypes{log: log, trManager: trManager, repo: repo}
}

// WithCache sets the cache to invalidate after a change and the key prefixes
// of the cached dictionary responses. Without it nothing is invalidated.
func (uc *MarkTypes) WithCache(c DictionaryCache, prefixes ...string) *MarkTypes {
	if c != nil {
		uc.cache = c
		uc.prefixes = prefixes
	}
	return uc
}

// List returns every mark type, inactive ones included, sorted by
// sort_order and the localised name.
func (uc *MarkTypes) List(ctx context.Context, lang models.Lang) ([]models.MarkType, error) {
	const op = "usecase.MarkTypes.List"

	types, err := uc.repo.GetAllMarkTypes(ctx, lang)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return types, nil
}

// Create adds a mark type (ErrConflict when the code is taken) and returns
// it with the name in lang.
func (uc *MarkTypes) Create(ctx context.Context, t models.MarkTypeCreate, lang models.Lang) (models.MarkType, error) {
	const op = "usecase.MarkTypes.Create"

	if err := validateMarkTypeCreate(t); err != nil {
		return models.MarkType{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	var id int64
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		var err error
		id, err = uc.repo.AddMarkType(ctx, t)
		return err
	})
	if err != nil {
		return models.MarkType{}, mapRepoErr(op, err)
	}

	uc.invalidate(ctx, op)

	created, err := uc.repo.GetMarkTypeById(ctx, int(id), lang)
	if err != nil {
		return models.MarkType{}, mapRepoErr(op, err)
	}
	return created, nil
}

// Update applies the non-nil fields of upd to the mark type (ErrNotFound,
// ErrConflict on a taken code) and returns it with the name in lang.
func (uc *MarkTypes) Update(ctx context.Context, id int, upd models.MarkTypeUpdate, lang models.Lang) (models.MarkType, error) {
	const op = "usecase.MarkTypes.Update"

	if upd.IsEmpty() {
		return models.MarkType{}, fmt.Errorf("%s: %w: nothing to update", op, ErrInvalidArgument)
	}
	if err := validateMarkTypeUpdate(upd); err != nil {
		return models.MarkType{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		return uc.repo.UpdateMarkType(ctx, id, upd)
	})
	if err != nil {
		return models.MarkType{}, mapRepoErr(op, err)
	}

	uc.invalidate(ctx, op)

	updated, err := uc.repo.GetMarkTypeById(ctx, id, lang)
	if err != nil {
		return models.MarkType{}, mapRepoErr(op, err)
	}
	return updated, nil
}

// invalidate drops the cached dictionary responses; a failure is logged
// because the cache entries expire on their own anyway.
func (uc *MarkTypes) invalidate(ctx context.Context, op string) {
	if uc.cache == nil {
		return
	}
	for _, prefix := range uc.prefixes {
		if err := uc.cache.DeleteByPrefix(ctx, prefix); err != nil {
			uc.log.Warn("failed to invalidate dictionary cache", slog.String("op", op), slog.String("prefix", prefix), logger.Err(err))
		}
	}
}

func validateMarkTypeCreate(t models.MarkTypeCreate) error {
	var errs []error
	if !markTypeCodeRe.MatchString(t.Code) {
		errs = append(errs, errors.New("code must match ^[a-z][a-z0-9_]{0,39}$"))
	}
	errs = append(errs, validateMarkTypeName("name_ru", t.NameRU, true))
	if t.NameEN != "" {
		errs = append(errs, validateMarkTypeName("name_en", t.NameEN, false))
	}
	if t.Icon.Valid {
		errs = append(errs, validateMarkTypeIcon(t.Icon.String))
	}
	if t.Color.Valid {
		errs = append(errs, validateMarkTypeColor(t.Color.String))
	}
	errs = append(errs, validateSLAHours(t.SLAHours))
	return errors.Join(errs...)
}

func validateMarkTypeUpdate(upd models.MarkTypeUpdate) error {
	var errs []error
	if upd.Code != nil && !markTypeCodeRe.MatchString(*upd.Code) {
		errs = append(errs, errors.New("code must match ^[a-z][a-z0-9_]{0,39}$"))
	}
	if upd.NameRU != nil {
		errs = append(errs, validateMarkTypeName("name_ru", *upd.NameRU, true))
	}
	if upd.NameEN != nil {
		errs = append(errs, validateMarkTypeName("name_en", *upd.NameEN, true))
	}
	// An empty icon/color clears the attribute.
	if upd.Icon != nil && *upd.Icon != "" {
		errs = append(errs, validateMarkTypeIcon(*upd.Icon))
	}
	if upd.Color != nil && *upd.Color != "" {
		errs = append(errs, validateMarkTypeColor(*upd.Color))
	}
	if upd.SLAHours != nil {
		errs = append(errs, validateSLAHours(*upd.SLAHours))
	}
	if upd.SortOrder != nil && (*upd.SortOrder < 0 || *upd.SortOrder > MaxMarkTypeSort) {
		errs = append(errs, fmt.Errorf("sort_order must be in [0, %d]", MaxMarkTypeSort))
	}
	return errors.Join(errs...)
}

func validateMarkTypeName(field, name string, required bool) error {
	n := utf8.RuneCountInString(name)
	if (required && n == 0) || n > MaxMarkTypeNameLen {
		return fmt.Errorf("%s must be 1..%d characters", field, MaxMarkTypeNameLen)
	}
	return nil
}

func validateMarkTypeIcon(icon string) error {
	if n := utf8.RuneCountInString(icon); n == 0 || n > MaxMarkTypeIconLen {
		return fmt.Errorf("icon must be 1..%d characters", MaxMarkTypeIconLen)
	}
	return nil
}

func validateMarkTypeColor(color string) error {
	if !markTypeColorRe.MatchString(color) {
		return errors.New("color must be a hex colour like #ff8800")
	}
	return nil
}

func validateSLAHours(h int) error {
	if h < 1 || h > MaxSLAHours {
		return fmt.Errorf("sla_hours must be in [1, %d]", MaxSLAHours)
	}
	return nil
}
