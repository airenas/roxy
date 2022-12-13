package utils

import (
	"database/sql"
	"reflect"
	"testing"
)

func TestToSQLStr(t *testing.T) {
	tests := []struct {
		name string
		args string
		want sql.NullString
	}{
		{name: "empty", args: "", want: sql.NullString{}},
		{name: "non empty", args: "olia", want: sql.NullString{String: "olia", Valid: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToSQLStr(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToSQLStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromSQLStr(t *testing.T) {
	tests := []struct {
		name string
		args sql.NullString
		want string
	}{
		{name: "empty", args: sql.NullString{}, want: ""},
		{name: "non empty", args: sql.NullString{String: "olia", Valid: true}, want: "olia"},
		{name: "non valid", args: sql.NullString{String: "olia", Valid: false}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromSQLStr(tt.args); got != tt.want {
				t.Errorf("FromSQLStr() = %v, want %v", got, tt.want)
			}
		})
	}
}
