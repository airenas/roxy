package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
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

func Code(t *testing.T, tEcho *echo.Echo, req *http.Request, code int) *httptest.ResponseRecorder {
	t.Helper()
	tResp := httptest.NewRecorder()
	tEcho.ServeHTTP(tResp, req)
	require.Equal(t, code, tResp.Code)
	return tResp
}

func RStr(t *testing.T, r io.Reader) string {
	t.Helper()
	var b bytes.Buffer
	_, err := b.ReadFrom(r)
	require.Nil(t, err)
	return b.String()
}
