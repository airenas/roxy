package persistence

import "time"

type (

	//ReqData table
	ReqData struct {
		ID         string
		FileCount  int
		Filename   string
		AudioReady bool
		Created    time.Time
		Email      string
		Params     map[string]string
		RequestID  string
	}

	//Status information table
	Status struct {
		ID     string `bson:"ID"`
		Status string `bson:"status,omitempty"`
		Error  string `bson:"error,omitempty"`
	}
)
