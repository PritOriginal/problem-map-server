package handlers

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
)

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
