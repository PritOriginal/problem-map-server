package logger

import (
	"errors"
	"log/slog"
	"reflect"
	"testing"
)

func TestErr(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want slog.Attr
	}{
		{
			name: "Test1",
			args: args{
				err: errors.New("test"),
			},
			want: slog.Attr{
				Key:   "error",
				Value: slog.StringValue(errors.New("test").Error()),
			},
		},
		{
			name: "Test2",
			args: args{
				err: errors.New("1234"),
			},
			want: slog.Attr{
				Key:   "error",
				Value: slog.StringValue(errors.New("1234").Error()),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Err(tt.args.err); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Err() = %v, want %v", got, tt.want)
			}
		})
	}
}
