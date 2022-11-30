//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type config struct {
	uploadURL  string
	statusURL  string
	dbURL      string
	httpclient *http.Client
}

var cfg config

func TestMain(m *testing.M) {
	cfg.uploadURL = GetEnvOrFail("UPLOAD_URL")
	cfg.statusURL = GetEnvOrFail("STATUS_URL")
	cfg.dbURL = GetEnvOrFail("DB_URL")
	cfg.httpclient = &http.Client{Timeout: time.Second * 30}

	tCtx, cf := context.WithTimeout(context.Background(), time.Second*20)
	defer cf()
	WaitForOpenOrFail(tCtx, cfg.dbURL)
	WaitForOpenOrFail(tCtx, cfg.uploadURL)
	WaitForOpenOrFail(tCtx, cfg.statusURL)
	waitForDB(tCtx, cfg.dbURL)

	//start mocks service for private services - not in this docker compose
	l, ts := startMockService(9876)
	defer ts.Close()
	defer l.Close()

	os.Exit(m.Run())
}

func TestUploadLive(t *testing.T) {
	t.Parallel()
	CheckCode(t, Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.uploadURL, "/live", nil)), http.StatusOK)
}

func TestUpload(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	CheckCode(t, Invoke(t, cfg.httpclient, req), http.StatusOK)
}

func TestUpload_SeveralFiles(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav", "audio2.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	CheckCode(t, Invoke(t, cfg.httpclient, req), http.StatusOK)
}

func TestUpload_Fail_NoFile(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	CheckCode(t, Invoke(t, cfg.httpclient, req), http.StatusBadRequest)
}

func TestStatusLive(t *testing.T) {
	t.Parallel()
	CheckCode(t, Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.statusURL, "/live", nil)), http.StatusOK)
}

func TestStatus_Check_None(t *testing.T) {
	t.Parallel()
	st := getStatus(t, "10")
	assert.Equal(t, "NOT_FOUND", st.Error)
	assert.Equal(t, "10", st.ID)
}

type uploadResponse struct {
	ID string `json:"id"`
}

func getStatus(t *testing.T, id string) api.StatusData {
	resp := Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.statusURL, "status/"+id, nil))
	CheckCode(t, resp, http.StatusOK)
	var st api.StatusData
	Decode(t, resp, &st)
	return st
}

func TestStatus_Check(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	resp := Invoke(t, cfg.httpclient, req)
	CheckCode(t, resp, http.StatusOK)
	var ur uploadResponse
	Decode(t, resp, &ur)
	assert.NotEmpty(t, ur.ID)
	st := getStatus(t, ur.ID)
	assert.NotEqual(t, "NOT_FOUND", st.Status)
	dur := time.Second * 10
	tm := time.After(dur)
	for {
		select {
		case <-tm:
			require.Failf(t, "Fail", "Not COMPLETED in %v", dur)
		default:
			st = getStatus(t, ur.ID)
			if st.Status == "COMPLETED" {
				return
			}
			time.Sleep(time.Second)
		}
	}
}

func newUploadRequest(t *testing.T, files []string, params [][2]string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for i, f := range files {
		n := "file"
		if i > 0 {
			n = fmt.Sprintf("file%d", i+1)
		}
		part, _ := writer.CreateFormFile(n, f)
		_, _ = io.Copy(part, strings.NewReader(f))
	}
	for _, p := range params {
		writer.WriteField(p[0], p[1])
	}
	writer.Close()
	req, err := http.NewRequest(http.MethodPost, cfg.uploadURL+"/upload", body)
	require.Nil(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-doorman-requestid", "m:testRequestID")
	return req
}

func startMockService(port int) (net.Listener, *httptest.Server) {
	// create a listener with the desired port.
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("can't start mock service: %v", err)
	}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// log.Printf("request to: " + r.URL.String())
		switch r.URL.String() {
		case "/ausis/transcriber/upload":
			io.Copy(w, strings.NewReader(`{"id":"1111"}`))
		case "/ausis/status.service/subscribe":
			handleStatusWS(w, r, func() string {
				return `{"status":"COMPLETED", "audioReady":true, "avResults":["lat.txt", "res.txt"]}`
			})
		case "/ausis/result.service/result/1111/lat.txt":
			w.Header().Add("content-disposition", `attachment; filename="lat123.txt"`)
			io.Copy(w, strings.NewReader(`file content`))
		case "/ausis/result.service/result/1111/res.txt":
			w.Header().Add("content-disposition", `attachment; filename="lat123res.txt"`)
			io.Copy(w, strings.NewReader(`file content`))
		case "/ausis/clean.service/1111":
			io.Copy(w, strings.NewReader(`OK`))
		default:
			log.Printf("Unknown request to: " + r.URL.String())
		}
	}))

	ts.Listener.Close()
	ts.Listener = l

	// Start the server.
	ts.Start()
	log.Printf("started mock srv on port: %d", port)
	return l, ts
}
