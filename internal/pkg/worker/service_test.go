package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/test/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vgarvardt/gue/v5"
)

var (
	filerMock       *mocks.Filer
	dbMock          *mocks.DB
	senderMock      *mocks.Sender
	transcriberMock *mocks.Transcriber
	srvData         *ServiceData
)

func initTest(t *testing.T) {
	filerMock = &mocks.Filer{}
	dbMock = &mocks.DB{}
	senderMock = &mocks.Sender{}
	transcriberMock = &mocks.Transcriber{}
	srvData = &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
		Filer: filerMock, Transcriber: transcriberMock}
	transcriberMock.On("Clean", mock.Anything, mock.Anything).Return(nil)
}

func Test_handleClean(t *testing.T) {
	initTest(t)
	err := handleClean(testCtx(t), &messages.CleanMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2"}, srvData)
	assert.Nil(t, err)
}

func Test_handleClean_Fail(t *testing.T) {
	initTest(t)
	transcriberMock.ExpectedCalls = nil
	transcriberMock.On("Clean", mock.Anything, mock.Anything).Return(fmt.Errorf("olia err"))
	err := handleClean(testCtx(t), &messages.CleanMessage{QueueMessage: amessages.QueueMessage{ID: "1"}, ExternalID: "2"}, srvData)
	assert.NotNil(t, err)
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
			Filer: filerMock, Transcriber: transcriberMock}}, wantErr: false},
		{name: "Fail no data", args: args{data: &ServiceData{GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock, Transcriber: transcriberMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock, Transcriber: transcriberMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, MsgSender: senderMock,
			Filer: filerMock, Transcriber: transcriberMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10,
			Filer: filerMock, Transcriber: transcriberMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
			Transcriber: transcriberMock}}, wantErr: true},
		{name: "Fail no data", args: args{data: &ServiceData{DB: dbMock, GueClient: &gue.Client{}, WorkerCount: 10, MsgSender: senderMock,
			Filer: filerMock}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("StartServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cf := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(func() { cf() })
	return ctx
}
