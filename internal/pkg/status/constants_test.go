package status

import (
	"testing"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		name string
		st   Status
		want string
	}{
		{st: Uploaded, want: "UPLOADED"},
		{st: Completed, want: "COMPLETED"},
		{st: Working, want: "Working"},
		{st: ServiceError, want: "SERVICE_ERROR"},
		{st: NotFound, want: "NOT_FOUND"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.st.String(); got != tt.want {
				t.Errorf("Status.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFrom(t *testing.T) {
	tests := []struct {
		name string
		args string
		want Status
	}{
		{args: "COMPLETED", want: Completed},
		{args: "olia", want: 0},
		{args: "Working", want: Working},
		{args: "UPLOADED", want: Uploaded},
		{args: "SERVICE_ERROR", want: ServiceError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := From(tt.args); got != tt.want {
				t.Errorf("From() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrCodes_String(t *testing.T) {
	tests := []struct {
		name string
		st   ErrCode
		want string
	}{
		{st: ECServiceError, want: "SERVICE_ERROR"},
		{st: ECNotFound, want: "NOT_FOUND"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.st.String(); got != tt.want {
				t.Errorf("Status.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
