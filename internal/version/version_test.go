package version_test

import (
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/version"
	"github.com/stretchr/testify/suite"
)

type VersionSuite struct {
	suite.Suite
}

func TestVersion(t *testing.T) {
	suite.Run(t, new(VersionSuite))
}

// The build id goes into cache keys, so it must be non-empty, short and
// stable for the life of the process.
func (suite *VersionSuite) TestBuild() {
	build := version.Build()

	suite.NotEmpty(build)
	suite.LessOrEqual(len(build), 12)
	suite.NotContains(build, ":", "a colon would split the cache key")
	suite.Equal(build, version.Build())
}
