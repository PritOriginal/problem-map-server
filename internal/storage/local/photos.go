package local

import "os"

type PhotosRepo struct {
}

func NewPhotos() *PhotosRepo {
	return &PhotosRepo{}
}

func (repo *PhotosRepo) AddPhotos(photos [][]byte) error {
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

func (repo *PhotosRepo) GetPhotos() error {

	return nil
}
