package status

// Status represents asr status
type Status int

const (
	// Uploaded value
	Uploaded Status = iota + 1
	// Working step
	Working
	// Completed - final step
	Completed
	// ServiceError - indicates service error
	ServiceError
	// ServiceError - indicates service error
	NotFound
)

var (
	statusName = map[Status]string{Uploaded: "UPLOADED", Completed: "COMPLETED",
		Working: "Working", ServiceError: "SERVICE_ERROR", NotFound: "NOT_FOUND"}
	nameStatus = map[string]Status{"UPLOADED": Uploaded, "COMPLETED": Completed,
		"Working": Working, "SERVICE_ERROR": ServiceError, "NOT_FOUND": NotFound}
)

func (st Status) String() string {
	return statusName[st]
}

// From returns status obj from string
func From(st string) Status {
	return nameStatus[st]
}

// ErrCode represents asr err codes
type ErrCode int

const (
	// ServiceError - indicates service error
	ECServiceError ErrCode = iota + 1
	// ServiceError - indicates service error
	ECNotFound
)

var (
	errCodes = map[ErrCode]string{ECServiceError: "SERVICE_ERROR", ECNotFound: "NOT_FOUND"}
)

func (ec ErrCode) String() string {
	return errCodes[ec]
}
