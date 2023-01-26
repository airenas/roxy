//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/gorilla/websocket"
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
	if data == nil {
		return nil
	}
	bytes, _ := json.Marshal(data)
	return strings.NewReader(string(bytes))
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

const consulData = `
{
    "Datacenter": "dc1",
    "ID": "9f735e1d-5696-4cc2-a4d6-5040ad1c449f",
    "Node": "n1",
    "Address": "127.0.0.1",
    "TaggedAddresses": {
      "lan": "127.0.0.1",
      "wan": "127.0.0.1"
    },
    "NodeMeta": {
      "external-node": "true",
      "external-probe": "true"
    },
    "Service": {
      "ID": "asr1",
      "Service": "asr",
      "Tags": [
        "olia",
        ""
      ],
      "Meta": {
        "uploadURL": "ausis/transcriber/upload",
		"statusURL": "ausis/status.service",
		"resultURL": "ausis/result.service",
		"cleanURL": "ausis/clean.service"
      },
      "Address": "integration-tests",
      "Port": 9876
    },
    "Checks": [{
      "Node": "n1",
      "CheckID": "am:live",
      "Name": "AM Live check",
      "Notes": "",
      "ServiceID": "asr1",
      "Status": "passing",
      "Definition": {
        "HTTP": "http://127.0.0.1:8055/live",
        "Interval": "30s",
        "Timeout": "5s",
        "DeregisterCriticalServiceAfter": "1m",
        "Failures_before_critical": 3
      }
    }]
  }
`

func registerToConsul(ctx context.Context, URL string) {
	log.Printf("Registering: %s", URL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, URL, strings.NewReader(consulData))
	if err != nil {
		log.Fatalf("FAIL: can't register to consul: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("FAIL: can't register to consul: %v", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("FAIL: can't register to consul: %d, %s", resp.StatusCode, string(b))
	}
}

func handleStatusWS(rw http.ResponseWriter, req *http.Request, connf func(*websocket.Conn)) {
	upgrader := websocket.Upgrader{}
	c, err := upgrader.Upgrade(rw, req, nil)
	if err != nil {
		return
	}
	defer c.Close()
	connf(c)
}
