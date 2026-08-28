package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"strconv"
	"strings"

	// Register decoders used by image.Decode / image.DecodeConfig.
	_ "image/gif"
	_ "image/png"
)

const (
	// MaxPhotos is the maximum number of photos accepted in a single request.
	MaxPhotos = 5
	// MaxPhotoSize is the maximum size of a single uploaded photo in bytes.
	MaxPhotoSize = 10 << 20 // 10 MiB
	// MaxPhotoDimension is the maximum width/height of an uploaded photo in pixels.
	MaxPhotoDimension = 8000
)

// ErrInvalidPhoto is returned (wrapped) by ParsePhotos when the uploaded
// files violate the limits or cannot be decoded as an image. Handlers should
// map it to a 400 response.
var ErrInvalidPhoto = errors.New("invalid photo")

func ParseIntArray(param string) ([]int, error) {
	if param == "" {
		return []int{}, nil
	}

	parts := strings.Split(param, ",")
	result := make([]int, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		num, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		result = append(result, num)
	}

	return result, nil
}

// ParsePhotos validates the uploaded files and re-encodes them as JPEG.
// Validation failures are reported as errors wrapping ErrInvalidPhoto.
func ParsePhotos(fheaders []*multipart.FileHeader) ([]io.Reader, error) {
	if len(fheaders) > MaxPhotos {
		return nil, fmt.Errorf("%w: too many files (%d > %d)", ErrInvalidPhoto, len(fheaders), MaxPhotos)
	}

	photos := make([]io.Reader, 0, len(fheaders))
	for _, header := range fheaders {
		if header.Size > MaxPhotoSize {
			return nil, fmt.Errorf("%w: file %q is too large (%d > %d bytes)",
				ErrInvalidPhoto, header.Filename, header.Size, MaxPhotoSize)
		}

		photo, err := parsePhoto(header)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}
	return photos, nil
}

func parsePhoto(header *multipart.FileHeader) (io.Reader, error) {
	file, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", header.Filename, err)
	}
	defer func() { _ = file.Close() }()

	// Guard against decompression bombs: read only the header first.
	limited := io.LimitReader(file, MaxPhotoSize)
	cfg, _, err := image.DecodeConfig(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrInvalidPhoto, header.Filename, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > MaxPhotoDimension || cfg.Height > MaxPhotoDimension {
		return nil, fmt.Errorf("%w: %q: image dimensions %dx%d exceed %dx%d",
			ErrInvalidPhoto, header.Filename, cfg.Width, cfg.Height, MaxPhotoDimension, MaxPhotoDimension)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek %q: %w", header.Filename, err)
	}

	img, _, err := image.Decode(io.LimitReader(file, MaxPhotoSize))
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrInvalidPhoto, header.Filename, err)
	}

	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, nil); err != nil {
		return nil, fmt.Errorf("encode %q: %w", header.Filename, err)
	}
	return buf, nil
}
