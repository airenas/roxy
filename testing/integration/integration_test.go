//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type config struct {
	uploadURL  string
	dbURL      string
	httpclient *http.Client
}

var cfg config

func TestMain(m *testing.M) {
	cfg.uploadURL = GetEnvOrFail("UPLOAD_URL")
	cfg.dbURL = GetEnvOrFail("DB_URL")
	cfg.httpclient = &http.Client{Timeout: time.Second * 20}

	tCtx, cf := context.WithTimeout(context.Background(), time.Second*20)
	defer cf()
	WaitForOpenOrFail(tCtx, cfg.dbURL)
	WaitForOpenOrFail(tCtx, cfg.uploadURL)
	waitForDB(tCtx, cfg.dbURL)

	os.Exit(m.Run())
}

func TestLive(t *testing.T) {
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
