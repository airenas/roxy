package upload

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/airenas/roxy/internal/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	saverMock  *mocks.Filer
	dbMock     *mocks.DB
	senderMock *mocks.Sender
	tData      *Data
	tEcho      *echo.Echo
	tResp      *httptest.ResponseRecorder
)

func initTest(t *testing.T) {
	saverMock = &mocks.Filer{}
	dbMock = &mocks.DB{}
	senderMock = &mocks.Sender{}
	tData = &Data{}
	tData.Saver = saverMock
	tData.DB = dbMock
	tData.MsgSender = senderMock
	tData.RetrySecret = "secret"
	tEcho = initRoutes(tData)
	tResp = httptest.NewRecorder()
	dbMock.On("InsertRequest", mock.Anything, mock.Anything).Return(nil)
	dbMock.On("InsertStatus", mock.Anything, mock.Anything).Return(nil)
	saverMock.On("SaveFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
}

func TestWrongPath(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/invalid", nil)
	testCode(t, req, 404)
}

func TestWrongMethod(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/upload", nil)
	testCode(t, req, 405)
}

func Test_Returns(t *testing.T) {
	initTest(t)
	req := newTestRequest("file", "file.wav", "olia", nil)
	resp := testCode(t, req, 200)
	bytes, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(bytes), `"id":"`)
	require.Equal(t, len(senderMock.Calls), 1)
}

func Test_400(t *testing.T) {
	type args struct {
		filep, file string
		params      [][2]string
	}
	tests := []struct {
		name     string
		args     args
		wantCode int
	}{
		{name: "OK", args: args{file: "file.wav", filep: "file"}, wantCode: http.StatusOK},
		{name: "File", args: args{file: "file.wav", filep: "file1"}, wantCode: http.StatusBadRequest},
		{name: "FileName", args: args{file: "file.txt", filep: "file"}, wantCode: http.StatusBadRequest},
		{name: "Param", args: args{file: "file.wav", filep: "file", params: [][2]string{{"email", "olia"}}},
			wantCode: http.StatusOK},
		{name: "Voice", args: args{file: "file.wav", filep: "file", params: [][2]string{{"email1", "wrong param"}}},
			wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initTest(t)
			req := newTestRequest(tt.args.filep, tt.args.file, "olia", tt.args.params)
			testCode(t, req, tt.wantCode)
		})
	}
}

func Test_Fails_Saver(t *testing.T) {
	initTest(t)
	req := newTestRequest("file", "file.wav", "olia", nil)
	saverMock.ExpectedCalls = nil
	saverMock.On("SaveFile", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("err"))

	testCode(t, req, http.StatusInternalServerError)
}

func Test_Fails_ReqSaver(t *testing.T) {
	initTest(t)
	req := newTestRequest("file", "file.wav", "olia", nil)
	dbMock.ExpectedCalls = nil
	dbMock.On("InsertRequest", mock.Anything, mock.Anything).Return(fmt.Errorf("err"))

	testCode(t, req, http.StatusInternalServerError)
}

func Test_Fails_MsgSender(t *testing.T) {
	initTest(t)
	req := newTestRequest("file", "file.wav", "olia", nil)
	senderMock.ExpectedCalls = nil
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("err"))

	testCode(t, req, http.StatusInternalServerError)
}

func Test_Retry(t *testing.T) {
	initTest(t)
	dbMock.On("DeleteWorkData", mock.Anything, "10").Return(nil)
	req := httptest.NewRequest("POST", "/retry/secret/10", nil)
	resp := testCode(t, req, http.StatusOK)
	bytes, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(bytes), `"id":"`)
	require.Equal(t, len(dbMock.Calls), 1)
	require.Equal(t, len(senderMock.Calls), 1)
}

func Test_Retry_Fails(t *testing.T) {
	initTest(t)
	dbMock.On("DeleteWorkData", mock.Anything, "10").Return(fmt.Errorf("err"))
	req := httptest.NewRequest("POST", "/retry/secret/10", nil)
	testCode(t, req, http.StatusInternalServerError)
	require.Equal(t, len(dbMock.Calls), 1)
}

func Test_Live(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	testCode(t, req, 200)
}

func testCode(t *testing.T, req *http.Request, code int) *httptest.ResponseRecorder {
	t.Helper()
	tEcho.ServeHTTP(tResp, req)
	require.Equal(t, code, tResp.Code)
	return tResp
}

func Test_validate(t *testing.T) {
	initTest(t)
	type args struct {
		data *Data
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "OK", args: args{data: &Data{Saver: saverMock, DB: dbMock, MsgSender: senderMock}}, wantErr: false},
		{name: "Fail Saver", args: args{data: &Data{DB: dbMock, MsgSender: senderMock}}, wantErr: true},
		{name: "Fail DB", args: args{data: &Data{Saver: saverMock, MsgSender: senderMock}}, wantErr: true},
		{name: "Fail Sender", args: args{data: &Data{Saver: saverMock, DB: dbMock}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("StartWebServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func newTestRequest(filep, file, bodyText string, params [][2]string) *http.Request {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if file != "" {
		part, _ := writer.CreateFormFile(filep, file)
		_, _ = io.Copy(part, strings.NewReader(bodyText))
	}
	for _, p := range params {
		_ = writer.WriteField(p[0], p[1])
	}
	writer.Close()
	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(requestIDHEader, "m:testRequestID")
	return req
}

func Test_extractRequestID(t *testing.T) {
	req := newTestRequest("file", "file.wav", "olia", nil)
	assert.Equal(t, "m:testRequestID", extractRequestID(req.Header))
}
