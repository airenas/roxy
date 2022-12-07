package result

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	filerMock *mocks.Filer
	dbMock    *mocks.DB
	tData     *Data
	tEcho     *echo.Echo
)

func initTest(t *testing.T) {
	filerMock = &mocks.Filer{}
	dbMock = &mocks.DB{}
	tData = &Data{}
	tData.NameProvider = dbMock
	tData.Reader = filerMock
	tEcho = initRoutes(tData)
	filerMock.On("LoadFile", mock.Anything, "1/olia").Return(&testFileWrap{s: "olia", n: "res.txt"}, nil)
	filerMock.On("LoadFile", mock.Anything, "1/1.wav").Return(&testFileWrap{s: "audio", n: "res.wav"}, nil)
	dbMock.On("LoadRequest", mock.Anything, "1").Return(&persistence.ReqData{ID: "1", Filename: "1.wav"}, nil)
}

func TestWrongPath(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/invalid", nil)
	test.Code(t, tEcho, req, http.StatusNotFound)
}

func TestWrongMethod(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodPost, "/result/1/olia", nil)
	test.Code(t, tEcho, req, 405)
}

func Test_Result(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/result/1/olia", nil)
	resp := test.Code(t, tEcho, req, http.StatusOK)
	assert.Equal(t, "olia", test.RStr(t, resp.Body))
	assert.Equal(t, "attachment; filename=res.txt", resp.Header().Get("Content-Disposition"))
}

func Test_Audio(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/audio/1", nil)
	resp := test.Code(t, tEcho, req, http.StatusOK)
	assert.Equal(t, "audio", test.RStr(t, resp.Body))
	assert.Equal(t, "attachment; filename=res.wav", resp.Header().Get("Content-Disposition"))
}

func Test_Result_NoFile(t *testing.T) {
	initTest(t)
	filerMock.On("LoadFile", mock.Anything, "2/olia").Return(nil, minio.ErrorResponse{StatusCode: http.StatusNotFound})
	req := httptest.NewRequest(http.MethodGet, "/result/2/olia", nil)
	test.Code(t, tEcho, req, http.StatusNotFound)
}

func Test_Result_NoFile2(t *testing.T) {
	initTest(t)
	filerMock.On("LoadFile", mock.Anything, "1/olia.tt").Return(nil, minio.ErrorResponse{StatusCode: http.StatusNotFound})
	req := httptest.NewRequest(http.MethodGet, "/result/1/olia.tt", nil)
	test.Code(t, tEcho, req, http.StatusNotFound)
}

func Test_Audio_NoFile(t *testing.T) {
	initTest(t)
	filerMock.ExpectedCalls = nil
	filerMock.On("LoadFile", mock.Anything, "1/1.wav").Return(nil, minio.ErrorResponse{StatusCode: http.StatusNotFound})
	req := httptest.NewRequest(http.MethodGet, "/audio/1", nil)
	test.Code(t, tEcho, req, http.StatusNotFound)
}

func Test_ResultHead(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodHead, "/result/1/olia", nil)
	resp := test.Code(t, tEcho, req, http.StatusOK)
	assert.Equal(t, "", test.RStr(t, resp.Body))
	assert.Equal(t, "attachment; filename=res.txt", resp.Header().Get("Content-Disposition"))
}

func Test_AudioHead(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodHead, "/audio/1", nil)
	resp := test.Code(t, tEcho, req, http.StatusOK)
	assert.Equal(t, "", test.RStr(t, resp.Body))
	assert.Equal(t, "attachment; filename=res.wav", resp.Header().Get("Content-Disposition"))
}

func Test_Live(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	test.Code(t, tEcho, req, 200)
}

type testFileWrap struct {
	s string
	n string
}

// Read implements io.ReadSeekCloser
func (fw *testFileWrap) Read(p []byte) (n int, err error) {
	return strings.NewReader(fw.s).Read(p)
}

// Seek implements io.ReadSeekCloser
func (fw *testFileWrap) Seek(offset int64, whence int) (int64, error) {
	return strings.NewReader(fw.s).Seek(offset, whence)
}

// Close implements io.ReadSeekCloser
func (fw *testFileWrap) Close() error {
	return nil
}

// Stat returns file stat
func (fw *testFileWrap) Stat() (fs.FileInfo, error) {
	return &testStatsWrap{size: int64(len(fw.s)), name: fw.n}, nil
}

type testStatsWrap struct {
	size int64
	name string
}

// IsDir implements fs.FileInfo
func (sw *testStatsWrap) IsDir() bool {
	return false
}

// ModTime implements fs.FileInfo
func (sw *testStatsWrap) ModTime() time.Time {
	return time.Now()
}

// Mode implements fs.FileInfo
func (sw *testStatsWrap) Mode() fs.FileMode {
	return fs.ModeTemporary
}

// Name implements fs.FileInfo
func (sw *testStatsWrap) Name() string {
	return sw.name
}

// Size implements fs.FileInfo
func (sw *testStatsWrap) Size() int64 {
	return sw.size
}

// Sys implements fs.FileInfo
func (sw *testStatsWrap) Sys() any {
	return nil
}
