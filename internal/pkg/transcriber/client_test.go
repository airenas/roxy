package transcriber

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testResp struct {
	code    int
	resp    string
	headers map[string]string
}

type testReq struct {
	resp string
	URL  string
}

func newTestR(code int, resp string) testResp {
	return testResp{code: code, resp: resp}
}

func newTestReq(req *http.Request) testReq {
	b, _ := io.ReadAll(req.Body)
	return testReq{URL: req.URL.String(), resp: string(b)}
}

func initTestServer(t *testing.T, rData map[string]testResp) (*Client, *httptest.Server, *[]testReq) {
	t.Helper()
	resRequest := make([]testReq, 0)
	rLock := &sync.Mutex{}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rLock.Lock()
		defer rLock.Unlock()
		resRequest = append(resRequest, newTestReq(req))
		resp, f := rData[req.URL.String()]
		if f {
			for k, v := range resp.headers {
				rw.Header().Set(k, v)
			}
			rw.WriteHeader(resp.code)
			_, _ = rw.Write([]byte(resp.resp))
		} else {
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	// Use Client & URL from our local test server
	api := Client{}
	api.httpclient = server.Client()
	api.statusWSURL = server.URL
	api.statusURL = server.URL
	api.resultURL = server.URL
	api.uploadURL, _ = url.JoinPath(server.URL, "upload")
	api.cleanURL = server.URL
	api.uploadTimeout = time.Second * 5
	api.timeout = time.Second
	api.backoff = func() backoff.BackOff {
		return &backoff.StopBackOff{}
	}
	t.Cleanup(func() { server.Close() })
	return &api, server, &resRequest
}

func testCalled(t *testing.T, URL string, tReq []testReq) {
	assert.GreaterOrEqual(t, len(tReq), 1)
	str := ""
	for _, r := range tReq {
		str = r.URL
		if str == URL {
			return
		}
	}
	assert.Equal(t, URL, str)
}

func TestStatusWS(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		c, err := upgrader.Upgrade(rw, req, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, _, err := c.ReadMessage()
			if err != nil {
				break
			}
			err = c.WriteMessage(mt, []byte(`{"status":"COMPLETED"}`))
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	client := Client{}
	client.httpclient = server.Client()
	client.statusWSURL, _ = url.JoinPath("ws"+strings.TrimPrefix(server.URL, "http"), "status")
	client.timeout = time.Second
	client.backoff = func() backoff.BackOff {
		return &backoff.StopBackOff{}
	}

	r, cf, err := client.HookToStatus(test.Ctx(t), "k10")
	defer cf()
	require.Nil(t, err)
	select {
	case v := <-r:
		assert.Equal(t, "COMPLETED", v.Status)
	case <-time.After(time.Second):
		assert.Fail(t, "timeout")
	}
}

func TestStatus(t *testing.T) {
	resp := newTestR(http.StatusOK, `{"status":"COMPLETED"}`)
	client, _, tReq := initTestServer(t, map[string]testResp{"/status/k10": resp})

	r, err := client.GetStatus(test.Ctx(t), "k10")

	assert.Nil(t, err)
	assert.Equal(t, "COMPLETED", r.Status)
	testCalled(t, "/status/k10", *tReq)
}

func TestResult(t *testing.T) {
	resp := newTestR(200, "olia")
	resp.headers = map[string]string{"content-disposition": `attachment; filename="lat123.txt"`}
	client, _, tReq := initTestServer(t, map[string]testResp{"/result/k10/lat.txt": resp})

	r, err := client.GetResult(test.Ctx(t), "k10", "lat.txt")

	assert.Nil(t, err)
	assert.Equal(t, r.Name, "lat123.txt")
	assert.Equal(t, []byte("olia"), r.Content)
	testCalled(t, "/result/k10/lat.txt", *tReq)
}

func TestResult_WrongCode_Fails(t *testing.T) {
	resp := newTestR(400, "olia")
	resp.headers = map[string]string{"content-disposition": `attachment; filename="lat123.txt"`}
	client, _, tReq := initTestServer(t, map[string]testResp{"/result/k10/file": resp})

	r, err := client.GetResult(test.Ctx(t), "k10", "file")

	assert.NotNil(t, err)
	assert.Nil(t, r)
	testCalled(t, "/result/k10/file", *tReq)
}

func TestResult_WrongMediaType(t *testing.T) {
	resp := newTestR(200, "olia")
	client, _, tReq := initTestServer(t, map[string]testResp{"/result/k10/file": resp})

	r, err := client.GetResult(test.Ctx(t), "k10", "file")

	assert.NotNil(t, err)
	assert.Nil(t, r)
	testCalled(t, "/result/k10/file", *tReq)
}

func TestUpload(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(200, "{\"id\":\"1\"}")})

	r, err := client.Upload(test.Ctx(t), &api.UploadData{Params: map[string]string{"name": "name"}})

	assert.Nil(t, err)
	assert.Equal(t, r, "1")
	testCalled(t, "/upload", *tReq)
}

func TestUpload_WrongCode_Fails(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/": newTestR(300, "{\"id\":\"1\"}")})

	r, err := client.Upload(test.Ctx(t), &api.UploadData{Params: map[string]string{"name": "name"}})

	assert.NotNil(t, err)
	assert.Equal(t, "", r)
	testCalled(t, "/upload", *tReq)
}

func TestUpload_WrongJSON_Fails(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(300, "olia")})

	r, err := client.Upload(test.Ctx(t), &api.UploadData{Params: map[string]string{"name": "name"}})

	assert.NotNil(t, err)
	assert.Equal(t, r, "")
	testCalled(t, "/upload", *tReq)
}

func TestUpload_PassParams(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(200, "{\"id\":\"1\"}")})

	r, err := client.Upload(test.Ctx(t), &api.UploadData{Params: map[string]string{"name": "__olia__"}})

	assert.Nil(t, err)
	assert.Equal(t, "1", r)
	testCalled(t, "/upload", *tReq)
	bs := (*tReq)[0].resp
	assert.Contains(t, bs, "name")
	assert.Contains(t, bs, "__olia__")
}

func TestUpload_PassFile(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(200, "{\"id\":\"1\"}")})

	r, err := client.Upload(test.Ctx(t), &api.UploadData{Params: map[string]string{"name": "__olia__"},
		Files: map[string]io.Reader{"file.wav": strings.NewReader("__file_olia__")}})

	assert.Nil(t, err)
	assert.Equal(t, r, "1")
	testCalled(t, "/upload", *tReq)
	bs := (*tReq)[0].resp
	assert.Contains(t, bs, "file.wav")
	assert.Contains(t, bs, "__file_olia__")
}

func TestUpload_Backoff(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(http.StatusTooManyRequests, "{\"id\":\"1\"}")})
	client.backoff = newSimpleBackoff

	_, err := client.Upload(test.Ctx(t), &api.UploadData{Params: map[string]string{"name": "__olia__"},
		Files: map[string]io.Reader{"file.wav": strings.NewReader("__file_olia__")}})

	assert.NotNil(t, err)
	assert.Equal(t, 4, len((*tReq)))
}

func TestUpload_NoBackoff(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(http.StatusBadRequest, "{\"id\":\"1\"}")})
	client.backoff = newSimpleBackoff

	_, err := client.Upload(test.Ctx(t), &api.UploadData{Params: map[string]string{"name": "__olia__"},
		Files: map[string]io.Reader{"file.wav": strings.NewReader("__file_olia__")}})

	assert.NotNil(t, err)
	assert.Equal(t, 1, len((*tReq)))
}

func TestUpload_NoBackoff_Deadline(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(http.StatusBadRequest, "{\"id\":\"1\"}")})
	client.backoff = newSimpleBackoff

	ctx, cf := context.WithDeadline(context.Background(), time.Now())
	defer cf()

	_, err := client.Upload(ctx, &api.UploadData{Params: map[string]string{"name": "__olia__"},
		Files: map[string]io.Reader{"file.wav": strings.NewReader("__file_olia__")}})

	assert.NotNil(t, err)
	assert.Equal(t, 0, len((*tReq)))
}

func TestUpload_NoBackoff_Canceled(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/upload": newTestR(http.StatusBadRequest, "{\"id\":\"1\"}")})
	client.backoff = newSimpleBackoff

	ctx, cf := context.WithCancel(context.Background())
	cf()

	_, err := client.Upload(ctx, &api.UploadData{Params: map[string]string{"name": "__olia__"},
		Files: map[string]io.Reader{"file.wav": strings.NewReader("__file_olia__")}})

	assert.NotNil(t, err)
	assert.Equal(t, 0, len((*tReq)))
}

func TestClean(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/10": newTestR(200, "OK")})

	err := client.Clean(test.Ctx(t), "10")

	assert.Nil(t, err)
	testCalled(t, "/10", *tReq)
}

func TestClean_Fails(t *testing.T) {
	client, _, tReq := initTestServer(t, map[string]testResp{"/10": newTestR(500, "Error")})

	err := client.Clean(test.Ctx(t), "10")

	assert.NotNil(t, err)
	testCalled(t, "/10", *tReq)
	assert.Equal(t, 4, len((*tReq)))
}

func TestNewClient(t *testing.T) {
	type args struct {
		uploadURL string
		statusURL string
		resultURL string
		cleanURL  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "OK", args: args{uploadURL: "http://olia", statusURL: "http://olia", resultURL: "http://olia", cleanURL: "http://olia"}, wantErr: false},
		{name: "Missing params", args: args{statusURL: "http://olia", resultURL: "http://olia", cleanURL: "http://olia"}, wantErr: true},
		{name: "Missing params", args: args{uploadURL: "http://olia", resultURL: "http://olia", cleanURL: "http://olia"}, wantErr: true},
		{name: "Missing params", args: args{uploadURL: "http://olia", statusURL: "http://olia", cleanURL: "http://olia"}, wantErr: true},
		{name: "Missing params", args: args{uploadURL: "http://olia", statusURL: "http://olia", resultURL: "http://olia"}, wantErr: true},
		{name: "Wrong  status", args: args{uploadURL: "http://olia", statusURL: "ops://olia", resultURL: "http://olia", cleanURL: "http://olia"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewClient(tt.args.uploadURL, tt.args.statusURL, tt.args.resultURL, tt.args.cleanURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("NewClient() = nil, want object")
			}
		})
	}
}
