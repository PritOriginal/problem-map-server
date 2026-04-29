package handlers

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
)

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

func ParsePhotos(fheaders []*multipart.FileHeader) ([]io.Reader, error) {
	var photos []io.Reader
	for _, header := range fheaders {
		file, err := header.Open()
		if err != nil {
			return photos, err
		}

		img, _, err := image.Decode(file)
		if err != nil {
			return photos, err
		}

		buf := new(bytes.Buffer)
		if err := jpeg.Encode(buf, img, nil); err != nil {
			return photos, err
		}
		photos = append(photos, buf)
	}
	return photos, nil
}
