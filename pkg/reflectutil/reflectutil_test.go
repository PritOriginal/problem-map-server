package reflectutil_test

import (
	"io"
	"os"
	"testing"

	"github.com/PritOriginal/problem-map-server/pkg/reflectutil"
	"github.com/stretchr/testify/assert"
)

func TestIsNil(t *testing.T) {
	var nilFile *os.File
	var nilCloser io.Closer
	var nilMap map[string]int
	var nilFunc func()

	tests := []struct {
		name string
		v    any
		want bool
	}{
		{name: "UntypedNil", v: nil, want: true},
		{name: "TypedNilPointer", v: nilFile, want: true},
		{name: "NilInterface", v: nilCloser, want: true},
		{name: "NilMap", v: nilMap, want: true},
		{name: "NilFunc", v: nilFunc, want: true},
		{name: "NonNilPointer", v: &os.File{}, want: false},
		{name: "Struct", v: struct{}{}, want: false},
		{name: "Int", v: 0, want: false},
		{name: "EmptySlice", v: []int{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reflectutil.IsNil(tt.v))
		})
	}
}
