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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := From(tt.args); got != tt.want {
				t.Errorf("From() = %v, want %v", got, tt.want)
			}
		})
	}
}
