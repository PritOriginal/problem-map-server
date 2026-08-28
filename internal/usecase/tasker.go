package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type Tasker struct {
	log   *slog.Logger
	repos TaskerRepositories
}

type TaskerRepositories struct {
	Tasks TasksRepository
	Marks MarksRepository
	Users UsersRepository
}

func NewTaskser(log *slog.Logger, repos TaskerRepositories) *Tasker {
	return &Tasker{
		log:   log,
		repos: repos,
	}
}

func (uc *Tasker) Update() error {
	const op = "usecase.Tasker.Update"

	uc.log.Debug("start update")
	marks, err := uc.repos.Marks.GetMarks(context.Background(), models.GetMarksFilters{
		MarkStatusIds: []int{
			int(models.UnconfirmedStatus),
			int(models.UnderReviewStatus),
		},
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	uc.log.Debug("marks received")

	users, err := uc.repos.Users.GetUsers(context.Background())
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	uc.log.Debug("users received")

	tasks, err := uc.repos.Tasks.GetTasks(context.Background(), models.GetTasksFilters{
		Statuses: []int{
			int(models.UnfulfilledStatus),
		},
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	uc.log.Debug("tasks received")

	distances, err := uc.repos.Marks.GetDistancesFromMarkToPoint(context.Background(), models.GetDistanceFromMarkToPointFilters{
		MarkStatusIds: []models.MarkStatusType{
			models.UnconfirmedStatus,
			models.UnderReviewStatus,
		},
		MaxRadius: 5000,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	uc.log.Debug("distances received")

	assignments := uc.update(marks, users, tasks, distances)

	for markId, users := range assignments {
		for userId := range users {
			_, err := uc.repos.Tasks.AddTask(context.Background(), models.Task{
				MarkID: markId,
				UserID: userId,
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type userWithStats struct {
	*models.User
	numAssignedTasks int
}

// func (uc *Tasker) update(marks []models.Mark, users []models.User, tasks []models.Task, distances []models.DistanceFromMarkToPoint) map[int][]userWithProbability {
// 	// fmt.Println(marks, users, tasks, distances)

// 	marksMap := make(map[int]markWithStats, len(marks))
// 	for _, mark := range marks {
// 		marksMap[mark.ID] = markWithStats{
// 			Mark: &mark,
// 		}
// 	}

// 	usersMap := make(map[int]userWithStats, len(users))
// 	for _, user := range users {
// 		usersMap[user.Id] = userWithStats{
// 			User: &user,
// 		}
// 	}

// 	distancesMap := make(map[int]map[int]float64)
// 	for _, distance := range distances {
// 		if _, exist := distancesMap[distance.UserId]; !exist {
// 			distancesMap[distance.UserId] = make(map[int]float64)
// 			// distancesMap[distance.UserId] = make(map[int]models.DistanceFromMarkToPoint)
// 		}
// 		distancesMap[distance.UserId][distance.MarkId] = distance.Distance
// 	}

// 	S := make(map[int][]userWithProbability)
// 	P := make(map[int]float64)
// 	p := make(map[int]map[int]markWithProbability)

// 	for userId := range distancesMap {
// 		p[userId] = make(map[int]markWithProbability)
// 		for markId := range distancesMap[userId] {
// 			// if _, exist := p[user.Id][mark.ID]; !exist {
// 			// }
// 			p[userId][markId] = markWithProbability{
// 				markId:      markId,
// 				probability: probabilityVerification(usersMap[userId], distancesMap[userId][markId]),
// 			}
// 			// p[userIdx][markIdx] = markWithProbability{
// 			// 	markId:      mark.ID,
// 			// 	probability: probabilityVerification(),
// 			// }
// 		}
// 		// sort.Slice(p, func(i, j int) bool {
// 		// 	return p[userIdx][i].probability < p[userIdx][j].probability
// 		// })
// 	}

// 	// tasksMap := make(map[int]models.Task, len(tasks))
// 	for _, task := range tasks {
// 		delete(p[task.UserID], task.MarkID)
// 	}

// 	// for _, user := range users {
// 	// 	p[user.Id] = make(map[int]markWithProbability)
// 	// 	for _, mark := range marks {
// 	// 		// if _, exist := p[user.Id][mark.ID]; !exist {
// 	// 		// }
// 	// 		p[user.Id][mark.ID] = markWithProbability{
// 	// 			markId:      mark.ID,
// 	// 			probability: probabilityVerification(usersMap[user.Id], mark, distancesMap[user.Id][mark.ID]),
// 	// 		}
// 	// 		// p[userIdx][markIdx] = markWithProbability{
// 	// 		// 	markId:      mark.ID,
// 	// 		// 	probability: probabilityVerification(),
// 	// 		// }
// 	// 	}
// 	// 	// sort.Slice(p, func(i, j int) bool {
// 	// 	// 	return p[userIdx][i].probability < p[userIdx][j].probability
// 	// 	// })
// 	// }

// 	iter := 1
// 	for len(marksMap) > 0 && len(p) > 0 {
// 		for _, mark := range marksMap {
// 			var bestProb float64
// 			var bestUserId int
// 			for userId := range p {
// 				if p, exist := p[userId][mark.ID]; exist {
// 					if bestProb < p.probability {
// 						bestProb = p.probability
// 						bestUserId = userId
// 					}
// 				}
// 			}

// 			if bestUserId == 0 {
// 				continue
// 			}

// 			S[mark.ID] = append(S[mark.ID], userWithProbability{
// 				userId:      bestUserId,
// 				probability: bestProb,
// 			})
// 			delete(p[bestUserId], mark.ID)
// 			bestUser := usersMap[bestUserId]
// 			bestUser.numAssignedTasks++
// 			usersMap[bestUserId] = bestUser
// 			if len(p[bestUserId]) == 0 || bestUser.numAssignedTasks >= 3 {
// 				delete(p, bestUserId)
// 			}

// 			P[mark.ID] = probabilityVerificationByN(2, S[mark.ID], mark.ID, p)

// 			for markId := range p[bestUserId] {
// 				p[bestUserId][markId] = markWithProbability{
// 					markId:      markId,
// 					probability: probabilityVerification(usersMap[bestUserId], mark, distancesMap[usersMap[bestUserId].Id][markId]),
// 				}
// 			}

// 			if P[mark.ID] >= 0.8 {
// 				delete(marksMap, mark.ID)
// 				for userId := range p {
// 					delete(p[userId], mark.ID)
// 					if len(p[userId]) == 0 {
// 						delete(p, userId)
// 					}
// 				}
// 			}
// 		}
// 		fmt.Println(iter, len(marksMap), len(p))
// 		// pKeys := slices.Collect(maps.Keys(p))
// 		// fmt.Println(iter, pKeys[0], p[pKeys[0]])
// 		// pKeys2 := slices.Collect(maps.Keys(p[pKeys[0]]))
// 		// fmt.Println(marksMap[pKeys2[0]])

// 		// fmt.Println(marksMap[53])
// 		// time.Sleep(time.Second)
// 		iter++
// 	}
// 	// fmt.Println(S)

// 	fmt.Println("Выданные задания")
// 	sum := 0
// 	for markId, users := range S {
// 		fmt.Printf("mark_id: %d - %d\n", markId, len(users))
// 		sum += len(users)
// 	}
// 	fmt.Println("Sum: ", sum)

// 	for _, user := range usersMap {
// 		fmt.Printf("user_id: %d - %d\n", user.Id, user.numAssignedTasks)
// 	}

// 	return S
// }

func (uc *Tasker) update(marks []models.Mark, users []models.User, tasks []models.Task, distances []models.DistanceFromMarkToPoint) map[int]map[int]float64 {
	marksMap := make(map[int]models.Mark, len(marks))
	// unallocatedMarks := make(map[int]models.Mark, len(marks))
	for _, mark := range marks {
		marksMap[mark.ID] = mark
		// unallocatedMarks[mark.ID] = mark
	}

	usersMap := make(map[int]userWithStats, len(users))
	for _, user := range users {
		usersMap[user.Id] = userWithStats{
			User: &user,
		}
	}

	distancesMap := make(map[int]map[int]float64)
	for _, distance := range distances {
		if _, exist := distancesMap[distance.UserId]; !exist {
			distancesMap[distance.UserId] = make(map[int]float64)
		}
		distancesMap[distance.UserId][distance.MarkId] = distance.Distance
	}

	assignmentsOld := make(map[int]map[int]float64)
	assignments := make(map[int]map[int]float64)
	P := make(map[int]float64)
	probalities := make(map[int]map[int]float64)

	for _, task := range tasks {
		user := usersMap[task.UserID]
		user.numAssignedTasks++
		usersMap[task.UserID] = user
	}

	for _, task := range tasks {
		user := usersMap[task.UserID]
		user.numAssignedTasks--

		if _, exist := assignmentsOld[task.MarkID]; !exist {
			assignmentsOld[task.MarkID] = map[int]float64{}
		}
		assignmentsOld[task.MarkID][task.UserID] = probabilityVerification(user, distancesMap[task.UserID][task.MarkID])
	}

	freeUsers := make(map[int]map[int]userWithStats, len(users))

	for userId := range distancesMap {
		probalities[userId] = make(map[int]float64)
		freeUsers[userId] = make(map[int]userWithStats)
		for markId := range distancesMap[userId] {
			probalities[userId][markId] = probabilityVerification(usersMap[userId], distancesMap[userId][markId])
			freeUsers[userId][markId] = usersMap[userId]
		}
	}

	for markId := range assignmentsOld {
		for userId := range assignmentsOld[markId] {
			delete(freeUsers[userId], markId)
			if len(freeUsers[userId]) == 0 {
				delete(freeUsers, userId)
			}
		}
	}

	for userId := range freeUsers {
		if usersMap[userId].numAssignedTasks >= 3 {
			delete(freeUsers, userId)
		}
	}

	for _, mark := range marks {
		assignmentsAll := append(
			slices.Collect(maps.Values(assignmentsOld[mark.ID])),
			slices.Collect(maps.Values(assignments[mark.ID]))...,
		)
		P[mark.ID] = probabilityVerificationByN(2, assignmentsAll)

		fmt.Printf("mark_id: %d - %d; P = %f\n", mark.ID, len(assignmentsOld[mark.ID]), P[mark.ID])

		// if P[mark.ID] >= 0.8 {
		// 	delete(unallocatedMarks, mark.ID)
		// 	for userId := range freeUsers {
		// 		delete(freeUsers[userId], mark.ID)
		// 		if len(freeUsers[userId]) == 0 {
		// 			delete(freeUsers, userId)
		// 		}
		// 	}
		// }
	}

	iter := 0
	for len(freeUsers) > 0 {
		numUnallocatedMarks := 0
		allMarksAllocated := true
		allUsersAllocated := true
		for _, mark := range marksMap {
			if P[mark.ID] >= 0.8 {
				continue
			}
			allMarksAllocated = false
			numUnallocatedMarks++

			var bestProb float64
			var bestUserId int
			for userId := range freeUsers {
				if _, exist := freeUsers[userId][mark.ID]; exist {
					if bestProb < probalities[userId][mark.ID] {
						bestProb = probalities[userId][mark.ID]
						bestUserId = userId
					}
				}
			}

			if bestUserId == 0 {
				// fmt.Println("нет лучшего пользователя")
				// time.Sleep(time.Millisecond * 100)
				continue
			}
			allUsersAllocated = false

			if _, exist := assignments[mark.ID]; !exist {
				assignments[mark.ID] = map[int]float64{}
			}
			assignments[mark.ID][bestUserId] = bestProb
			// assignments[mark.ID] = append(assignments[mark.ID], userWithProbability{
			// 	userId:      bestUserId,
			// 	probability: bestProb,
			// })
			delete(freeUsers[bestUserId], mark.ID)
			bestUser := usersMap[bestUserId]
			bestUser.numAssignedTasks++
			usersMap[bestUserId] = bestUser
			if len(freeUsers[bestUserId]) == 0 || bestUser.numAssignedTasks >= 3 {
				delete(freeUsers, bestUserId)
			}

			for markId := range assignmentsOld {
				if _, exist := assignmentsOld[markId][bestUserId]; exist {
					assignmentsOld[markId][bestUserId] = probalities[bestUserId][markId]

					assignmentsAll := append(
						slices.Collect(maps.Values(assignmentsOld[markId])),
						slices.Collect(maps.Values(assignments[markId]))...,
					)
					P[markId] = probabilityVerificationByN(2, assignmentsAll)
				}
			}
			for markId := range assignments {
				if _, exist := assignments[markId][bestUserId]; exist {
					assignments[markId][bestUserId] = probalities[bestUserId][markId]

					assignmentsAll := append(
						slices.Collect(maps.Values(assignmentsOld[markId])),
						slices.Collect(maps.Values(assignments[markId]))...,
					)
					P[markId] = probabilityVerificationByN(2, assignmentsAll)
				}
			}

			for markId := range probalities[bestUserId] {
				probalities[bestUserId][markId] = probabilityVerification(usersMap[bestUserId], distancesMap[bestUserId][markId])
			}
		}

		fmt.Println(iter, numUnallocatedMarks, len(freeUsers))
		iter++

		if allMarksAllocated || allUsersAllocated {
			break
		}
	}

	fmt.Println("Выданные задания")
	sum := 0
	for markId, assignedUsers := range assignments {
		fmt.Printf("mark_id: %d - %d; P = %f\n", markId, len(assignedUsers), P[markId])
		sum += len(assignedUsers)
	}
	fmt.Println("Sum: ", sum)

	// for _, user := range usersMap {
	// 	fmt.Printf("user_id: %d - %d\n", user.Id, user.numAssignedTasks)
	// }

	return assignments
}

func probabilityVerification(user userWithStats, distance float64) float64 {
	// homeDist := xy.Distance(user.HomePoint.Ewkb.Coords(), mark.Geom.Ewkb.Coords())

	probability := (ratingFactor(user.Rating) + homeDistFactor(distance, 0.05)) * loadFactor(user.numAssignedTasks, 0.3) * fatigueFactor(0, 0.2)
	// probability = (ratingFactor(user.Rating) + homeDistFactor(distance, 0.05)) * loadFactor(user.numAssignedTasks, 0.3) * fatigueFactor(0, 0.2)

	return min(1.0, probability)
}

func ratingFactor(r int) float64 {
	res := 1.0 / (1.0 + 100*math.Exp(-float64(r)/2)) * 0.2
	return res
	// return 1.0 / (1.0 + 100*math.Exp(-float64(r)/2)) * 0.2
}

func loadFactor(r int, delta float64) float64 {
	res := 1.0 / (1.0 + delta*float64(r+1))
	return res
}

// Функция усталости g(a) = 1/(1+beta*a)
func fatigueFactor(a int, beta float64) float64 {
	res := 1.0 / (1.0 + beta*float64(a))
	return res
}

func homeDistFactor(dist float64, lambda float64) float64 {
	res := math.Exp(-lambda*dist) * 0.5
	return res
}

// func probabilityVerificationByN(n int, users []userWithProbability, markId int, probalities map[int]map[int]float64) float64 {
func probabilityVerificationByN(n int, probabilities []float64) float64 {
	if len(probabilities) < n {
		return 0
	}

	dp := make([]float64, len(probabilities)+1)
	dp[0] = 1.0

	for _, user := range probabilities {
		p := user

		for k := len(probabilities); k >= 0; k-- {
			if k > 0 {
				dp[k] = dp[k]*(1-p) + dp[k-1]*p
			} else {
				dp[k] = dp[k] * (1 - p)
			}
		}
	}

	result := 0.0
	for k := n; k <= len(probabilities); k++ {
		result += dp[k]
	}
	if result > 1.0 {
		result = 1.0
	}
	return result
}
