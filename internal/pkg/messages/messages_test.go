package messages

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMessageFrom(t *testing.T) {
	assert.Equal(t, &ASRMessage{RequestID: "rID"},
		NewMessageFrom(&ASRMessage{RequestID: "rID"}))
}
