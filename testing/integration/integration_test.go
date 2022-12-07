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

	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/transcriber"
	"github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latFileContent   = "lat file content 1"
	resFileContent   = "res file content 1"
	audioFileContent = "audio.wav content"
)

type config struct {
	uploadURL          string
	statusURL          string
	resultURL          string
	statusSubscribeURL string
	dbURL              string
	httpclient         *http.Client
}

var cfg config

func TestMain(m *testing.M) {
	cfg.uploadURL = GetEnvOrFail("UPLOAD_URL")
	cfg.statusURL = GetEnvOrFail("STATUS_URL")
	cfg.resultURL = GetEnvOrFail("RESULT_URL")
	cfg.statusSubscribeURL = GetEnvOrFail("STATUS_SUBSCRIBE_URL")
	cfg.dbURL = GetEnvOrFail("DB_URL")
	cfg.httpclient = &http.Client{Timeout: time.Second * 30}

	tCtx, cf := context.WithTimeout(context.Background(), time.Second*20)
	defer cf()
	WaitForOpenOrFail(tCtx, cfg.dbURL)
	WaitForOpenOrFail(tCtx, cfg.uploadURL)
	WaitForOpenOrFail(tCtx, cfg.statusURL)
	WaitForOpenOrFail(tCtx, cfg.resultURL)
	waitForDB(tCtx, cfg.dbURL)

	//start mocks service for private services - not in this docker compose
	l, ts := startMockService(9876)
	defer ts.Close()
	defer l.Close()

	os.Exit(m.Run())
}

func TestUploadLive(t *testing.T) {
	t.Parallel()
	test.CheckCode(t, test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.uploadURL, "/live", nil)), http.StatusOK)
}

func TestUpload(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	test.CheckCode(t, test.Invoke(t, cfg.httpclient, req), http.StatusOK)
}

func TestUpload_SeveralFiles(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav", "audio2.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	test.CheckCode(t, test.Invoke(t, cfg.httpclient, req), http.StatusOK)
}

func TestUpload_Fail_NoFile(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	test.CheckCode(t, test.Invoke(t, cfg.httpclient, req), http.StatusBadRequest)
}

func TestStatusLive(t *testing.T) {
	t.Parallel()
	test.CheckCode(t, test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.statusURL, "/live", nil)), http.StatusOK)
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
	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.statusURL, "status/"+id, nil))
	test.CheckCode(t, resp, http.StatusOK)
	return test.Decode[api.StatusData](t, resp)
}

func TestStatus_Check(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[api.StatusData](t, resp)
	assert.NotEmpty(t, ur.ID)
	st := getStatus(t, ur.ID)
	assert.NotEqual(t, "NOT_FOUND", st.Status)
	testWaitCompleted(t, ur.ID)
}

func testWaitCompleted(t *testing.T, id string) {
	dur := time.Second * 60
	tm := time.After(dur)
	for {
		select {
		case <-tm:
			require.Failf(t, "Fail", "Not COMPLETED in %v", dur)
		default:
			st := getStatus(t, id)
			if st.Status == "COMPLETED" {
				return
			}
			time.Sleep(time.Second)
		}
	}
}

func TestStatus_Subscribe(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	var ur = test.Decode[uploadResponse](t, resp)
	assert.NotEmpty(t, ur.ID)
	st := getStatus(t, ur.ID)
	assert.NotEqual(t, "NOT_FOUND", st.Status)
	dur := time.Second * 60
	tm := time.After(dur)
	r, cf := getStatusSubscribe(t, ur.ID)
	defer cf()
	for {
		select {
		case <-tm:
			require.Failf(t, "Fail", "Not COMPLETED in %v", dur)
		case st, cl := <-r:
			require.Truef(t, cl, "Closed subscribe channel")
			log.Printf("got status: %s", st.Status)
			if st.Status == "COMPLETED" {
				return
			}
		}
	}
}

func TestResultLive(t *testing.T) {
	t.Parallel()
	test.CheckCode(t, test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, "/live", nil)), http.StatusOK)
}

func TestResult_NoFile(t *testing.T) {
	t.Parallel()
	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, "result/1xx/some.txt", nil))
	test.CheckCode(t, resp, http.StatusNotFound)
}

func TestResult_GetFile(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)

	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, fmt.Sprintf("result/%s/lat.txt", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	assert.Equal(t, "attachment; filename=lat.txt", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, latFileContent, takeStr(resp))
}

func TestResult_GetFile2(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)

	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, fmt.Sprintf("result/%s/res.txt", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	assert.Equal(t, "attachment; filename=res.txt", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, resFileContent, takeStr(resp))
}

func takeStr(resp *http.Response) string {
	var b bytes.Buffer
	_, _ = b.ReadFrom(resp.Body)
	return b.String()
}

func TestResult_Head(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)
	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodHead, cfg.resultURL, fmt.Sprintf("result/%s/lat.txt", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	assert.Equal(t, "attachment; filename=lat.txt", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, "", test.RStr(t, resp.Body))
}

func TestResult_Audio(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)

	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, fmt.Sprintf("audio/%s", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	assert.Equal(t, "attachment; filename=lat.txt", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, audioFileContent, takeStr(resp))
}

func TestResult_AudioHead(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)

	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodHead, cfg.resultURL, fmt.Sprintf("audio/%s", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	assert.Equal(t, "attachment; filename=lat.txt", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, "audio.wav content", takeStr(resp))
}

func uploadWaitFakeFile(t *testing.T) string {
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[api.StatusData](t, resp)

	testWaitCompleted(t, ur.ID)
	return ur.ID
}

func getStatusSubscribe(t *testing.T, id string) (<-chan api.StatusData, func()) {
	t.Helper()
	client, _ := transcriber.NewClient(cfg.uploadURL, cfg.statusSubscribeURL, cfg.uploadURL, cfg.uploadURL)
	r, ccf, err := client.HookToStatus(test.Ctx(t), id)
	require.Nil(t, err)
	return r, ccf
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
		_, _ = io.Copy(part, strings.NewReader(audioFileContent))
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
			handleStatusWS(w, r, func(c *websocket.Conn) {
				mt, _, err := c.ReadMessage()
				if err != nil {
					log.Print(err)
					return
				}
				err = c.WriteMessage(mt, []byte(`{"status":"Decode", "audioReady":true, "avResults":["lat.txt", "res.txt"]}`))
				if err != nil {
					log.Print(err)
					return
				}
				time.Sleep(1 * time.Second)
				err = c.WriteMessage(mt, []byte(`{"status":"COMPLETED", "audioReady":true, "avResults":["lat.txt", "res.txt"]}`))
				if err != nil {
					log.Print(err)
					return
				}
			})
		case "/ausis/result.service/result/1111/lat.txt":
			w.Header().Add("content-disposition", `attachment; filename="lat.txt"`)
			io.Copy(w, strings.NewReader(latFileContent))
		case "/ausis/result.service/result/1111/res.txt":
			w.Header().Add("content-disposition", `attachment; filename="res.txt"`)
			io.Copy(w, strings.NewReader(resFileContent))
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
