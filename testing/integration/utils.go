//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func WaitForOpenOrFail(ctx context.Context, URL string) {
	u, err := url.Parse(URL)
	if err != nil {
		log.Fatalf("FAIL: can't parse %s", URL)
	}
	for {
		err = listen(net.JoinHostPort(u.Hostname(), u.Port()))
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			log.Fatalf("FAIL: can't access %s", URL)
			break
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func GetEnvOrFail(s string) string {
	res := os.Getenv(s)
	if res == "" {
		log.Fatalf("no env '%s'", s)
	}
	return res
}

func listen(urlStr string) error {
	log.Printf("dial %s", urlStr)
	conn, err := net.DialTimeout("tcp", urlStr, time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}

func NewRequest(t *testing.T, method string, srv, urlSuffix string, body interface{}) *http.Request {
	t.Helper()
	path, _ := url.JoinPath(srv, urlSuffix)
	req, err := http.NewRequest(method, path, ToReader(body))
	require.Nil(t, err, "not nil error = %v", err)
	if body != nil {
		req.Header.Add(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	return req
}

func ToReader(data interface{}) io.Reader {
	bytes, _ := json.Marshal(data)
	return strings.NewReader(string(bytes))
}

func Invoke(t *testing.T, cl *http.Client, r *http.Request) *http.Response {
	t.Helper()
	resp, err := cl.Do(r)
	require.Nil(t, err, "not nil error = %v", err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func CheckCode(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		b, _ := ioutil.ReadAll(resp.Body)
		require.Equal(t, expected, resp.StatusCode, string(b))
	}
}

func Decode(t *testing.T, resp *http.Response, to interface{}) {
	t.Helper()
	require.Nil(t, json.NewDecoder(resp.Body).Decode(to))
}

func waitForDB(ctx context.Context, URL string) {
	dbPool, err := pgxpool.New(ctx, URL)
	if err != nil {
		log.Fatalf("FAIL: can't init db pool")
	}
	defer dbPool.Close()

	for {
		log.Printf("check db live ...")
		db, err := postgres.NewDB(dbPool)
		if err == nil {
			if err = db.Live(ctx); err == nil {
				return
			}
			log.Printf(err.Error())
		}
		select {
		case <-ctx.Done():
			log.Fatalf("FAIL: can't access db")
			break
		case <-time.After(500 * time.Millisecond):
		}
	}
}
