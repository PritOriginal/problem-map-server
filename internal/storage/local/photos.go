package local

import (
	"context"
	"os"
)

type PhotosRepo struct {
}

func NewPhotos() *PhotosRepo {
	return &PhotosRepo{}
}

func (repo *PhotosRepo) AddPhotos(ctx context.Context, markId, reviewId int, photos [][]byte) error {
	for _, photo := range photos {
		file, err := os.CreateTemp("photos", "p")
		if err != nil {
			return err
		}
		defer file.Close()

		file.Write(photo)
	}

	return nil
}

func (repo *PhotosRepo) GetPhotos(ctx context.Context) (map[int]map[int][]string, error) {
	return map[int]map[int][]string{}, nil
}

func (repo *PhotosRepo) GetPhotosByMarkId(ctx context.Context, arkId int) (map[int]map[int][]string, error) {
	return map[int]map[int][]string{}, nil
}
