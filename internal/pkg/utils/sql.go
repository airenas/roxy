package utils

import "database/sql"

// ToSQLStr create snew sql str instance
func ToSQLStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// FromSQLStr return string from sql.NullString
func FromSQLStr(sqlStr sql.NullString) string {
	if sqlStr.Valid {
		return sqlStr.String
	}
	return ""
}
