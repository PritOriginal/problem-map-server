package handlers_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/suite"
)

type HandlersSuite struct {
	suite.Suite
}

func TestHandlers(t *testing.T) {
	suite.Run(t, new(HandlersSuite))
}

type file struct {
	name string
	data []byte
}

// multipartHeaders builds multipart file headers the same way Gin does.
func (suite *HandlersSuite) multipartHeaders(files []file) []*multipart.FileHeader {
	b := &bytes.Buffer{}
	mpw := multipart.NewWriter(b)
	for _, f := range files {
		fw, err := mpw.CreateFormFile("photos", f.name)
		suite.NoError(err)
		_, err = io.Copy(fw, bytes.NewReader(f.data))
		suite.NoError(err)
	}
	suite.NoError(mpw.Close())

	req := httptest.NewRequest(http.MethodPost, "/", b)
	req.Header.Set("Content-Type", mpw.FormDataContentType())
	suite.NoError(req.ParseMultipartForm(32 << 20))

	return req.MultipartForm.File["photos"]
}

// hugePNG encodes a mostly-empty PNG with the given dimensions; it stays small
// on disk because it compresses well, so only the dimension check can reject it.
func hugePNG(width, height int) []byte {
	img := image.NewGray(image.Rect(0, 0, width, height))
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (suite *HandlersSuite) TestParsePhotos() {
	jpg := gofakeit.ImageJpeg(10, 10)
	pngImg := gofakeit.ImagePng(10, 10)

	tests := []struct {
		name       string
		files      []file
		wantCount  int
		wantErr    bool
		wantBadReq bool
	}{
		{
			name:      "NoFiles",
			wantCount: 0,
		},
		{
			name:      "JpegAndPng",
			files:     []file{{"a.jpg", jpg}, {"b.png", pngImg}},
			wantCount: 2,
		},
		{
			name:       "NotAnImage",
			files:      []file{{"a.jpg", []byte("not an image")}},
			wantErr:    true,
			wantBadReq: true,
		},
		{
			name: "TooManyFiles",
			files: []file{
				{"1.jpg", jpg}, {"2.jpg", jpg}, {"3.jpg", jpg},
				{"4.jpg", jpg}, {"5.jpg", jpg}, {"6.jpg", jpg},
			},
			wantErr:    true,
			wantBadReq: true,
		},
		{
			name:       "TooLarge",
			files:      []file{{"big.jpg", make([]byte, handlers.MaxPhotoSize+1)}},
			wantErr:    true,
			wantBadReq: true,
		},
		{
			name:       "TooWide",
			files:      []file{{"wide.png", hugePNG(handlers.MaxPhotoDimension+1, 1)}},
			wantErr:    true,
			wantBadReq: true,
		},
		{
			name:       "TooTall",
			files:      []file{{"tall.png", hugePNG(1, handlers.MaxPhotoDimension+1)}},
			wantErr:    true,
			wantBadReq: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			photos, err := handlers.ParsePhotos(suite.multipartHeaders(tt.files))

			if tt.wantErr {
				suite.Error(err)
				suite.Equal(tt.wantBadReq, errors.Is(err, handlers.ErrInvalidPhoto))
				return
			}

			suite.NoError(err)
			suite.Len(photos, tt.wantCount)
			for _, photo := range photos {
				_, format, err := image.Decode(photo)
				suite.NoError(err)
				suite.Equal("jpeg", format)
			}
		})
	}
}
