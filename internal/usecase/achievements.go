package usecase

import (
	"context"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// AchievementsRepository stores the badge catalogue and the badges users
// earned, and computes the metrics the badge thresholds apply to.
type AchievementsRepository interface {
	GetBadges(ctx context.Context, lang models.Lang) ([]models.Badge, error)
	GetUserBadges(ctx context.Context, userId int, lang models.Lang) ([]models.UserBadge, error)
	// AddUserBadges awards the badges and returns the codes that were new.
	AddUserBadges(ctx context.Context, userId int, codes []string) ([]string, error)
	GetAchievementMetrics(ctx context.Context, userId int) (models.AchievementMetrics, error)
}

// AchievementsUsersRepository is the part of UsersRepository the profile
// needs.
type AchievementsUsersRepository interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUserStats(ctx context.Context, userId int) (models.UserStats, error)
}

type AchievementsRepositories struct {
	Achievements AchievementsRepository
	Users        AchievementsUsersRepository
}

// Achievements awards badges, and serves the badge catalogue and the
// gamification profile.
type Achievements struct {
	log    *slog.Logger
	repos  AchievementsRepositories
	events events.Publisher
}

func NewAchievements(log *slog.Logger, repos AchievementsRepositories) *Achievements {
	return &Achievements{
		log:    log,
		repos:  repos,
		events: events.NoopPublisher{},
	}
}

// WithEvents sets the publisher of badge.earned events. Without it events
// are dropped.
func (uc *Achievements) WithEvents(p events.Publisher) *Achievements {
	if p != nil {
		uc.events = p
	}
	return uc
}

// ListBadges returns the catalogue localised for the language of ctx.
func (uc *Achievements) ListBadges(ctx context.Context) ([]models.Badge, error) {
	const op = "usecase.Achievements.ListBadges"

	badges, err := uc.repos.Achievements.GetBadges(ctx, models.LangFromContext(ctx))
	if err != nil {
		return nil, mapRepoErr(op, err)
	}

	return badges, nil
}

// GetProfile returns the public gamification profile of the user: level,
// badges (localised for the language of ctx) and activity counters.
func (uc *Achievements) GetProfile(ctx context.Context, userId int) (models.UserProfile, error) {
	const op = "usecase.Achievements.GetProfile"

	lang := models.LangFromContext(ctx)

	user, err := uc.repos.Users.GetUserById(ctx, userId)
	if err != nil {
		return models.UserProfile{}, mapRepoErr(op, err)
	}

	stats, err := uc.repos.Users.GetUserStats(ctx, userId)
	if err != nil {
		return models.UserProfile{}, mapRepoErr(op, err)
	}

	badges, err := uc.repos.Achievements.GetUserBadges(ctx, userId, lang)
	if err != nil {
		return models.UserProfile{}, mapRepoErr(op, err)
	}

	return models.UserProfile{
		User:        user.Public(),
		Level:       models.LevelFor(user.Rating, lang),
		Badges:      badges,
		Stats:       stats,
		MemberSince: user.CreatedAt,
	}, nil
}

// Evaluate recomputes the user's metrics, awards every catalogue badge the
// metrics satisfy that the user does not have yet and returns the newly
// earned badges (localised for the language of ctx). A badge.earned event
// is published per new badge. The call is idempotent: a second evaluation
// with the same metrics awards nothing and returns an empty list.
func (uc *Achievements) Evaluate(ctx context.Context, userId int) ([]models.Badge, error) {
	const op = "usecase.Achievements.Evaluate"

	metrics, err := uc.repos.Achievements.GetAchievementMetrics(ctx, userId)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}

	catalogue, err := uc.repos.Achievements.GetBadges(ctx, models.LangFromContext(ctx))
	if err != nil {
		return nil, mapRepoErr(op, err)
	}

	earned := models.EarnedBadges(catalogue, metrics)
	if len(earned) == 0 {
		return []models.Badge{}, nil
	}

	codes := make([]string, 0, len(earned))
	byCode := make(map[string]models.Badge, len(earned))
	for _, b := range earned {
		codes = append(codes, b.Code)
		byCode[b.Code] = b
	}

	added, err := uc.repos.Achievements.AddUserBadges(ctx, userId, codes)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}

	// Returned in catalogue order regardless of the order of the insert.
	newBadges := make([]models.Badge, 0, len(added))
	addedSet := make(map[string]struct{}, len(added))
	for _, code := range added {
		addedSet[code] = struct{}{}
	}
	for _, b := range earned {
		if _, ok := addedSet[b.Code]; ok {
			newBadges = append(newBadges, b)
		}
	}

	for _, b := range newBadges {
		uc.log.Info("badge earned", slog.String("op", op), slog.Int("user_id", userId), slog.String("badge", b.Code))
		events.PublishEvent(ctx, uc.log, uc.events, events.NewBadgeEarned(userId, b.Code))
	}

	return newBadges, nil
}
