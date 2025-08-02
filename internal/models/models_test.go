package models

import (
	"encoding/json"
	"testing"
)

func TestMark_UnmarshalJSON(t *testing.T) {
	var mark Mark
	data := []byte(`{"geom":{"type":"Point","coordinates":[41.402893,52.700111]},"mark_id":1,"name":"Свалка","type_mark_id":1,"user_id":1,"district_id":2,"number_votes":0,"number_checks":0}`)
	if err := json.Unmarshal(data, &mark); err != nil {
		t.Errorf("Mark UnmarshalJSON() error = %v", err)
	}
}
