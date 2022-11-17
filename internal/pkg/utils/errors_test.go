package utils

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrNonRestorableUsage_Error(t *testing.T) {
	assert.Equal(t, "non restorable usage error: olia", NewErrNonRestorableUsage(errors.New("olia")).Error())
}

func TestErrNonRestorableUsage_Unwrap(t *testing.T) {
	assert.True(t, errors.Is(NewErrNonRestorableUsage(io.EOF), io.EOF))
}
