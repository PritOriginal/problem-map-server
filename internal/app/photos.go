package app

import (
	"io"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/repository/local"
	"github.com/PritOriginal/problem-map-server/internal/repository/s3"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

// NewPhotosRepository builds the photo repository selected by cfg.PhotoStorage
// and, when a remote client is used, the closer that must be released on
// shutdown. The closer is a nil interface (not a typed nil) for local storage,
// so it can be passed straight to Closers.Add.
func NewPhotosRepository(log *slog.Logger, cfg *config.Config) (usecase.PhotosRepository, io.Closer) {
	switch cfg.PhotoStorage {
	case config.S3:
		s3Client, err := s3.New(log, cfg.Aws)
		if err != nil {
			log.Error("failed connection to s3", slogger.Err(err))
			panic(err)
		}
		log.Info("s3 connected!")

		return s3.NewPhotos(s3Client), s3Client
	default:
		return local.NewPhotos(), nil
	}
}
