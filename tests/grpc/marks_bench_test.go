//go:build functional && grpc

package tests

import (
	"io"
	"net/http"
	"testing"

	"github.com/PritOriginal/problem-map-server/tests/grpc/suite"
	"google.golang.org/protobuf/types/known/emptypb"
)

// BenchmarkGetMarks_grpc measures sequential GetMarks calls over gRPC.
func BenchmarkGetMarks_grpc(b *testing.B) {
	ctx, st := suite.NewBench(b)

	b.ResetTimer()
	for b.Loop() {
		if _, err := st.MarksClient.GetMarks(ctx, &emptypb.Empty{}); err != nil {
			b.Fatalf("GetMarks: %v", err)
		}
	}
}

// BenchmarkGetMarks_grpcParallel measures GetMarks over gRPC with concurrent
// callers sharing one connection.
func BenchmarkGetMarks_grpcParallel(b *testing.B) {
	ctx, st := suite.NewBench(b)

	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			if _, err := st.MarksClient.GetMarks(ctx, &emptypb.Empty{}); err != nil {
				b.Errorf("GetMarks: %v", err)
				return
			}
		}
	})
}

// BenchmarkGetMarks_rest is the REST counterpart (GET /marks) against the
// server from the same config, for a like-for-like comparison.
func BenchmarkGetMarks_rest(b *testing.B) {
	_, st := suite.NewBench(b)
	url := suite.RESTAddress(st.Cfg) + "/marks"
	client := &http.Client{Timeout: st.Cfg.GRPC.Timeout}

	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			resp, err := client.Get(url)
			if err != nil {
				b.Errorf("GET %s: %v", url, err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b.Errorf("GET %s: status %d", url, resp.StatusCode)
				return
			}
		}
	})
}
