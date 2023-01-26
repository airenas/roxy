package worker

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/api"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/status"
	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/airenas/roxy/internal/pkg/test/mocks"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/airenas/roxy/internal/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/vgarvardt/gue/v5"
)

var (
	filerMock       *mocks.Filer
	dbMock          *mocks.DB
	senderMock      *mocks.Sender
	transcriberMock *mocks.Transcriber
	transcriberPrMock *mocks.TranscriberProvider
	uRestorerMock   *mockUsageRestorer
	srvData         *ServiceData
)

func initTest(t *testing.T) {
	filerMock = &mocks.Filer{}
	dbMock = &mocks.DB{}
	senderMock = &mocks.Sender{}
	transcriberMock = &mocks.Transcriber{}
	transcriberPrMock = &mocks.TranscriberProvider{}
	uRestorerMock = &mockUsageRestorer{}
	srvData = &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
		Filer: filerMock, TranscriberPr: transcriberPrMock, UsageRestorer: uRestorerMock}
	transcriberMock.On("Clean", mock.Anything, mock.Anything).Return(nil)
	uRestorerMock.On("Do", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	transcriberPrMock.On("Get", mock.Anything, mock.Anything).Return(transcriberMock, "http://srv:8080", nil)
}

func Test_handleClean(t *testing.T) {
	initTest(t)
	err := handleClean(test.Ctx(t), &messages.CleanMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2"}, srvData)
	assert.Nil(t, err)
}

func Test_handleClean_Fail(t *testing.T) {
	initTest(t)
	transcriberMock.ExpectedCalls = nil
	transcriberMock.On("Clean", mock.Anything, mock.Anything).Return(fmt.Errorf("olia err"))
	err := handleClean(test.Ctx(t), &messages.CleanMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2"}, srvData)
	assert.NotNil(t, err)
}

func Test_handleRestoreUsage(t *testing.T) {
	initTest(t)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadRequest", mock.Anything, mock.Anything).Return(&persistence.ReqData{ID: "1", RequestID: "rID"}, nil)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr(status.ECServiceError.String()), Error: utils.ToSQLStr("st err")}, nil)
	err := handleRestoreUsage(test.Ctx(t), &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: "1"}}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 1, len(uRestorerMock.Calls))
	assert.Equal(t, "1", uRestorerMock.Calls[0].Arguments[1])
	assert.Equal(t, "rID", uRestorerMock.Calls[0].Arguments[2])
	assert.Equal(t, "st err", uRestorerMock.Calls[0].Arguments[3])
}

func Test_handleRestoreUsage_skip(t *testing.T) {
	initTest(t)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadRequest", mock.Anything, mock.Anything).Return(&persistence.ReqData{ID: "1", RequestID: "rID"}, nil)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr("errCode"), Error: utils.ToSQLStr("st err")}, nil)
	err := handleRestoreUsage(test.Ctx(t), &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: "1"}}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 0, len(uRestorerMock.Calls))
}

func Test_handleRestoreUsage_Fail(t *testing.T) {
	initTest(t)
	dbMock.ExpectedCalls = nil
	dbMock.On("LoadRequest", mock.Anything, mock.Anything).Return(&persistence.ReqData{ID: "1", RequestID: "rID"}, nil)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr(status.ECServiceError.String()), Error: utils.ToSQLStr("st err")}, nil)
	uRestorerMock.ExpectedCalls = nil
	uRestorerMock.On("Do", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("err"))
	err := handleRestoreUsage(test.Ctx(t), &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: "1"}}, srvData)
	assert.NotNil(t, err)
}

func Test_handleStatus(t *testing.T) {
	initTest(t)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr(status.ECServiceError.String()), Error: utils.ToSQLStr("st err")}, nil)
	dbMock.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := handleStatus(test.Ctx(t), &messages.StatusMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2",
		Status: "Starting"}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 1, len(senderMock.Calls))
	require.Equal(t, messages.DefaultOpts(messages.StatusChange) , senderMock.Calls[0].Arguments[2])
}

func Test_handleStatus_saveAudio(t *testing.T) {
	initTest(t)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr(status.ECServiceError.String()), Error: utils.ToSQLStr("st err")}, nil)
	dbMock.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	transcriberMock.On("GetAudio", mock.Anything, mock.Anything).Return(&tapi.FileData{Name: "olia", Content: []byte("Olia data")}, nil)
	filerMock.On("SaveFile", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := handleStatus(test.Ctx(t), &messages.StatusMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2",
		Status: "Starting", AudioReady: true}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 1, len(filerMock.Calls))
	require.Equal(t, int64(9), filerMock.Calls[0].Arguments[3])
}

func Test_handleStatus_saveFiles(t *testing.T) {
	initTest(t)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr(status.ECServiceError.String()), Error: utils.ToSQLStr("st err")}, nil)
	dbMock.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	transcriberMock.On("GetAudio", mock.Anything, mock.Anything).Return(&tapi.FileData{Name: "olia", Content: []byte("Olia data")}, nil)
	transcriberMock.On("GetResult", mock.Anything, mock.Anything, mock.Anything).Return(&tapi.FileData{Name: "olia", Content: []byte("res data")}, nil)
	filerMock.On("SaveFile", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := handleStatus(test.Ctx(t), &messages.StatusMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2",
		Status: "Starting", AudioReady: true, AvailableResults: []string{"res.txt"}}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 2, len(filerMock.Calls))
	require.Equal(t, int64(8), filerMock.Calls[1].Arguments[3])
}

func Test_handleStatus_completed(t *testing.T) {
	initTest(t)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr(status.ECServiceError.String()), Error: utils.ToSQLStr("st err")}, nil)
	dbMock.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := handleStatus(test.Ctx(t), &messages.StatusMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2",
		Status: "COMPLETED"}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 3, len(senderMock.Calls))
	require.Equal(t, messages.DefaultOpts(messages.StatusChange), senderMock.Calls[0].Arguments[2])
	require.Equal(t, messages.DefaultOpts(wrkQueuePrefix+wrkStatusClean), senderMock.Calls[1].Arguments[2])
	require.Equal(t, messages.DefaultOpts(messages.Inform), senderMock.Calls[2].Arguments[2])
}

func Test_handleStatus_completedOnError(t *testing.T) {
	initTest(t)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		ErrorCode: utils.ToSQLStr(status.ECServiceError.String()), Error: utils.ToSQLStr("st err")}, nil)
	dbMock.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := handleStatus(test.Ctx(t), &messages.StatusMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2",
		Status: "Start", ErrorCode: "Service_err", Error: "error"}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 4, len(senderMock.Calls))
	require.Equal(t, messages.DefaultOpts(messages.StatusChange), senderMock.Calls[0].Arguments[2])
	require.Equal(t, messages.DefaultOpts(wrkQueuePrefix+wrkStatusClean), senderMock.Calls[1].Arguments[2])
	require.Equal(t, messages.DefaultOpts(messages.Inform), senderMock.Calls[2].Arguments[2])
	require.Equal(t, messages.DefaultOpts(wrkQueuePrefix+wrkRestoreUsage), senderMock.Calls[3].Arguments[2])
}

func Test_handleStatus_skip(t *testing.T) {
	initTest(t)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		Progress: utils.ToSQLInt32(50), Updated: time.Now().Add(-time.Minute)}, nil)
	dbMock.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := handleStatus(test.Ctx(t), &messages.StatusMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2",
		Status: "Start", Progress: 40}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 1, len(dbMock.Calls)) // just load
}

func Test_handleStatus_noSkip_oldRecord(t *testing.T) {
	initTest(t)
	dbMock.On("LoadStatus", mock.Anything, mock.Anything).Return(&persistence.Status{ID: "1", Status: "Done",
		Progress: utils.ToSQLInt32(50), Updated: time.Now().Add(-60 * time.Minute)}, nil)
	dbMock.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	senderMock.On("SendMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := handleStatus(test.Ctx(t), &messages.StatusMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2",
		Status: "Start", Progress: 40}, srvData)
	assert.Nil(t, err)
	require.Equal(t, 2, len(dbMock.Calls)) // with save
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
		{name: "OK", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock, TranscriberPr: transcriberPrMock, UsageRestorer: uRestorerMock}}, wantErr: false},
		{name: "Fail no data", args: args{data: &ServiceData{GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock, TranscriberPr: transcriberPrMock, UsageRestorer: uRestorerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock, TranscriberPr: transcriberPrMock, UsageRestorer: uRestorerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, MsgSender: senderMock,
			Filer: filerMock, TranscriberPr: transcriberPrMock, UsageRestorer: uRestorerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10,
			Filer: filerMock, TranscriberPr: transcriberPrMock, UsageRestorer: uRestorerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
		TranscriberPr: transcriberPrMock, UsageRestorer: uRestorerMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock}}, wantErr: true},
		{name: "No usage restorer", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock, TranscriberPr: transcriberPrMock}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("StartServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_prepareParams(t *testing.T) {
	tests := []struct {
		name string
		args map[string]string
		want map[string]string
	}{
		{name: "Empty", args: map[string]string{}, want: map[string]string{}},
		{name: "Drops email", args: map[string]string{api.PrmEmail: "olia"}, want: map[string]string{}},
		{name: "Drops email", args: map[string]string{api.PrmEmail: "olia", api.PrmRecognizer: "ben"}, want: map[string]string{api.PrmRecognizer: "ben"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prepareParams(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("prepareParams() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_limit(t *testing.T) {
	type args struct {
		s string
		l int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := limit(tt.args.s, tt.args.l); got != tt.want {
				t.Errorf("limit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isCompleted(t *testing.T) {
	type args struct {
		st     string
		errStr string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "Completed", args: args{st: "COMPLETED"}, want: true},
		{name: "Completed err", args: args{st: "Olia", errStr: "err"}, want: true},
		{name: "Not completed", args: args{st: "Upload", errStr: ""}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCompleted(tt.args.st, tt.args.errStr); got != tt.want {
				t.Errorf("isCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockUsageRestorer struct{ mock.Mock }

func (m *mockUsageRestorer) Do(ctx context.Context, msgID, reqID, errStr string) error {
	args := m.Called(ctx, msgID, reqID, errStr)
	return args.Error(0)
}
