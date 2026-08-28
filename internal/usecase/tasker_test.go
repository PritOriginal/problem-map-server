package usecase

import (
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/twpayne/go-geom"
)

func TestTasker_update(t *testing.T) {
	log := slogdiscard.NewDiscardLogger()
	tasksRepo := NewMockTasksRepository(t)
	marksRepo := NewMockMarksRepository(t)
	usersRepo := NewMockUsersRepository(t)
	uc := NewTaskser(log, TaskerRepositories{
		Tasks: tasksRepo,
		Marks: marksRepo,
		Users: usersRepo,
	})
	uc.update(
		[]models.Mark{
			{
				ID:   1,
				Geom: models.NewPoint(geom.Coord{41.451290, 52.720885}),
			},
			{
				ID:   2,
				Geom: models.NewPoint(geom.Coord{41.408142, 52.715772}),
			},
			{
				ID:   3,
				Geom: models.NewPoint(geom.Coord{41.462046, 52.713459}),
			},
		},
		[]models.User{
			{
				Id:        1,
				Rating:    1,
				HomePoint: models.NewPoint(geom.Coord{41.445465, 52.721912}),
			},
			{
				Id:        2,
				Rating:    4,
				HomePoint: models.NewPoint(geom.Coord{41.431332, 52.716574}),
			},
			{
				Id:        3,
				Rating:    2,
				HomePoint: models.NewPoint(geom.Coord{41.447947, 52.713253}),
			},
			{
				Id:        4,
				Rating:    1,
				HomePoint: models.NewPoint(geom.Coord{41.434276, 52.732121}),
			},
			{
				Id:        5,
				Rating:    3,
				HomePoint: models.NewPoint(geom.Coord{41.460438, 52.713030}),
			},
			{
				Id:        6,
				Rating:    3,
				HomePoint: models.NewPoint(geom.Coord{41.460438, 52.713030}),
			},
		},
		[]models.Task{
			{ID: 1, UserID: 2, MarkID: 10},
			{ID: 2, UserID: 5, MarkID: 1},
		},
		[]models.DistanceFromMarkToPoint{
			{MarkId: 1, UserId: 1, Distance: 1},
			{MarkId: 1, UserId: 2, Distance: 4},
			{MarkId: 1, UserId: 3, Distance: 1.5},
			{MarkId: 1, UserId: 4, Distance: 5},
			{MarkId: 1, UserId: 5, Distance: 2},
			{MarkId: 2, UserId: 1, Distance: 0.2},
			{MarkId: 2, UserId: 2, Distance: 4},
			{MarkId: 2, UserId: 3, Distance: 2},
			{MarkId: 2, UserId: 4, Distance: 1},
			{MarkId: 2, UserId: 5, Distance: 6},
			{MarkId: 3, UserId: 1, Distance: 2},
			{MarkId: 3, UserId: 2, Distance: 2},
			{MarkId: 3, UserId: 3, Distance: 1.2},
			{MarkId: 3, UserId: 4, Distance: 3},
			{MarkId: 3, UserId: 5, Distance: 0.5},
		},
	)
}
