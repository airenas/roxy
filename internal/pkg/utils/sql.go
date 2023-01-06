package utils

import "database/sql"

// ToSQLStr creates new sql str instance
func ToSQLStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// FromSQLStr returns string from sql.NullString
func FromSQLStr(sqlStr sql.NullString) string {
	if sqlStr.Valid {
		return sqlStr.String
	}
	return ""
}

// ToSQLInt32 creates new sql int instance
func ToSQLInt32(i int32) sql.NullInt32 {
	return sql.NullInt32{Int32: i, Valid: true}
}

// FromSQLInt32OrZero returns int from sql.NullInt32
func FromSQLInt32OrZero(sqlData sql.NullInt32) int32 {
	if sqlData.Valid {
		return sqlData.Int32
	}
	return 0
}
