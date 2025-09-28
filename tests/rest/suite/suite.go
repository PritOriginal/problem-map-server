package suite

import (
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
)

type Suite struct {
	*testing.T
	Cfg *config.Config
}

func New(t *testing.T) *Suite {
	t.Helper()
	t.Parallel()

	cfg := config.MustLoadPath("../../configs/config.yaml")

	return &Suite{
		T:   t,
		Cfg: cfg,
	}
}
