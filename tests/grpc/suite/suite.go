//go:build functional && grpc

package suite

import (
	"context"
	"net"
	"strconv"
	"testing"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ConfigPath is the config the functional tests run against, relative to the
// tests/grpc package. The gRPC (and REST, for benchmarks) servers under test
// must be started with the same file.
const ConfigPath = "../../configs/config-tests.yaml"

// Suite carries a ready-to-use gRPC client for a functional test.
type Suite struct {
	*testing.T
	Cfg         *config.Config
	MarksClient pb.MarksClient
}

// SuiteBench is the benchmark counterpart of Suite.
type SuiteBench struct {
	*testing.B
	Cfg         *config.Config
	MarksClient pb.MarksClient
}

// New connects to the gRPC server from ConfigPath for a parallel test.
func New(t *testing.T) (context.Context, *Suite) {
	t.Helper()
	t.Parallel()

	ctx, cfg, client := newClient(t)
	return ctx, &Suite{T: t, Cfg: cfg, MarksClient: client}
}

// NewBench connects to the gRPC server from ConfigPath for a benchmark.
func NewBench(b *testing.B) (context.Context, *SuiteBench) {
	b.Helper()

	ctx, cfg, client := newClient(b)
	return ctx, &SuiteBench{B: b, Cfg: cfg, MarksClient: client}
}

// newClient loads the test config, dials the gRPC server and returns a
// context bounded by the configured timeout. Connection and context are
// released through tb.Cleanup.
func newClient(tb testing.TB) (context.Context, *config.Config, pb.MarksClient) {
	tb.Helper()

	cfg := config.MustLoadPath(ConfigPath)

	ctx, cancelCtx := context.WithTimeout(context.Background(), cfg.GRPC.Timeout)
	tb.Cleanup(cancelCtx)

	// Insecure transport is fine for the local test server.
	cc, err := grpc.NewClient(GRPCAddress(cfg), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		tb.Fatalf("grpc server connection failed: %v", err)
	}
	tb.Cleanup(func() { _ = cc.Close() })

	return ctx, cfg, pb.NewMarksClient(cc)
}

// GRPCAddress is the address of the gRPC server under test. The gRPC config
// has no host, so the REST host from the same config is reused.
func GRPCAddress(cfg *config.Config) string {
	return net.JoinHostPort(cfg.REST.Host, strconv.Itoa(cfg.GRPC.Port))
}

// RESTAddress is the base URL of the REST server under test.
func RESTAddress(cfg *config.Config) string {
	return "http://" + net.JoinHostPort(cfg.REST.Host, strconv.Itoa(cfg.REST.Port))
}
