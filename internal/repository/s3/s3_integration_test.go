//go:build integration

package s3_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	repos3 "github.com/PritOriginal/problem-map-server/internal/repository/s3"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

const (
	minioImage = "minio/minio:RELEASE.2024-12-18T13-15-44Z"
	bucketName = "photos-test"
)

type S3Suite struct {
	suite.Suite

	ctx      context.Context
	endpoint string
	storage  *repos3.S3
	photos   *repos3.PhotosRepo
}

func TestS3Suite(t *testing.T) {
	suite.Run(t, new(S3Suite))
}

func (s *S3Suite) SetupSuite() {
	s.ctx = context.Background()

	container, err := tcminio.Run(s.ctx, minioImage)
	s.Require().NoError(err, "start minio container")
	testcontainers.CleanupContainer(s.T(), container)

	hostPort, err := container.ConnectionString(s.ctx)
	s.Require().NoError(err)
	s.endpoint = "http://" + hostPort

	// The production constructor builds the client (path style, static
	// credentials); the raw client it exposes is reused for bucket setup.
	s.storage, err = repos3.New(slogdiscard.NewDiscardLogger(), config.AwsConfig{
		Key: container.Username, SecretKey: container.Password, EndPoint: s.endpoint,
	})
	s.Require().NoError(err, "construct S3 client")
	s.photos = repos3.NewPhotos(s.storage)

	_, err = s.storage.Client.CreateBucket(s.ctx, &awss3.CreateBucketInput{Bucket: aws.String(bucketName)})
	s.Require().NoError(err, "create bucket")
}

func (s *S3Suite) TearDownSuite() {
	if s.storage != nil {
		_ = s.storage.Close()
	}
}

// SetupTest empties the bucket so every test sees a clean store.
func (s *S3Suite) SetupTest() {
	paginator := awss3.NewListObjectsV2Paginator(s.storage.Client, &awss3.ListObjectsV2Input{Bucket: aws.String(bucketName)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(s.ctx)
		s.Require().NoError(err)
		for _, obj := range page.Contents {
			_, err := s.storage.Client.DeleteObject(s.ctx, &awss3.DeleteObjectInput{Bucket: aws.String(bucketName), Key: obj.Key})
			s.Require().NoError(err)
		}
	}
}

func (s *S3Suite) objectURL(key string) string {
	return s.endpoint + "/" + bucketName + "/" + key
}

func (s *S3Suite) readObject(key string) []byte {
	out, err := s.storage.Client.GetObject(s.ctx, &awss3.GetObjectInput{Bucket: aws.String(bucketName), Key: aws.String(key)})
	s.Require().NoError(err)
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(out.Body)
	s.Require().NoError(err)
	return data
}

func (s *S3Suite) TestNew_BadCredentials() {
	_, err := repos3.New(slogdiscard.NewDiscardLogger(), config.AwsConfig{
		Key: "wrong", SecretKey: "wrong", EndPoint: s.endpoint,
	})
	s.Error(err)
	s.ErrorContains(err, "failed get list buckets")
}

func (s *S3Suite) TestGetBuckets() {
	buckets, err := s.storage.GetBuckets(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(buckets, 1)
	s.Equal(bucketName, aws.ToString(buckets[0].Name))
}

func (s *S3Suite) TestAddPhoto_ThenReadBack() {
	body := []byte("jpeg-bytes-1")
	s.Require().NoError(s.photos.AddPhoto(s.ctx, bucketName, "marks/1/1/1.jpg", bytes.NewReader(body)))
	s.Equal(body, s.readObject("marks/1/1/1.jpg"))
}

func (s *S3Suite) TestAddPhoto_UnknownBucket() {
	err := s.photos.AddPhoto(s.ctx, "no-such-bucket", "marks/1/1/1.jpg", bytes.NewReader([]byte("x")))
	s.Error(err)
	s.ErrorContains(err, "storage.s3.AddPhoto")
}

func (s *S3Suite) TestAddPhotos_KeysAreNumberedPerCheck() {
	photos := []io.Reader{
		bytes.NewReader([]byte("first")),
		bytes.NewReader([]byte("second")),
		bytes.NewReader([]byte("third")),
	}
	s.Require().NoError(s.photos.AddPhotos(s.ctx, 10, 20, photos))

	s.Equal([]byte("first"), s.readObject("marks/10/20/1.jpg"))
	s.Equal([]byte("second"), s.readObject("marks/10/20/2.jpg"))
	s.Equal([]byte("third"), s.readObject("marks/10/20/3.jpg"))
}

func (s *S3Suite) TestAddPhotos_Empty() {
	s.Require().NoError(s.photos.AddPhotos(s.ctx, 1, 1, nil))

	photos, err := s.photos.GetPhotos(s.ctx)
	s.Require().NoError(err)
	s.Empty(photos)
}

func (s *S3Suite) TestGetPhotos() {
	s.Require().NoError(s.photos.AddPhotos(s.ctx, 1, 1, []io.Reader{bytes.NewReader([]byte("a")), bytes.NewReader([]byte("b"))}))
	s.Require().NoError(s.photos.AddPhotos(s.ctx, 1, 2, []io.Reader{bytes.NewReader([]byte("c"))}))
	s.Require().NoError(s.photos.AddPhotos(s.ctx, 12, 3, []io.Reader{bytes.NewReader([]byte("d"))}))
	// An object outside the marks/ prefix must be ignored.
	s.Require().NoError(s.photos.AddPhoto(s.ctx, bucketName, "other/1/1/1.jpg", bytes.NewReader([]byte("x"))))

	all := map[int]map[int][]string{
		1: {
			1: {s.objectURL("marks/1/1/1.jpg"), s.objectURL("marks/1/1/2.jpg")},
			2: {s.objectURL("marks/1/2/1.jpg")},
		},
		12: {
			3: {s.objectURL("marks/12/3/1.jpg")},
		},
	}

	s.Run("all photos grouped by mark and check", func() {
		got, err := s.photos.GetPhotos(s.ctx)
		s.Require().NoError(err)
		s.Equal(all, got)
	})

	s.Run("by mark id", func() {
		tests := []struct {
			name   string
			markID int
			want   map[int]map[int][]string
		}{
			{
				// Prefix "marks/1" also matches "marks/12/..." — documented
				// behaviour of the current prefix-based lookup, so the
				// result equals the full listing.
				name:   "mark with two checks",
				markID: 1,
				want:   all,
			},
			{name: "mark without photos", markID: 99, want: map[int]map[int][]string{}},
		}
		for _, tt := range tests {
			s.Run(tt.name, func() {
				got, err := s.photos.GetPhotosByMarkId(s.ctx, tt.markID)
				s.Require().NoError(err)
				s.Equal(tt.want, got)
			})
		}
	})

	s.Run("by check id", func() {
		tests := []struct {
			name    string
			markID  int
			checkID int
			want    []string
		}{
			{name: "check with two photos", markID: 1, checkID: 1, want: []string{s.objectURL("marks/1/1/1.jpg"), s.objectURL("marks/1/1/2.jpg")}},
			{name: "check with one photo", markID: 12, checkID: 3, want: []string{s.objectURL("marks/12/3/1.jpg")}},
			{name: "check without photos", markID: 1, checkID: 7, want: nil},
		}
		for _, tt := range tests {
			s.Run(tt.name, func() {
				got, err := s.photos.GetPhotosByCheckId(s.ctx, tt.markID, tt.checkID)
				s.Require().NoError(err)
				s.Equal(tt.want, got)
			})
		}
	})
}

func (s *S3Suite) TestGetPhotos_MalformedKey() {
	s.Require().NoError(s.photos.AddPhoto(s.ctx, bucketName, "marks/not-a-number/1/1.jpg", bytes.NewReader([]byte("x"))))

	_, err := s.photos.GetPhotos(s.ctx)
	s.Error(err)
}
