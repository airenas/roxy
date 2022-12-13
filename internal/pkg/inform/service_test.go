package inform

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/airenas/async-api/pkg/inform"
	"github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/test/mocks"
	"github.com/jordan-wright/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/vgarvardt/gue/v5"
)

var (
	dbMock     *mocks.DB
	senderMock *mockEmailSender
	makerMock  *mockEmailMaker
	srvData    *ServiceData
)

func initTest(t *testing.T) {
	dbMock = &mocks.DB{}
	senderMock = &mockEmailSender{}
	makerMock = &mockEmailMaker{}
	srvData = &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, EmailSender: senderMock,
		EmailMaker: makerMock, Location: nil}
	dbMock.On("LoadRequest", mock.Anything, "1").Return(&persistence.ReqData{ID: "1", FileName: sql.NullString{String: "1.wav"},
		FileNames: []string{"1.wav"}, Email: "o@o.lt"}, nil)
	dbMock.On("LockEmailTable", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("UnLockEmailTable", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	senderMock.On("Send", mock.Anything).Return(nil)
	makerMock.On("Make", mock.Anything).Return(&email.Email{From: "o@o.lt", Text: []byte("text")}, nil)

}

func Test_handleInform(t *testing.T) {
	initTest(t)
	err := handleInform(test.Ctx(t), &messages.InformMessage{QueueMessage: messages.QueueMessage{ID: "1"}, Type: messages.InformTypeStarted}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 3, len(dbMock.Calls))
	assert.Equal(t, "Started", dbMock.Calls[1].Arguments[2])
	assert.Equal(t, "Started", dbMock.Calls[2].Arguments[2])
	assert.Equal(t, 2, dbMock.Calls[2].Arguments[3])
}

func Test_handleInformFinish(t *testing.T) {
	initTest(t)
	err := handleInform(test.Ctx(t), &messages.InformMessage{QueueMessage: messages.QueueMessage{ID: "1"}, Type: messages.InformTypeFinished}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 3, len(dbMock.Calls))
	assert.Equal(t, messages.InformTypeFinished, dbMock.Calls[1].Arguments[2])
	assert.Equal(t, messages.InformTypeFinished, dbMock.Calls[2].Arguments[2])
}

func Test_handleInform_FailDB(t *testing.T) {
	initTest(t)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadRequest", mock.Anything, "1").Return(nil, fmt.Errorf("err"))
	err := handleInform(test.Ctx(t), &messages.InformMessage{QueueMessage: messages.QueueMessage{ID: "1"}, Type: messages.InformTypeStarted}, srvData)
	assert.NotNil(t, err)
}

func Test_handleInform_FailMaker(t *testing.T) {
	initTest(t)
	makerMock.ExpectedCalls = nil
	makerMock.On("Make", mock.Anything).Return(nil, fmt.Errorf("err"))
	err := handleInform(test.Ctx(t), &messages.InformMessage{QueueMessage: messages.QueueMessage{ID: "1"}, Type: messages.InformTypeStarted}, srvData)
	assert.NotNil(t, err)
}

func Test_handleInform_FailSender(t *testing.T) {
	initTest(t)
	senderMock.ExpectedCalls = nil
	senderMock.On("Send", mock.Anything).Return(fmt.Errorf("err"))
	err := handleInform(test.Ctx(t), &messages.InformMessage{QueueMessage: messages.QueueMessage{ID: "1"}, Type: messages.InformTypeStarted}, srvData)
	assert.NotNil(t, err)
	require.Equal(t, 3, len(dbMock.Calls))
	assert.Equal(t, "Started", dbMock.Calls[1].Arguments[2])
	assert.Equal(t, "Started", dbMock.Calls[2].Arguments[2])
	assert.Equal(t, 0, dbMock.Calls[2].Arguments[3])
}

func Test_validate(t *testing.T) {
	initTest(t)
	type args struct {
		data *ServiceData
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "OK", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, EmailSender: senderMock,
			EmailMaker: makerMock}}, wantErr: false},
		{name: "Fail no data", args: args{data: &ServiceData{GueClient: &gue.Client{}, WorkerCount: 10, EmailSender: senderMock,
			EmailMaker: makerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, WorkerCount: 10, EmailSender: senderMock,
			EmailMaker: makerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, EmailSender: senderMock,
			EmailMaker: makerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10,
			EmailMaker: makerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, EmailSender: senderMock}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("StartServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type mockEmailSender struct{ mock.Mock }

func (m *mockEmailSender) Send(email *email.Email) error {
	args := m.Called(email)
	return args.Error(0)
}

type mockEmailMaker struct{ mock.Mock }

func (m *mockEmailMaker) Make(data *inform.Data) (*email.Email, error) {
	args := m.Called(data)
	return mocks.To[*email.Email](args.Get(0)), args.Error(1)
}
