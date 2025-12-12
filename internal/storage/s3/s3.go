package s3

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3 struct {
	Client *s3.Client
}

func New(log *slog.Logger, cfg config.AwsConfig) (*S3, error) {
	clientS3 := S3{}
	options := s3.Options{
		Region:       "ru-1",
		BaseEndpoint: aws.String(cfg.EndPoint),
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(cfg.Key, cfg.SecretKey, "")),
		UsePathStyle: true,
	}
	clientS3.Client = s3.New(options)
	log.Info("S3 Client created!")

	// Запрашиваем список бакетов
	buckets, err := clientS3.GetBuckets(context.Background())

	if err != nil {
		return &clientS3, fmt.Errorf("failed get list buckets: %w", err)
	}

	for _, bucket := range buckets {
		log.Info(
			"bucket info",
			slog.String("bucket", aws.ToString(bucket.Name)),
			slog.String("creation time", bucket.CreationDate.Format("2006-01-02 15:04:05 Monday")))
	}

	return &clientS3, nil
}

func (client *S3) GetBuckets(ctx context.Context) ([]types.Bucket, error) {
	const op = "storage.s3.GetBuckets"

	var accessibleBuckets []types.Bucket

	result, err := client.Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return accessibleBuckets, fmt.Errorf("%s: %w", op, err)
	}

	for _, bucket := range result.Buckets {
		_, err := client.Client.HeadBucket(context.Background(), &s3.HeadBucketInput{
			Bucket: bucket.Name,
		})
		if err == nil {
			accessibleBuckets = append(accessibleBuckets, bucket)
		}
	}

	return accessibleBuckets, nil
}
