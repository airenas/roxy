//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/transcriber"
	"github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jordan-wright/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latFileContent   = "lat file content 1"
	resFileContent   = "res file content 1"
	audioFileContent = "audio.wav content"
)

type config struct {
	uploadURL  string
	statusURL  string
	resultURL  string
	cleanURL   string
	dbURL      string
	httpclient *http.Client
}

type emails struct {
	mails []email.Email
	lock  *sync.RWMutex
}

type restores struct {
	ids  []string
	lock *sync.RWMutex
}

var cfg config
var emailData = emails{lock: &sync.RWMutex{}}
var restoresData = restores{lock: &sync.RWMutex{}}

func TestMain(m *testing.M) {
	cfg.uploadURL = GetEnvOrFail("UPLOAD_URL")
	cfg.statusURL = GetEnvOrFail("STATUS_URL")
	cfg.resultURL = GetEnvOrFail("RESULT_URL")
	cfg.cleanURL = GetEnvOrFail("CLEAN_URL")
	cfg.dbURL = GetEnvOrFail("DB_URL")
	cfg.httpclient = &http.Client{Timeout: time.Second * 30}

	tCtx, cf := context.WithTimeout(context.Background(), time.Second*20)
	defer cf()
	WaitForOpenOrFail(tCtx, cfg.dbURL)
	WaitForOpenOrFail(tCtx, cfg.uploadURL)
	WaitForOpenOrFail(tCtx, cfg.statusURL)
	WaitForOpenOrFail(tCtx, cfg.resultURL)
	WaitForOpenOrFail(tCtx, cfg.cleanURL)
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

func TestUpload_WorksWithoutEmail(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[api.StatusData](t, resp)
	testWaitStatus(t, ur.ID, "COMPLETED", false)
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
	assert.Equal(t, "Nežinomas ID: 10", st.Error)
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
	testWaitStatus(t, ur.ID, "COMPLETED", false)
}

func TestStatus_Text(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)
	st := getStatus(t, id)
	assert.Equal(t, "Olia olia", st.RecognizedText)
}

func testWaitStatus(t *testing.T, id, status string, fail bool) {
	dur := time.Second * 30
	tm := time.After(dur)
	failStr := "not failed"
	if fail {
		failStr = "failed"
	}
	for {
		select {
		case <-tm:
			require.Failf(t, "Fail", "Not %s(%s) in %v (ID:%s)", status, failStr, dur, id)
		default:
			st := getStatus(t, id)
			if st.Status == status && (!fail || st.Error != "") {
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

func TestStatus_Failure(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "failUpload"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[api.StatusData](t, resp)
	testWaitStatus(t, ur.ID, "UPLOADED", true)
}

func TestStatus_ExtFailure(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "failStatus"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[uploadResponse](t, resp)
	testWaitStatus(t, ur.ID, "Failure", true)
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
	assert.Equal(t, latFileContent, test.RStr(t, resp.Body))
}

func TestResult_GetFile2(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)

	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, fmt.Sprintf("result/%s/res.txt", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	assert.Equal(t, "attachment; filename=res.txt", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, resFileContent, test.RStr(t, resp.Body))
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

	assert.Equal(t, "attachment; filename=audio.wav", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, audioFileContent, test.RStr(t, resp.Body), "for id: "+id)
}

func TestResult_AudioHead(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)

	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodHead, cfg.resultURL, fmt.Sprintf("audio/%s", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	assert.Equal(t, "attachment; filename=audio.wav", resp.Header.Get("Content-Disposition"))
	assert.Equal(t, "", test.RStr(t, resp.Body))
}

func TestInform_Send(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)
	testEmailReceived(t, id, "Pradėta")
	testEmailReceived(t, id, "Baigta")
}

func TestInform_Failure(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "failUpload"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[uploadResponse](t, resp)
	testEmailReceived(t, ur.ID, "Pradėta")
	testEmailReceived(t, ur.ID, "Nepavyko")
}

func TestInform_ExtFailure(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "failStatus"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[uploadResponse](t, resp)
	testEmailReceived(t, ur.ID, "Pradėta")
	testEmailReceived(t, ur.ID, "Nepavyko")
}


func Test_RestoreReceived(t *testing.T) {
	t.Parallel()
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "failStatus"},
	{"numberOfSpeakers", "1"}})
	restoreID := "restore123"
	req.Header.Set("x-doorman-requestid", "asr:"+restoreID)
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	testRestoreReceived(t, restoreID)
}

func TestClean(t *testing.T) {
	t.Parallel()
	id := uploadWaitFakeFile(t)

	resp := test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, fmt.Sprintf("audio/%s", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	resp = test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodDelete, cfg.cleanURL, fmt.Sprintf("delete/%s", id), nil))
	test.CheckCode(t, resp, http.StatusOK)

	resp = test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodGet, cfg.resultURL, fmt.Sprintf("audio/%s", id), nil))
	test.CheckCode(t, resp, http.StatusNotFound)

	resp = test.Invoke(t, cfg.httpclient, NewRequest(t, http.MethodHead, cfg.resultURL, fmt.Sprintf("result/%s/lat.txt", id), nil))
	test.CheckCode(t, resp, http.StatusNotFound)

	st := getStatus(t, id)
	assert.Equalf(t, "NOT_FOUND", st.Status, "was '%s'", st.Status)
}

func testEmailReceived(t *testing.T, id, msgType string) {
	t.Helper()
	dur := time.Second * 30
	tm := time.After(dur)
	for {
		select {
		case <-tm:
			require.Failf(t, "Fail", "Not found email for %s(%s) %v", id, msgType, dur)
		default:
			emailData.lock.RLock()
			for _, e := range emailData.mails {
				et := string(e.Text)
				if strings.Contains(et, id) && strings.Contains(e.Subject, msgType) {
					emailData.lock.RUnlock()
					return
				}
			}
			emailData.lock.RUnlock()
			time.Sleep(time.Second)
		}
	}
}

func testRestoreReceived(t *testing.T, id string) {
	t.Helper()
	dur := time.Second * 30
	tm := time.After(dur)
	for {
		select {
		case <-tm:
			require.Failf(t, "Fail", "Not found restore for %s %v", id, dur)
		default:
			restoresData.lock.RLock()
			for _, rID := range restoresData.ids {
				if rID == id {
					restoresData.lock.RUnlock()
					return
				}
			}
			restoresData.lock.RUnlock()
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func uploadWaitFakeFile(t *testing.T) string {
	req := newUploadRequest(t, []string{"audio.wav"}, [][2]string{{"email", "olia@o.o"}, {"recognizer", "ben"},
		{"numberOfSpeakers", "1"}})
	resp := test.Invoke(t, cfg.httpclient, req)
	test.CheckCode(t, resp, http.StatusOK)
	ur := test.Decode[api.StatusData](t, resp)

	testWaitStatus(t, ur.ID, "COMPLETED", false)
	return ur.ID
}

func getStatusSubscribe(t *testing.T, id string) (<-chan api.StatusData, func()) {
	t.Helper()
	client, _ := transcriber.NewClient(cfg.uploadURL, cfg.statusURL, cfg.uploadURL, cfg.uploadURL)
	require.NotNil(t, client)
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
	req.Header.Set("x-doorman-requestid", "asr:" + uuid.NewString())
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
		okID, failID := "1111", "2222"
		switch r.URL.String() {
		case "/ausis/transcriber/upload":
			err := r.ParseMultipartForm(20 << 1)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			rec := ""
			if len(r.MultipartForm.Value["recognizer"]) > 0 {
				rec = r.MultipartForm.Value["recognizer"][0]
			}
			if rec == "failUpload" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if rec == "failStatus" {
				io.Copy(w, strings.NewReader(`{"id":"`+failID+`"}`))
				return
			}
			io.Copy(w, strings.NewReader(`{"id":"`+okID+`"}`))
		case "/ausis/status.service/subscribe":
			handleStatusWS(w, r, func(c *websocket.Conn) {
				mt, id, err := c.ReadMessage()
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
				if string(id) == failID {
					err = c.WriteMessage(mt, []byte(`{"status":"Failure", "error": "error"}`))
				} else {
					err = c.WriteMessage(mt, []byte(`{"status":"COMPLETED", "audioReady":true, "avResults":["lat.txt", "res.txt"],
					"recognizedText": "Olia olia"}`))
				}
				if err != nil {
					log.Print(err)
					return
				}
			})
		case "/ausis/result.service/result/" + okID + "/lat.txt":
			w.Header().Add("content-disposition", `attachment; filename="lat.txt"`)
			io.Copy(w, strings.NewReader(latFileContent))
		case "/ausis/result.service/result/" + okID + "/res.txt":
			w.Header().Add("content-disposition", `attachment; filename="res.txt"`)
			io.Copy(w, strings.NewReader(resFileContent))
		case "/ausis/result.service/audio/" + okID:
			w.Header().Add("content-disposition", `attachment; filename="res.wav"`)
			io.Copy(w, strings.NewReader(audioFileContent))
		case "/ausis/clean.service/delete/" + okID:
			io.Copy(w, strings.NewReader(`OK`))
		case "/fakeURL":
			emailData.lock.Lock()
			defer emailData.lock.Unlock()
			var email email.Email
			err := json.NewDecoder(r.Body).Decode(&email)
			if err != nil {
				log.Print(err)
			}
			log.Printf("got email: %s, %.50s", email.Subject, goapp.Sanitize(string(email.Text)))
			emailData.mails = append(emailData.mails, email)
		default:
			if strings.HasPrefix(r.URL.String(), "/doorman/asr/restore/"){
				restoresData.lock.Lock()
				defer restoresData.lock.Unlock()
				rID := r.URL.String()[len("/doorman/asr/restore/"):]
				log.Printf("got restore: %s", rID)
				restoresData.ids = append(restoresData.ids, rID)
			} else {
				w.WriteHeader(http.StatusNotFound)
				log.Printf("Unknown request to: '%s'", r.URL.String())
			}
		}
	}))

	ts.Listener.Close()
	ts.Listener = l

	// Start the server.
	ts.Start()
	log.Printf("started mock srv on port: %d", port)
	return l, ts
}
