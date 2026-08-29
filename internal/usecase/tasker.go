package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/guregu/null/v6"
)

// TaskerTasksRepository extends TasksRepository with the bulk operations
// needed by the scheduled tasker job.
type TaskerTasksRepository interface {
	TasksRepository
	// ExpireOverdueTasks moves issued tasks with due_at < now to
	// models.OverdueStatus and returns how many rows were affected.
	ExpireOverdueTasks(ctx context.Context, now time.Time) (int64, error)
}

// TaskerStats summarises a single Tasker.Update run.
type TaskerStats struct {
	// Marks is the number of marks that needed verification.
	Marks int
	// Users is the number of registered users considered.
	Users int
	// Candidates is the number of (user, mark) pairs within the radius that
	// were free for assignment at the start of the run.
	Candidates int
	// Assigned is the number of tasks created by this run.
	Assigned int
	// Covered is the number of marks whose coverage probability reached the
	// target after the run.
	Covered int
	// Iterations is the number of assignment rounds performed.
	Iterations int
}

// Tasker distributes verification tasks for unconfirmed marks between users.
type Tasker struct {
	log       *slog.Logger
	cfg       config.TaskerConfig
	trManager trm.Manager
	repos     TaskerRepositories
	now       func() time.Time
}

type TaskerRepositories struct {
	Tasks TaskerTasksRepository
	Marks MarksRepository
	Users UsersRepository
}

func NewTasker(log *slog.Logger, cfg config.TaskerConfig, trManager trm.Manager, repos TaskerRepositories) *Tasker {
	return &Tasker{
		log:       log,
		cfg:       cfg,
		trManager: trManager,
		repos:     repos,
		now:       time.Now,
	}
}

// marksToVerify lists mark statuses for which verification tasks are issued.
var marksToVerify = []models.MarkStatusType{
	models.UnconfirmedStatus,
	models.UnderReviewStatus,
}

// ExpireOverdue marks every issued task whose deadline has passed as
// overdue and returns the number of such tasks.
func (uc *Tasker) ExpireOverdue(ctx context.Context) (int64, error) {
	const op = "usecase.Tasker.ExpireOverdue"

	n, err := uc.repos.Tasks.ExpireOverdueTasks(ctx, uc.now())
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	uc.log.Info("overdue tasks expired", slog.String("op", op), slog.Int64("expired", n))

	return n, nil
}

// Update assigns verification tasks to users for marks that are not yet
// sufficiently covered and writes them in a single transaction.
func (uc *Tasker) Update(ctx context.Context) (TaskerStats, error) {
	const op = "usecase.Tasker.Update"

	log := uc.log.With(slog.String("op", op))
	started := uc.now()

	markStatusIds := make([]int, 0, len(marksToVerify))
	for _, s := range marksToVerify {
		markStatusIds = append(markStatusIds, int(s))
	}

	marks, err := uc.repos.Marks.GetMarks(ctx, models.GetMarksFilters{MarkStatusIds: markStatusIds})
	if err != nil {
		return TaskerStats{}, fmt.Errorf("%s: %w", op, err)
	}

	users, err := uc.repos.Users.GetUsers(ctx, models.Pagination{})
	if err != nil {
		return TaskerStats{}, fmt.Errorf("%s: %w", op, err)
	}

	// Tasks in every status are needed: issued ones count towards load and
	// coverage, overdue ones towards fatigue, and any existing task excludes
	// the (user, mark) pair from re-assignment.
	tasks, err := uc.repos.Tasks.GetTasks(ctx, models.GetTasksFilters{
		Statuses: []int{
			int(models.UnfulfilledStatus),
			int(models.CompletedStatus),
			int(models.OverdueStatus),
		},
	})
	if err != nil {
		return TaskerStats{}, fmt.Errorf("%s: %w", op, err)
	}

	distances, err := uc.repos.Marks.GetDistancesFromMarkToPoint(ctx, models.GetDistanceFromMarkToPointFilters{
		MarkStatusIds: marksToVerify,
		MaxRadius:     uc.cfg.MaxRadiusMeters,
	})
	if err != nil {
		return TaskerStats{}, fmt.Errorf("%s: %w", op, err)
	}

	assignments, stats := uc.plan(marks.Items, users.Items, tasks.Items, distances)

	dueAt := null.TimeFrom(uc.now().Add(uc.cfg.TaskTTL))

	// All assignments are written in one transaction so that a failure or a
	// cancellation (SIGTERM) never leaves a partially written batch.
	err = uc.trManager.Do(ctx, func(ctx context.Context) error {
		for _, a := range assignments {
			if _, err := uc.repos.Tasks.AddTask(ctx, models.Task{
				MarkID: a.markId,
				UserID: a.userId,
				DueAt:  dueAt,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return TaskerStats{}, fmt.Errorf("%s: %w", op, err)
	}

	// The pass summary is logged by cmd/tasker; this is a trace of the run.
	log.Debug("tasks assigned",
		slog.Int("marks", stats.Marks),
		slog.Int("users", stats.Users),
		slog.Int("candidates", stats.Candidates),
		slog.Int("assigned", stats.Assigned),
		slog.Int("covered", stats.Covered),
		slog.Int("iterations", stats.Iterations),
		slog.Duration("duration", uc.now().Sub(started)),
	)

	return stats, nil
}

// assignment is a new task planned by Tasker.plan.
type assignment struct {
	markId int
	userId int
}

// userStats is what the probability model knows about a user.
type userStats struct {
	rating  int
	issued  int // tasks currently in UnfulfilledStatus
	overdue int // tasks in OverdueStatus
}

// plan chooses which users get a task for which mark.
//
// For every mark that is not yet covered (P < TargetProbability) the free
// user with the highest verification probability is picked; this repeats
// round by round until every mark is covered or no free candidate is left.
// Assigning a task raises the user's load, which lowers their probability
// on other marks, so the coverage of those marks is recomputed on the fly.
func (uc *Tasker) plan(
	marks []models.Mark,
	users []models.User,
	tasks []models.Task,
	distances []models.DistanceFromMarkToPoint,
) ([]assignment, TaskerStats) {
	stats := TaskerStats{Marks: len(marks), Users: len(users)}

	userStatsById := make(map[int]*userStats, len(users))
	for _, user := range users {
		userStatsById[user.Id] = &userStats{rating: user.Rating}
	}

	// taken[userId][markId] — the user already has a task for the mark (any
	// status), so the pair is never assigned again.
	taken := make(map[int]map[int]bool)
	// existing[markId] — users holding an issued task for the mark.
	existing := make(map[int][]int)
	for _, task := range tasks {
		if _, ok := taken[task.UserID]; !ok {
			taken[task.UserID] = make(map[int]bool)
		}
		taken[task.UserID][task.MarkID] = true

		// GetUsers, GetTasks and GetDistancesFromMarkToPoint are separate
		// reads, so a user may appear in tasks or distances but not in
		// users (deleted in between); such users are never candidates.
		us, ok := userStatsById[task.UserID]
		if !ok {
			uc.log.Debug("task of unknown user skipped",
				slog.Int("user_id", task.UserID), slog.Int("task_id", task.ID))
			continue
		}
		switch task.StatusID {
		case models.UnfulfilledStatus:
			us.issued++
			existing[task.MarkID] = append(existing[task.MarkID], task.UserID)
		case models.OverdueStatus:
			us.overdue++
		case models.CompletedStatus:
		}
	}

	// authorByMark[markId] — the mark's author never verifies their own mark.
	authorByMark := make(map[int]int, len(marks))
	for _, mark := range marks {
		authorByMark[mark.ID] = mark.UserID
	}

	// distanceKm[userId][markId]; unknown pairs fall back to the radius so
	// that they are never over-estimated.
	distanceKm := make(map[int]map[int]float64)
	// free[markId] — users that may still be assigned to the mark.
	free := make(map[int]map[int]bool)
	for _, d := range distances {
		if _, ok := userStatsById[d.UserId]; !ok {
			uc.log.Debug("distance of unknown user skipped",
				slog.Int("user_id", d.UserId), slog.Int("mark_id", d.MarkId))
			continue
		}
		if author, ok := authorByMark[d.MarkId]; ok && author == d.UserId {
			continue
		}
		if _, ok := distanceKm[d.UserId]; !ok {
			distanceKm[d.UserId] = make(map[int]float64)
		}
		distanceKm[d.UserId][d.MarkId] = d.Distance

		if taken[d.UserId][d.MarkId] {
			continue
		}
		if _, ok := free[d.MarkId]; !ok {
			free[d.MarkId] = make(map[int]bool)
		}
		free[d.MarkId][d.UserId] = true
		stats.Candidates++
	}

	probability := func(userId, markId int) float64 {
		dist, ok := distanceKm[userId][markId]
		if !ok {
			dist = float64(uc.cfg.MaxRadiusMeters) / 1000
		}
		return uc.verificationProbability(*userStatsById[userId], dist)
	}

	// assignees[markId] — existing issued users plus new assignments;
	// marksOf[userId] — the inverse, to refresh only the affected marks;
	// coverage[markId] — P(at least RequiredChecks of them verify the mark).
	assignees := make(map[int][]int, len(marks))
	marksOf := make(map[int][]int)
	coverage := make(map[int]float64, len(marks))
	recompute := func(markId int) {
		probs := make([]float64, 0, len(assignees[markId]))
		for _, userId := range assignees[markId] {
			probs = append(probs, probability(userId, markId))
		}
		coverage[markId] = probabilityAtLeastN(uc.cfg.RequiredChecks, probs)
	}

	markIds := make([]int, 0, len(marks))
	for _, mark := range marks {
		markIds = append(markIds, mark.ID)
		assignees[mark.ID] = existing[mark.ID]
		for _, userId := range existing[mark.ID] {
			marksOf[userId] = append(marksOf[userId], mark.ID)
		}
		recompute(mark.ID)
	}
	// Deterministic order makes runs reproducible and testable.
	slices.Sort(markIds)

	var result []assignment
	for {
		stats.Iterations++
		progress := false

		for _, markId := range markIds {
			if coverage[markId] >= uc.cfg.TargetProbability {
				continue
			}

			bestUserId, bestProb := 0, -1.0
			for userId := range free[markId] {
				if userStatsById[userId].issued >= uc.cfg.MaxTasksPerUser {
					delete(free[markId], userId)
					continue
				}
				p := probability(userId, markId)
				if p > bestProb || (p == bestProb && userId < bestUserId) {
					bestUserId, bestProb = userId, p
				}
			}
			if bestUserId == 0 {
				continue
			}

			progress = true
			result = append(result, assignment{markId: markId, userId: bestUserId})
			delete(free[markId], bestUserId)
			userStatsById[bestUserId].issued++
			assignees[markId] = append(assignees[markId], bestUserId)
			marksOf[bestUserId] = append(marksOf[bestUserId], markId)

			// The user's load changed: refresh every mark they are part of.
			for _, id := range marksOf[bestUserId] {
				recompute(id)
			}
		}

		if !progress {
			break
		}
	}

	stats.Assigned = len(result)
	for _, markId := range markIds {
		if coverage[markId] >= uc.cfg.TargetProbability {
			stats.Covered++
		}
	}

	return result, stats
}

// verificationProbability estimates how likely a user is to verify a mark
// located distKm kilometres from their home:
//
//	p = (rating(r) + distance(d)) * load(l) * fatigue(o), clamped to [0, 1]
//
// where r is the user's rating, l the number of issued tasks and o the number
// of overdue tasks (see config.TaskerConfig for the factor definitions).
func (uc *Tasker) verificationProbability(us userStats, distKm float64) float64 {
	p := (ratingFactor(us.rating) + homeDistFactor(distKm, uc.cfg.DistanceLambda)) *
		loadFactor(us.issued, uc.cfg.LoadDelta) *
		fatigueFactor(us.overdue, uc.cfg.FatigueBeta)

	return min(1.0, p)
}

// ratingFactor is a logistic curve of the rating scaled to at most 0.2.
func ratingFactor(rating int) float64 {
	return 0.2 / (1.0 + 100*math.Exp(-float64(rating)/2))
}

// homeDistFactor decays exponentially with the distance in kilometres and
// contributes at most 0.5.
func homeDistFactor(distKm, lambda float64) float64 {
	return 0.5 * math.Exp(-lambda*distKm)
}

// loadFactor penalises users who already hold issued tasks:
// 1 / (1 + delta*(issued+1)).
func loadFactor(issued int, delta float64) float64 {
	return 1.0 / (1.0 + delta*float64(issued+1))
}

// fatigueFactor penalises users with overdue tasks: 1 / (1 + beta*overdue).
// A user who never lets a task expire keeps the factor at 1.
func fatigueFactor(overdue int, beta float64) float64 {
	return 1.0 / (1.0 + beta*float64(overdue))
}

// probabilityAtLeastN returns the probability that at least n of the
// independent events with the given probabilities happen
// (Poisson binomial distribution, computed by dynamic programming).
func probabilityAtLeastN(n int, probabilities []float64) float64 {
	if len(probabilities) < n {
		return 0
	}

	// dp[k] — probability that exactly k events happened so far.
	dp := make([]float64, len(probabilities)+1)
	dp[0] = 1.0
	for _, p := range probabilities {
		for k := len(probabilities); k > 0; k-- {
			dp[k] = dp[k]*(1-p) + dp[k-1]*p
		}
		dp[0] *= 1 - p
	}

	result := 0.0
	for k := n; k <= len(probabilities); k++ {
		result += dp[k]
	}

	return min(1.0, result)
}
