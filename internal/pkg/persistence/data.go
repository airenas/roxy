package persistence

import (
	"database/sql"
	"time"
)

type (

	//ReqData table
	ReqData struct {
		ID        string
		FileCount int
		FileName  sql.NullString // file name for audio download
		FileNames []string       // file names for upload
		Email     sql.NullString
		Params    map[string]string
		RequestID string

		Created time.Time
	}

	//WorkData table
	WorkData struct {
		ID          string
		ExternalID  string
		Transcriber sql.NullString
		TryCount    int

		Version int
		Created time.Time
		Updated time.Time
	}

	//Status information table
	Status struct {
		ID               string
		Status           string
		Error            sql.NullString
		ErrorCode        sql.NullString
		AudioReady       bool
		Progress         sql.NullInt32
		AvailableResults []string
		RecognizedText   sql.NullString

		Version int
		Created time.Time
		Updated time.Time
	}
)
