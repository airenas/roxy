package test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Invoke makes a request call
func Invoke(t *testing.T, cl *http.Client, r *http.Request) *http.Response {
	t.Helper()
	resp, err := cl.Do(r)
	require.Nil(t, err, "not nil error = %v", err)
	t.Cleanup(func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	})
	return resp
}

// CheckCode checks response code
func CheckCode(t *testing.T, resp *http.Response, expected int) *http.Response {
	t.Helper()
	if resp.StatusCode != expected {
		b, _ := io.ReadAll(resp.Body)
		require.Equal(t, expected, resp.StatusCode, string(b))
	}
	return resp
}

// Decode decodes response body to json type
func Decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var res T
	require.Nil(t, json.NewDecoder(resp.Body).Decode(&res))
	return res
}

func Ctx(t *testing.T) context.Context {
	t.Helper()
	ctx, cf := context.WithTimeout(context.Background(), time.Second*20)
	t.Cleanup(func() { cf() })
	return ctx
}
