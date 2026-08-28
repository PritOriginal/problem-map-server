//go:build functional && rest

package tests

import (
	"fmt"
	"math/rand"
	"net/url"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
	Cfg  *config.Config
	user struct {
		Username string
		Login    string
		Password string
	}
}

func (st *Suite) SetupSuite() {
	st.Cfg = config.MustLoadPath("../../configs/config.yaml")

	st.user.Username = gofakeit.FirstName()
	st.user.Login = gofakeit.Username()
	st.user.Password = gofakeit.Password(true, true, true, true, true, 10)
}

func Test(t *testing.T) {
	suite.Run(t, new(Suite))
}

type makeUrlParams struct {
	host  string
	port  int
	path  string
	query string
}

func makeUrl(params makeUrlParams) string {
	host := params.host
	if params.port > 0 {
		host = fmt.Sprintf("%s:%d", params.host, params.port)
	}

	u := url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     params.path,
		RawQuery: params.query,
	}
	return u.String()
}

func getImages(maxNum int) [][]byte {
	var images [][]byte
	num := rand.Intn(maxNum) + 1
	for range num {
		isVerticalImg := gofakeit.Bool()
		if isVerticalImg {
			images = append(images, gofakeit.ImageJpeg(9, 12))
		} else {
			images = append(images, gofakeit.ImageJpeg(12, 9))
		}
	}
	return images
}
