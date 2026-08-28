//go:build functional && grpc

package tests

import (
	"net/http"
	"testing"

	"github.com/PritOriginal/problem-map-server/tests/grpc/suite"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestGetMarks(t *testing.T) {
	ctx, st := suite.New(t)

	resp, err := st.MarksClient.GetMarks(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Marks)
}

func BenchmarkGetMarks(b *testing.B) {
	ctx, st := suite.NewBench(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := st.MarksClient.GetMarks(ctx, &emptypb.Empty{})
		_ = resp
		_ = err
	}

	// b.RunParallel(func(p *testing.PB) {
	// 	for p.Next() {
	// 		resp, err := st.MapClient.GetMarks(ctx, &emptypb.Empty{})
	// 		_ = resp
	// 		_ = err
	// 	}
	// })
}

func BenchmarkGetMarks_rest(b *testing.B) {
	client := &http.Client{}

	b.ResetTimer()
	// for i := 0; i < b.N; i++ {
	// 	client.Get("http://localhost:3333/map/marks")
	// 	// http.Get("http://localhost:3333/map/marks")
	// }

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			client.Get("http://localhost:3333/map/marks")
		}
	})
}
