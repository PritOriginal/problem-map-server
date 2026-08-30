package local

import (
	"context"
	"io"
	"os"
)

type PhotosRepo struct {
}

func NewPhotos() *PhotosRepo {
	return &PhotosRepo{}
}

func (repo *PhotosRepo) AddPhotos(ctx context.Context, markId, reviewId int, photos []io.Reader) error {
	for _, photo := range photos {
		file, err := os.CreateTemp("photos", "p")
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, photo); err != nil {
			_ = file.Close()
			return err
		}

		if err := file.Close(); err != nil {
			return err
		}
	}

	return nil
}

// DeletePhotos is a no-op: the local store does not index files by mark.
func (repo *PhotosRepo) DeletePhotos(ctx context.Context, markId int) error {
	return nil
}

func (repo *PhotosRepo) GetPhotos(ctx context.Context) (map[int]map[int][]string, error) {
	return map[int]map[int][]string{}, nil
}

func (repo *PhotosRepo) GetPhotosByMarkId(ctx context.Context, arkId int) (map[int]map[int][]string, error) {
	return map[int]map[int][]string{}, nil
}

func (repo *PhotosRepo) GetPhotosByCheckId(ctx context.Context, markId, checkId int) ([]string, error) {
	return []string{}, nil
}
