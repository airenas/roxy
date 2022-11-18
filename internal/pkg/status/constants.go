package status

//Status represents asr status
type Status int

const (
	// Uploaded value
	Uploaded Status = iota + 1
	// Working step
	Working
	// Completed - final step
	Completed
)

var (
	statusName = map[Status]string{Uploaded: "UPLOADED", Completed: "COMPLETED",
		Working: "Working"}
	nameStatus = map[string]Status{"UPLOADED": Uploaded, "COMPLETED": Completed,
		"Working": Working}
)

func (st Status) String() string {
	return statusName[st]
}

// From returns status obj from string
func From(st string) Status {
	return nameStatus[st]
}
