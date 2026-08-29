//go:build integration

package postgres_test

import (
	"encoding/json"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/guregu/null/v6"
)

const settingsKey = "runtime"

func (s *PostgresSuite) TestSettings_GetSetting_NotFound() {
	_, err := s.settings.GetSetting(s.ctx, settingsKey)
	s.ErrorIs(err, repository.ErrNotFound)
}

func (s *PostgresSuite) TestSettings_SetGetAndHistory() {
	first := json.RawMessage(`{"vote_threshold": 3}`)
	second := json.RawMessage(`{"vote_threshold": 4}`)

	// First write: inserted, recorded with old = NULL.
	s.Require().NoError(s.settings.SetSetting(s.ctx, settingsKey, first, null.IntFrom(fxUserAlice)))
	got, err := s.settings.GetSetting(s.ctx, settingsKey)
	s.Require().NoError(err)
	s.Equal(settingsKey, got.Key)
	s.JSONEq(string(first), string(got.Value))
	s.Equal(null.IntFrom(fxUserAlice), got.UpdatedBy)
	s.False(got.UpdatedAt.IsZero())

	// Same value again: the row is touched but no history row is added.
	s.Require().NoError(s.settings.SetSetting(s.ctx, settingsKey, first, null.IntFrom(fxUserBob)))

	// A change: updated in place, recorded with old = first.
	s.Require().NoError(s.settings.SetSetting(s.ctx, settingsKey, second, null.Int{}))
	got, err = s.settings.GetSetting(s.ctx, settingsKey)
	s.Require().NoError(err)
	s.JSONEq(string(second), string(got.Value))
	s.False(got.UpdatedBy.Valid)

	history, err := s.settings.GetSettingsHistory(s.ctx, settingsKey, 10)
	s.Require().NoError(err)
	s.Require().Len(history, 2, "one row per value change, newest first")

	s.Require().NotNil(history[0].Old)
	s.JSONEq(string(first), string(*history[0].Old))
	s.JSONEq(string(second), string(history[0].New))
	s.False(history[0].UpdatedBy.Valid)

	s.Nil(history[1].Old)
	s.JSONEq(string(first), string(history[1].New))
	s.Equal(null.IntFrom(fxUserAlice), history[1].UpdatedBy)
	s.True(history[0].ID > history[1].ID)

	// Limit and key filtering.
	limited, err := s.settings.GetSettingsHistory(s.ctx, settingsKey, 1)
	s.Require().NoError(err)
	s.Require().Len(limited, 1)
	s.Equal(history[0].ID, limited[0].ID)

	other, err := s.settings.GetSettingsHistory(s.ctx, "other", 10)
	s.Require().NoError(err)
	s.Empty(other)
}

func (s *PostgresSuite) TestSettings_DeletedAuthorKeepsRow() {
	// A user without marks, so that the delete is not blocked by marks.fk_user.
	var adminId int64
	s.Require().NoError(s.db.GetContext(s.ctx, &adminId,
		`INSERT INTO users (name, login, password_hash, rating, role) VALUES ('Admin', 'admin', 'hash', 0, 'admin') RETURNING user_id`))
	s.Require().NoError(s.settings.SetSetting(s.ctx, settingsKey, json.RawMessage(`{"a":1}`), null.IntFrom(adminId)))
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM users WHERE user_id = $1`, adminId)
	s.Require().NoError(err)

	got, err := s.settings.GetSetting(s.ctx, settingsKey)
	s.Require().NoError(err)
	s.False(got.UpdatedBy.Valid, "updated_by is set to NULL when the user is deleted")
}

func (s *PostgresSuite) TestMarkTypes_AdminCRUD() {
	created, err := s.marks.AddMarkType(s.ctx, models.MarkTypeCreate{
		Code: "potholes", NameRU: "Ямы", NameEN: "Potholes",
		Icon: null.StringFrom("pit"), Color: null.StringFrom("#ff8800"), SLAHours: 48,
	})
	s.Require().NoError(err)
	s.Equal(int64(5), created)

	// Duplicate code.
	_, err = s.marks.AddMarkType(s.ctx, models.MarkTypeCreate{Code: "potholes", NameRU: "Ямы", SLAHours: 1})
	s.ErrorIs(err, repository.ErrExists)

	t, err := s.marks.GetMarkTypeById(s.ctx, 5, models.LangEN)
	s.Require().NoError(err)
	s.Equal(models.MarkType{ID: 5, Code: "potholes", Name: "Potholes", SLAHours: 48,
		Icon: null.StringFrom("pit"), Color: null.StringFrom("#ff8800"), Active: true, SortOrder: 0}, t)

	t, err = s.marks.GetMarkTypeById(s.ctx, 5, models.LangRU)
	s.Require().NoError(err)
	s.Equal("Ямы", t.Name)

	_, err = s.marks.GetMarkTypeById(s.ctx, 999, models.LangRU)
	s.ErrorIs(err, repository.ErrNotFound)

	// Partial update: rename in English, clear the icon, deactivate, move first.
	nameEN, empty, active, order, sla := "Road holes", "", false, -1, 12
	s.Require().NoError(s.marks.UpdateMarkType(s.ctx, 5, models.MarkTypeUpdate{
		NameEN: &nameEN, Icon: &empty, Active: &active, SortOrder: &order, SLAHours: &sla,
	}))
	t, err = s.marks.GetMarkTypeById(s.ctx, 5, models.LangEN)
	s.Require().NoError(err)
	s.Equal("Road holes", t.Name)
	s.False(t.Icon.Valid, "empty icon clears the column")
	s.Equal(null.StringFrom("#ff8800"), t.Color, "untouched fields stay")
	s.False(t.Active)
	s.Equal(-1, t.SortOrder)
	s.Equal(12, t.SLAHours)

	// Renaming in Russian updates both the raw column and the translation.
	nameRU := "Дорожные ямы"
	s.Require().NoError(s.marks.UpdateMarkType(s.ctx, 5, models.MarkTypeUpdate{NameRU: &nameRU}))
	var raw string
	s.Require().NoError(s.db.GetContext(s.ctx, &raw, `SELECT name FROM types_marks WHERE type_mark_id = 5`))
	s.Equal(nameRU, raw)
	t, err = s.marks.GetMarkTypeById(s.ctx, 5, models.LangRU)
	s.Require().NoError(err)
	s.Equal(nameRU, t.Name)

	// Taken code, unknown id.
	taken := "garbage"
	s.ErrorIs(s.marks.UpdateMarkType(s.ctx, 5, models.MarkTypeUpdate{Code: &taken}), repository.ErrExists)
	s.ErrorIs(s.marks.UpdateMarkType(s.ctx, 999, models.MarkTypeUpdate{Active: &active}), repository.ErrNotFound)
}

func (s *PostgresSuite) TestMarkTypes_ActiveAndSortOrder() {
	// Type 3 (lighting) goes inactive, type 4 gets sort_order 0 but type 2
	// is promoted to the top with a negative order.
	inactive, top := false, -10
	s.Require().NoError(s.marks.UpdateMarkType(s.ctx, 3, models.MarkTypeUpdate{Active: &inactive}))
	s.Require().NoError(s.marks.UpdateMarkType(s.ctx, 2, models.MarkTypeUpdate{SortOrder: &top}))

	codes := func(types []models.MarkType) []string {
		out := make([]string, 0, len(types))
		for _, t := range types {
			out = append(out, t.Code)
		}
		return out
	}

	public, err := s.marks.GetMarkTypes(s.ctx, models.LangRU)
	s.Require().NoError(err)
	// green_zones first by sort_order, then the active rest by name
	// ("Информационные..." < "Мусор"); lighting is hidden.
	s.Equal([]string{"green_zones", "visual_defects", "garbage"}, codes(public))
	for _, t := range public {
		s.True(t.Active)
	}

	all, err := s.marks.GetAllMarkTypes(s.ctx, models.LangRU)
	s.Require().NoError(err)
	s.Equal([]string{"green_zones", "visual_defects", "garbage", "lighting"}, codes(all))
	s.False(all[3].Active)
}
