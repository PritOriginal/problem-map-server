package s3

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type PhotosRepo struct {
	S3 *S3
}

func NewPhotos(S3 *S3) *PhotosRepo {
	return &PhotosRepo{S3: S3}
}

func (repo *PhotosRepo) AddPhotos(ctx context.Context, markId, checkId int, photos [][]byte) error {
	const op = "storage.s3.AddPhotos"

	buckets, err := repo.S3.GetBuckets(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	for i, photo := range photos {
		objectKey := fmt.Sprintf("marks/%v/%v/%v.png", markId, checkId, i+1)
		err := repo.AddPhoto(ctx, *buckets[0].Name, objectKey, photo)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

func (repo *PhotosRepo) AddPhoto(ctx context.Context, bucketName string, objectKey string, data []byte) error {
	const op = "storage.s3.AddPhoto"

	buf := bytes.NewBuffer(data)

	_, err := repo.S3.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   buf,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (repo *PhotosRepo) GetPhotos(ctx context.Context) (map[int]map[int][]string, error) {
	const op = "storage.s3.GetPhotos"

	photos, err := repo.getPhotos(ctx, &s3.ListObjectsV2Input{
		Prefix: aws.String("marks/"),
	})
	if err != nil {
		return photos, fmt.Errorf("%s: %w", op, err)
	}

	return photos, nil
}

func (repo *PhotosRepo) GetPhotosByMarkId(ctx context.Context, markId int) (map[int]map[int][]string, error) {
	const op = "storage.s3.GetPhotosByMarkId"

	photos, err := repo.getPhotos(ctx, &s3.ListObjectsV2Input{
		Prefix: aws.String(fmt.Sprintf("marks/%v", markId)),
	})
	if err != nil {
		return photos, fmt.Errorf("%s: %w", op, err)
	}

	return photos, nil
}

func (repo *PhotosRepo) getPhotos(ctx context.Context, params *s3.ListObjectsV2Input) (map[int]map[int][]string, error) {
	const op = "storage.s3.getPhotos"

	photos := make(map[int]map[int][]string)

	buckets, err := repo.S3.GetBuckets(ctx)
	if err != nil {
		return photos, fmt.Errorf("%s: %w", op, err)
	}

	for _, bucket := range buckets {
		params.Bucket = aws.String(*bucket.Name)

		paginator := s3.NewListObjectsV2Paginator(repo.S3.Client, params)
		for paginator.HasMorePages() {
			output, err := paginator.NextPage(ctx)
			if err != nil {
				return photos, fmt.Errorf("%s: %w", op, err)
			}
			for _, object := range output.Contents {
				keyParts := strings.Split(*object.Key, "/")

				markId, err := strconv.Atoi(keyParts[1])
				if err != nil {
					return photos, err
				}

				reviewId, err := strconv.Atoi(keyParts[2])
				if err != nil {
					return photos, err
				}

				photo := keyParts[3]

				if photos[markId] == nil {
					photos[markId] = make(map[int][]string)
				}

				photos[markId][reviewId] = append(photos[markId][reviewId], photo)
			}
		}
	}

	return photos, nil
}
