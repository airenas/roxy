package statusservice

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	wsHandlerMock *mockWSConnHandler
	dbMock        *mocks.DB
	tData         *Data
	tEcho         *echo.Echo
	tResp         *httptest.ResponseRecorder
)

func initTest(t *testing.T) {
	wsHandlerMock = &mockWSConnHandler{}
	dbMock = &mocks.DB{}
	tData = &Data{}
	tData.DB = dbMock
	tData.WSHandler = wsHandlerMock
	tEcho = initRoutes(tData)
	tResp = httptest.NewRecorder()
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		AudioReady: true, AvailableResults: []string{"r1", "r2"}}, nil)
}

func TestWrongPath(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/invalid", nil)
	testCode(t, req, 404)
}

func TestWrongMethod(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodPost, "/status/1", nil)
	testCode(t, req, 405)
}

func Test_Status_Returns(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/status/1", nil)
	resp := testCode(t, req, http.StatusOK)
	res := test.Decode[result](t, resp.Result())
	assert.Equal(t, result{ID: "1", Status: "Done", AudioReady: true, AvailableResults: []string{"r1", "r2"}}, res)
}

func Test_Status_Empty(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/status/2", nil)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(nil, nil)
	resp := testCode(t, req, http.StatusOK)
	res := test.Decode[result](t, resp.Result())
	assert.Equal(t, result{ID: "2", Status: "NOT_FOUND", Error: "Nežinomas ID: 2", ErrorCode: "NOT_FOUND",
		AvailableResults: nil}, res)
}

func Test_Status_Fail(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/status/1", nil)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("olia"))
	_ = testCode(t, req, http.StatusInternalServerError)
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
		{name: "OK", args: args{data: &Data{DB: dbMock, WSHandler: wsHandlerMock}}, wantErr: false},
		{name: "Fail Handler", args: args{data: &Data{DB: dbMock}}, wantErr: true},
		{name: "Fail DB", args: args{data: &Data{WSHandler: wsHandlerMock}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("StartWebServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type mockWSConnHandler struct{ mock.Mock }

func (m *mockWSConnHandler) HandleConnection(wc WsConn) error {
	args := m.Called(wc)
	return args.Error(0)
}

func (m *mockWSConnHandler) GetConnections(id string) ([]WsConn, bool) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Bool(1)
	}
	return args.Get(0).([]WsConn), args.Bool(1)
}
