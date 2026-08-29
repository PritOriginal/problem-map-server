package app

import (
	"errors"
	"io"
	"testing"

	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/suite"
)

type ClosersSuite struct {
	suite.Suite
}

func TestClosers(t *testing.T) {
	suite.Run(t, new(ClosersSuite))
}

type recordingCloser struct {
	name string
	log  *[]string
	err  error
}

func (c *recordingCloser) Close() error {
	*c.log = append(*c.log, c.name)
	return c.err
}

func (suite *ClosersSuite) TestAdd() {
	var typedNil *recordingCloser

	tests := []struct {
		name string
		c    io.Closer
		want int
	}{
		{name: "NilInterface", c: nil, want: 0},
		{name: "TypedNilPointer", c: typedNil, want: 0},
		{name: "NonNil", c: &recordingCloser{log: new([]string)}, want: 1},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			var cs Closers
			cs.Add(tt.name, tt.c)
			suite.Len(cs, tt.want)
		})
	}
}

func (suite *ClosersSuite) TestCloseReverseOrder() {
	var (
		cs  Closers
		log []string
	)
	cs.Add("database", &recordingCloser{name: "database", log: &log})
	cs.Add("redis", &recordingCloser{name: "redis", log: &log, err: errors.New("boom")})
	var typedNil *recordingCloser
	cs.Add("s3", typedNil)

	suite.NotPanics(func() { cs.Close(slogdiscard.NewDiscardLogger()) })
	suite.Equal([]string{"redis", "database"}, log)
}
