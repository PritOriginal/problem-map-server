//go:build functional && grpc

package tests

import (
	"testing"

	"github.com/PritOriginal/problem-map-server/tests/grpc/suite"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestGetMarks(t *testing.T) {
	ctx, st := suite.New(t)

	resp, err := st.MarksClient.GetMarks(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	for _, mark := range resp.Marks {
		require.NotZero(t, mark.Id)
		require.NotNil(t, mark.Geom, "mark %d must carry geometry", mark.Id)
		require.NotNil(t, mark.Geom.Coordinates)
	}
}
