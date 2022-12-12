package clean

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
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
	tData.Cleaner = newCleanMock(false)
	tEcho = initRoutes(tData)
}

func TestWrongPath(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/invalid", nil)
	test.Code(t, tEcho, req, http.StatusNotFound)
}

func TestWrongMethod(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodPost, "/delete/1", nil)
	test.Code(t, tEcho, req, 405)
}

func Test_Clean(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodDelete, "/delete/1", nil)
	test.Code(t, tEcho, req, http.StatusOK)
}

func Test_Clean_Fails(t *testing.T) {
	initTest(t)
	tData.Cleaner = newCleanMock(true)
	tEcho = initRoutes(tData)
	req := httptest.NewRequest(http.MethodDelete, "/delete/1", nil)
	test.Code(t, tEcho, req, http.StatusInternalServerError)
}

func Test_Live(t *testing.T) {
	initTest(t)
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	test.Code(t, tEcho, req, 200)
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
		{name: "OK", args: args{data: &Data{Cleaner: newCleanMock(false)}}, wantErr: false},
		{name: "Fail Cleaner", args: args{data: &Data{}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("StartWebServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type mockCleaner struct{ mock.Mock }

func (m *mockCleaner) Clean(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func newCleanMock(fail bool) *mockCleaner {
	res := &mockCleaner{}
	var err error
	if fail {
		err = errors.New("olia")
	}
	res.On("Clean", mock.Anything, mock.Anything).Return(err)
	return res
}
