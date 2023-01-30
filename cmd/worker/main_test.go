package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_defaultV(t *testing.T) {
	assert.Equal(t, "vd", defaultV("", "vd"))
	assert.Equal(t, "aaa", defaultV("aaa", "vd"))
	assert.Equal(t, 1, defaultV(0, 1))
	assert.Equal(t, 10, defaultV(10, 1))
	assert.Equal(t, time.Minute, defaultV(time.Duration(0), time.Minute))
	assert.Equal(t, time.Minute*5, defaultV(time.Minute*5, time.Minute))
}
