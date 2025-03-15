package s3

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Repo struct {
	Client *s3.Client
}

func Initialize(log *slog.Logger) *S3Repo {
	clientS3 := S3Repo{}
	options := s3.Options{
		Region:       "ru-1",
		BaseEndpoint: aws.String(os.Getenv("AWS_END_POINT")),
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(os.Getenv("AWS_KEY"), os.Getenv("AWS_SECRET_KEY"), "")),
		UsePathStyle: true,
	}
	clientS3.Client = s3.New(options)
	log.Info("S3 Client created!")

	// Запрашиваем список бакетов
	result, err := clientS3.Client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		log.Error("failed get list buckets", logger.Err(err))
	}

	for _, bucket := range result.Buckets {
		log.Info(
			"bucket info",
			slog.String("bucket", aws.ToString(bucket.Name)),
			slog.String("creation time", bucket.CreationDate.Format("2006-01-02 15:04:05 Monday")))
	}

	return &clientS3
}

func (client *S3Repo) GetBuckets(ctx context.Context) ([]types.Bucket, error) {
	const op = "storage.s3.GetBuckets"

	// Запрашиваем список бакетов
	result, err := client.Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return result.Buckets, fmt.Errorf("%s: %w", op, err)
	}
	return result.Buckets, nil
}
