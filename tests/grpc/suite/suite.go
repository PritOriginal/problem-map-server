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

type Suite struct {
	*testing.T
	Cfg         *config.Config
	MarksClient pb.MarksClient
}

const (
	grpcHost = "localhost"
)

func New(t *testing.T) (context.Context, *Suite) {
	t.Helper()
	t.Parallel()

	cfg := config.MustLoadPath("../../configs/config.yaml")

	ctx, cancelCtx := context.WithTimeout(context.Background(), cfg.GRPC.Timeout)

	t.Cleanup(func() {
		t.Helper()
		cancelCtx()
	})

	cc, err := grpc.NewClient(
		grpcAddress(cfg),
		grpc.WithTransportCredentials(insecure.NewCredentials())) // Используем insecure-коннект для тестов
	if err != nil {
		t.Fatalf("grpc server connection failed: %v", err)
	}

	return ctx, &Suite{
		T:           t,
		Cfg:         cfg,
		MarksClient: pb.NewMarksClient(cc),
	}
}

func grpcAddress(cfg *config.Config) string {
	return net.JoinHostPort(grpcHost, strconv.Itoa(cfg.GRPC.Port))
}

type SuiteBench struct {
	*testing.B
	Cfg         *config.Config
	MarksClient pb.MarksClient
}

func NewBench(b *testing.B) (context.Context, *SuiteBench) {
	cfg := config.MustLoadPath("../configs/")

	ctx, cancelCtx := context.WithTimeout(context.Background(), cfg.GRPC.Timeout)

	b.Cleanup(func() {
		b.Helper()
		cancelCtx()
	})

	cc, err := grpc.NewClient(
		grpcAddress(cfg),
		grpc.WithTransportCredentials(insecure.NewCredentials())) // Используем insecure-коннект для тестов
	if err != nil {
		b.Fatalf("grpc server connection failed: %v", err)
	}

	return ctx, &SuiteBench{
		B:           b,
		Cfg:         cfg,
		MarksClient: pb.NewMarksClient(cc),
	}
}
