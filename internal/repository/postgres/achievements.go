package postgres

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// AchievementsRepository stores the badge catalogue and the badges users
// earned.
type AchievementsRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewAchievements(db *sqlx.DB, c *trmsqlx.CtxGetter) *AchievementsRepository {
	return &AchievementsRepository{db: db, getter: c}
}

// badgeColumns selects a badge localised for $1 (models.Lang); every
// language other than "en" gets the Russian texts.
const badgeColumns = `
	code,
	CASE WHEN $1 = 'en' THEN name_en ELSE name_ru END AS name,
	CASE WHEN $1 = 'en' THEN description_en ELSE description_ru END AS description,
	icon, threshold, metric`

// GetBadges returns the catalogue in a stable order (by metric, then by
// threshold).
func (r *AchievementsRepository) GetBadges(ctx context.Context, lang models.Lang) ([]models.Badge, error) {
	const op = "storage.postgres.GetBadges"

	badges := []models.Badge{}
	query := "SELECT " + badgeColumns + " FROM badges ORDER BY metric, threshold, code"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &badges, query, string(lang)); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return badges, nil
}

// GetUserBadges returns the badges the user earned, oldest first.
func (r *AchievementsRepository) GetUserBadges(ctx context.Context, userId int, lang models.Lang) ([]models.UserBadge, error) {
	const op = "storage.postgres.GetUserBadges"

	badges := []models.UserBadge{}
	query := "SELECT " + badgeColumns + `, ub.earned_at
		FROM user_badges ub JOIN badges b ON b.code = ub.badge_code
		WHERE ub.user_id = $2
		ORDER BY ub.earned_at ASC, b.code ASC`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &badges, query, string(lang), userId); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return badges, nil
}

// AddUserBadges awards the badges to the user and returns the codes that
// were actually new; badges the user already has are skipped (INSERT ...
// ON CONFLICT DO NOTHING), so the call is idempotent. An unknown user or
// badge code is repository.ErrInvalidReference.
func (r *AchievementsRepository) AddUserBadges(ctx context.Context, userId int, codes []string) ([]string, error) {
	const op = "storage.postgres.AddUserBadges"

	if len(codes) == 0 {
		return []string{}, nil
	}

	added := []string{}
	query := `
		INSERT INTO user_badges (user_id, badge_code)
		SELECT $1, code FROM unnest($2::text[]) AS code
		ON CONFLICT (user_id, badge_code) DO NOTHING
		RETURNING badge_code`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &added, query, userId, pq.Array(codes)); err != nil {
		return nil, wrapPgError(op, err)
	}

	return added, nil
}

// GetAchievementMetrics computes the counters the badge thresholds apply
// to. A mark counts as confirmed once it passed the first vote (same rule
// as GetUserStats); correct checks come from the rating event log; the
// check streak is the longest run of consecutive UTC days with a check.
func (r *AchievementsRepository) GetAchievementMetrics(ctx context.Context, userId int) (models.AchievementMetrics, error) {
	const op = "storage.postgres.GetAchievementMetrics"

	var m models.AchievementMetrics
	query := `
		WITH check_days AS (
			SELECT DISTINCT (created_at AT TIME ZONE 'UTC')::date AS d FROM checks WHERE user_id = $1
		), runs AS (
			SELECT d - (ROW_NUMBER() OVER (ORDER BY d))::int AS run FROM check_days
		)
		SELECT
			(SELECT COUNT(*) FROM marks m WHERE m.user_id = $1) AS marks_total,
			(SELECT COUNT(*) FROM marks m WHERE m.user_id = $1 AND m.mark_status_id IN ($2, $3, $4, $5)) AS marks_confirmed,
			(SELECT COUNT(*) FROM rating_events e WHERE e.user_id = $1 AND e.reason = $6) AS checks_correct,
			(SELECT COALESCE(MAX(n), 0) FROM (SELECT COUNT(*) AS n FROM runs GROUP BY run) s) AS check_streak_days,
			(SELECT COUNT(*) FROM tasks t WHERE t.user_id = $1 AND t.status_id = $7) AS tasks_completed,
			(SELECT COUNT(*) FROM marks m WHERE m.user_id = $1 AND m.mark_status_id = $5) AS marks_closed`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &m, query, userId,
		models.ConfirmedStatus, models.UnderReviewStatus, models.RediscoveredStatus, models.ClosedStatus,
		models.RatingReasonCheckCorrect, models.CompletedStatus,
	); err != nil {
		return m, fmt.Errorf("%s: %w", op, err)
	}

	return m, nil
}
