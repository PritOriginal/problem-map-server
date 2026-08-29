package models

import (
	"errors"
	"fmt"
	"time"
)

// levelThresholds are the ratings at which each level starts; index i is
// level i+1.
var levelThresholds = []int{0, 20, 50, 100, 200, 400, 800}

// levelNames are the level names per language, index i is level i+1.
var levelNames = map[Lang][]string{
	LangRU: {"Новичок", "Наблюдатель", "Активист", "Эксперт", "Мастер", "Герой", "Легенда"},
	LangEN: {"Novice", "Observer", "Activist", "Expert", "Master", "Hero", "Legend"},
}

// MaxLevel is the highest level a user can reach.
var MaxLevel = len(levelThresholds)

// Level is the user's level derived from the rating.
type Level struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	// NextThreshold is the rating that starts the next level; nil at the
	// last level.
	NextThreshold *int `json:"next_threshold"`
}

// LevelFor returns the level of a rating: level 1 covers everything below
// the second threshold (negative ratings included), the last level has no
// next threshold. The name is localised; an unsupported lang falls back
// to DefaultLang.
func LevelFor(rating int, lang Lang) Level {
	number := 1
	for i := 1; i < len(levelThresholds); i++ {
		if rating >= levelThresholds[i] {
			number = i + 1
		}
	}

	names, ok := levelNames[lang]
	if !ok {
		names = levelNames[DefaultLang]
	}
	level := Level{Number: number, Name: names[number-1]}
	if number < len(levelThresholds) {
		next := levelThresholds[number]
		level.NextThreshold = &next
	}
	return level
}

// BadgeMetric names the user metric a badge threshold applies to
// (badges.metric).
type BadgeMetric string

const (
	MetricMarksTotal      BadgeMetric = "marks_total"
	MetricMarksConfirmed  BadgeMetric = "marks_confirmed"
	MetricChecksCorrect   BadgeMetric = "checks_correct"
	MetricCheckStreakDays BadgeMetric = "check_streak_days"
	MetricTasksCompleted  BadgeMetric = "tasks_completed"
	MetricMarksClosed     BadgeMetric = "marks_closed"
)

// AchievementMetrics are the counters the badge thresholds are compared
// with (see AchievementsRepository.GetAchievementMetrics).
type AchievementMetrics struct {
	MarksTotal     int `db:"marks_total"`
	MarksConfirmed int `db:"marks_confirmed"`
	ChecksCorrect  int `db:"checks_correct"`
	// CheckStreakDays is the longest run of consecutive days (UTC) on which
	// the user submitted a check.
	CheckStreakDays int `db:"check_streak_days"`
	TasksCompleted  int `db:"tasks_completed"`
	// MarksClosed counts the user's marks in the Closed status.
	MarksClosed int `db:"marks_closed"`
}

// Value returns the counter of the metric; an unknown metric yields
// ok=false so that a catalogue row with a typo never awards a badge.
func (m AchievementMetrics) Value(metric BadgeMetric) (int, bool) {
	switch metric {
	case MetricMarksTotal:
		return m.MarksTotal, true
	case MetricMarksConfirmed:
		return m.MarksConfirmed, true
	case MetricChecksCorrect:
		return m.ChecksCorrect, true
	case MetricCheckStreakDays:
		return m.CheckStreakDays, true
	case MetricTasksCompleted:
		return m.TasksCompleted, true
	case MetricMarksClosed:
		return m.MarksClosed, true
	default:
		return 0, false
	}
}

// Badge is a catalogue entry; Name and Description are localised by the
// repository for the requested language.
type Badge struct {
	Code        string      `json:"code" db:"code"`
	Name        string      `json:"name" db:"name"`
	Description string      `json:"description" db:"description"`
	Icon        string      `json:"icon" db:"icon"`
	Threshold   int         `json:"threshold" db:"threshold"`
	Metric      BadgeMetric `json:"metric" db:"metric"`
}

// Achieved reports whether the metrics satisfy the badge threshold.
func (b Badge) Achieved(m AchievementMetrics) bool {
	v, ok := m.Value(b.Metric)
	return ok && v >= b.Threshold
}

// EarnedBadges returns the catalogue badges the metrics satisfy, in
// catalogue order.
func EarnedBadges(catalogue []Badge, m AchievementMetrics) []Badge {
	earned := make([]Badge, 0, len(catalogue))
	for _, b := range catalogue {
		if b.Achieved(m) {
			earned = append(earned, b)
		}
	}
	return earned
}

// UserBadge is a badge a user earned.
type UserBadge struct {
	Badge
	EarnedAt time.Time `json:"earned_at" db:"earned_at"`
}

// UserProfile is the public gamification profile of a user.
type UserProfile struct {
	User        User
	Level       Level
	Badges      []UserBadge
	Stats       UserStats
	MemberSince time.Time
}

// ErrInvalidPeriod is returned for an unknown leaderboard period (wrapped
// into usecase.ErrInvalidArgument).
var ErrInvalidPeriod = errors.New("invalid period")

// LeaderboardPeriod restricts the leaderboard to the rating events of a
// rolling window.
type LeaderboardPeriod string

const (
	LeaderboardAll   LeaderboardPeriod = "all"
	LeaderboardMonth LeaderboardPeriod = "month"
	LeaderboardWeek  LeaderboardPeriod = "week"
)

// Window returns the length of the period; zero for LeaderboardAll.
func (p LeaderboardPeriod) Window() time.Duration {
	switch p {
	case LeaderboardMonth:
		return 30 * 24 * time.Hour
	case LeaderboardWeek:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

// Validate reports an error for an unknown period; the empty period means
// LeaderboardAll.
func (p LeaderboardPeriod) Validate() error {
	switch p {
	case "", LeaderboardAll, LeaderboardMonth, LeaderboardWeek:
		return nil
	default:
		return fmt.Errorf("%w: unknown period %q", ErrInvalidPeriod, p)
	}
}

// LeaderboardFilters select which rating events make up the leaderboard.
// Without filters the leaderboard is users.rating; with any filter the
// rating is the sum of the matching rating events.
type LeaderboardFilters struct {
	// BoundaryID keeps the events whose mark lies inside the admin
	// boundary; 0 means no boundary.
	BoundaryID int
	Period     LeaderboardPeriod
}

// Any reports whether the filters restrict the events.
func (f LeaderboardFilters) Any() bool {
	return f.BoundaryID != 0 || f.Period.Window() > 0
}

// LeaderboardEntry is one row of the leaderboard.
type LeaderboardEntry struct {
	UserID      int    `db:"user_id"`
	Name        string `db:"name"`
	Rating      int    `db:"rating"`
	BadgesCount int    `db:"badges_count"`
}
