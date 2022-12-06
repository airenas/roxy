package test

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func CheckCode(t *testing.T, resp *http.Response, expected int) *http.Response {
	t.Helper()
	if resp.StatusCode != expected {
		b, _ := ioutil.ReadAll(resp.Body)
		require.Equal(t, expected, resp.StatusCode, string(b))
	}
	return resp
}

func Decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var res T
	require.Nil(t, json.NewDecoder(resp.Body).Decode(&res))
	return res
}
