package usecase_test

import (
	"context"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const typesCachePrefix = "http:GET:/marks/types"

type MarkTypesSuite struct {
	suite.Suite
	repo  *usecase.MockMarkTypesRepository
	cache *usecase.MockDictionaryCache
	trm   *usecase.MockManager
	uc    *usecase.MarkTypes
}

func TestMarkTypes(t *testing.T) {
	suite.Run(t, new(MarkTypesSuite))
}

func (suite *MarkTypesSuite) SetupTest() {
	suite.repo = usecase.NewMockMarkTypesRepository(suite.T())
	suite.cache = usecase.NewMockDictionaryCache(suite.T())
	suite.trm = usecase.NewMockManager(suite.T())
	suite.uc = usecase.NewMarkTypes(slogdiscard.NewDiscardLogger(), suite.trm, suite.repo).
		WithCache(suite.cache, typesCachePrefix)
}

func (suite *MarkTypesSuite) inTransaction() {
	suite.trm.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
}

func validCreate() models.MarkTypeCreate {
	return models.MarkTypeCreate{Code: "potholes", NameRU: "Ямы", NameEN: "Potholes", Icon: null.StringFrom("pit"), Color: null.StringFrom("#ff8800"), SLAHours: 48}
}

func (suite *MarkTypesSuite) TestList() {
	want := []models.MarkType{{ID: 1, Code: "garbage", Active: false}, {ID: 2, Code: "roads", Active: true}}
	suite.repo.On("GetAllMarkTypes", mock.Anything, models.LangEN).Once().Return(want, nil)
	got, err := suite.uc.List(context.Background(), models.LangEN)
	suite.NoError(err)
	suite.Equal(want, got)

	suite.repo.On("GetAllMarkTypes", mock.Anything, models.LangRU).Once().Return(nil, errRepo)
	_, err = suite.uc.List(context.Background(), models.LangRU)
	suite.ErrorIs(err, errRepo)
}

func (suite *MarkTypesSuite) TestCreate() {
	tests := []struct {
		name    string
		input   func(t *models.MarkTypeCreate)
		addErr  error
		wantErr error
	}{
		{name: "ok"},
		{name: "ok without optional fields", input: func(t *models.MarkTypeCreate) { t.NameEN, t.Icon, t.Color = "", null.String{}, null.String{} }},
		{name: "bad code", input: func(t *models.MarkTypeCreate) { t.Code = "Bad Code" }, wantErr: usecase.ErrInvalidArgument},
		{name: "empty name", input: func(t *models.MarkTypeCreate) { t.NameRU = "" }, wantErr: usecase.ErrInvalidArgument},
		{name: "long name", input: func(t *models.MarkTypeCreate) { t.NameRU = string(make([]rune, 41)) }, wantErr: usecase.ErrInvalidArgument},
		{name: "bad color", input: func(t *models.MarkTypeCreate) { t.Color = null.StringFrom("orange") }, wantErr: usecase.ErrInvalidArgument},
		{name: "bad sla", input: func(t *models.MarkTypeCreate) { t.SLAHours = 0 }, wantErr: usecase.ErrInvalidArgument},
		{name: "code taken", addErr: repository.ErrExists, wantErr: usecase.ErrConflict},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			in := validCreate()
			if tt.input != nil {
				tt.input(&in)
			}
			if tt.wantErr == nil || tt.addErr != nil {
				suite.inTransaction()
				suite.repo.On("AddMarkType", mock.Anything, in).Once().Return(int64(7), tt.addErr)
			}
			if tt.wantErr == nil {
				suite.cache.On("DeleteByPrefix", mock.Anything, typesCachePrefix).Once().Return(nil)
				suite.repo.On("GetMarkTypeById", mock.Anything, 7, models.LangRU).Once().Return(models.MarkType{ID: 7, Code: in.Code, Active: true}, nil)
			}

			got, err := suite.uc.Create(context.Background(), in, models.LangRU)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(7, got.ID)
		})
	}
	suite.cache.AssertExpectations(suite.T())
}

func (suite *MarkTypesSuite) TestUpdate() {
	name := "Дороги"
	empty := ""
	badColor := "red"
	goodColor := "#00AAff"
	active := false
	order := 5
	negative := -1

	tests := []struct {
		name      string
		upd       models.MarkTypeUpdate
		updateErr error
		cacheErr  error
		wantErr   error
	}{
		{name: "ok", upd: models.MarkTypeUpdate{NameRU: &name, Color: &goodColor, Active: &active, SortOrder: &order}},
		{name: "clearing icon and color is allowed", upd: models.MarkTypeUpdate{Icon: &empty, Color: &empty}},
		{name: "cache failure is not an error", upd: models.MarkTypeUpdate{Active: &active}, cacheErr: errRepo},
		{name: "empty update", upd: models.MarkTypeUpdate{}, wantErr: usecase.ErrInvalidArgument},
		{name: "bad color", upd: models.MarkTypeUpdate{Color: &badColor}, wantErr: usecase.ErrInvalidArgument},
		{name: "negative sort order", upd: models.MarkTypeUpdate{SortOrder: &negative}, wantErr: usecase.ErrInvalidArgument},
		{name: "not found", upd: models.MarkTypeUpdate{Active: &active}, updateErr: repository.ErrNotFound, wantErr: usecase.ErrNotFound},
		{name: "code taken", upd: models.MarkTypeUpdate{Code: &name}, wantErr: usecase.ErrInvalidArgument},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantErr == nil || tt.updateErr != nil {
				suite.inTransaction()
				suite.repo.On("UpdateMarkType", mock.Anything, 3, tt.upd).Once().Return(tt.updateErr)
			}
			if tt.wantErr == nil {
				suite.cache.On("DeleteByPrefix", mock.Anything, typesCachePrefix).Once().Return(tt.cacheErr)
				suite.repo.On("GetMarkTypeById", mock.Anything, 3, models.LangEN).Once().Return(models.MarkType{ID: 3}, nil)
			}

			got, err := suite.uc.Update(context.Background(), 3, tt.upd, models.LangEN)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(3, got.ID)
		})
	}
}

func (suite *MarkTypesSuite) TestWithoutCache() {
	uc := usecase.NewMarkTypes(slogdiscard.NewDiscardLogger(), suite.trm, suite.repo)
	suite.inTransaction()
	suite.repo.On("AddMarkType", mock.Anything, validCreate()).Once().Return(int64(1), nil)
	suite.repo.On("GetMarkTypeById", mock.Anything, 1, models.LangRU).Once().Return(models.MarkType{ID: 1}, nil)

	_, err := uc.Create(context.Background(), validCreate(), models.LangRU)
	suite.NoError(err)
	suite.cache.AssertNotCalled(suite.T(), "DeleteByPrefix", mock.Anything, mock.Anything)
}
