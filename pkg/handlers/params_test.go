package handlers_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/gin-gonic/gin"
)

func (suite *HandlersSuite) TestParamInt() {
	tests := []struct {
		name    string
		param   string
		want    int
		wantErr bool
	}{
		{name: "Ok", param: "42", want: 42},
		{name: "Negative", param: "-1", want: -1},
		{name: "NotANumber", param: "a", wantErr: true},
		{name: "Empty", param: "", wantErr: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Params = gin.Params{{Key: "id", Value: tt.param}}

			got, err := handlers.ParamInt(c, "id")

			if tt.wantErr {
				suite.ErrorIs(err, handlers.ErrBadRequest)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.want, got)
		})
	}
}

func (suite *HandlersSuite) TestQueryIntArray() {
	tests := []struct {
		name    string
		query   string
		want    []int
		wantErr bool
	}{
		{name: "Missing", query: "", want: []int{}},
		{name: "Single", query: "?ids=1", want: []int{1}},
		{name: "Many", query: "?ids=1,2,3", want: []int{1, 2, 3}},
		{name: "NotANumber", query: "?ids=1,a", wantErr: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/"+tt.query, nil)

			got, err := handlers.QueryIntArray(c, "ids")

			if tt.wantErr {
				suite.ErrorIs(err, handlers.ErrBadRequest)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.want, got)
		})
	}
}
