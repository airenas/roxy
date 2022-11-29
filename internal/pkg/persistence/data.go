package persistence

import "time"

type (

	//ReqData table
	ReqData struct {
		ID        string
		FileCount int
		Filename  string
		Created   time.Time
		Email     string
		Params    map[string]string
		RequestID string
	}

	//ReqData table
	WorkData struct {
		ID         string
		ExternalID string
		Created    time.Time
	}

	//Status information table
	Status struct {
		ID               string
		Status           string
		Error            string
		ErrorCode        string
		AudioReady       bool
		AvailableResults []string
		Created          time.Time
		Updated          time.Time
	}
)
