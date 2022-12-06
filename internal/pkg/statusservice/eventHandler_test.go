package statusservice

import (
	"fmt"
	"testing"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/vgarvardt/gue/v5"
)

var (
	dbEHMock       *mocks.DB
	handlerEHMMock *mockWSConnHandler
	hndData        *HandlerData
	connMock       *mockWSConn
)

func initHandlerTest(t *testing.T) {
	dbMock = &mocks.DB{}
	handlerEHMMock = &mockWSConnHandler{}
	connMock = &mockWSConn{}
	hndData = &HandlerData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, WSHandler: handlerEHMMock}
	handlerEHMMock.On("GetConnections", mock.Anything).Return([]WsConn{connMock}, true)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		AudioReady: true, AvailableResults: []string{"r1", "r2"}}, nil)
	connMock.On("WriteJSON", mock.Anything).Return(nil)
}

func Test_handleStatus(t *testing.T) {
	initHandlerTest(t)
	err := handleStatus(test.Ctx(t), &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: "1"}}, hndData)
	assert.Nil(t, err)
	require.Equal(t, 1, len(connMock.Calls))
	assert.Equal(t, &result{ID: "1", Status: "Done", AudioReady: true, AvailableResults: []string{"r1", "r2"}}, connMock.Calls[0].Arguments[0])
}

func Test_handleStatus_NoConn(t *testing.T) {
	initHandlerTest(t)
	handlerEHMMock.ExpectedCalls = nil
	handlerEHMMock.On("GetConnections", mock.Anything).Return([]WsConn{}, false)
	err := handleStatus(test.Ctx(t), &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: "1"}}, hndData)
	assert.Nil(t, err)
	require.Equal(t, 0, len(connMock.Calls))
}

func Test_handleStatus_NoStatus(t *testing.T) {
	initHandlerTest(t)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(nil, nil)
	err := handleStatus(test.Ctx(t), &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: "1"}}, hndData)
	assert.NotNil(t, err)
}

func Test_handleStatus_Error(t *testing.T) {
	initHandlerTest(t)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("olia"))
	err := handleStatus(test.Ctx(t), &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: "1"}}, hndData)
	assert.NotNil(t, err)
}

func Test_validateHandler(t *testing.T) {
	initHandlerTest(t)
	type args struct {
		data *HandlerData
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "OK", args: args{data: &HandlerData{DB: dbEHMock, GueClient: &gue.Client{}, WorkerCount: 10, WSHandler: handlerEHMMock}}, wantErr: false},
		{name: "Fail no data", args: args{data: &HandlerData{GueClient: &gue.Client{}, WorkerCount: 10, WSHandler: handlerEHMMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &HandlerData{DB: dbEHMock, WorkerCount: 10, WSHandler: handlerEHMMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &HandlerData{DB: dbEHMock, GueClient: &gue.Client{}, WSHandler: handlerEHMMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &HandlerData{DB: dbEHMock, GueClient: &gue.Client{}, WorkerCount: 10}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateHandler(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("StartServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type mockWSConn struct{ mock.Mock }

func (m *mockWSConn) ReadMessage() (messageType int, p []byte, err error) {
	args := m.Called()
	return args.Int(0), args.Get(1).([]byte), args.Error(2)
}

func (m *mockWSConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockWSConn) WriteJSON(v interface{}) error {
	args := m.Called(v)
	return args.Error(0)
}
