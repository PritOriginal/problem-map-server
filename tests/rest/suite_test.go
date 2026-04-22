package tests

import (
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
