package mocks

import (
	"context"
	"io"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/stretchr/testify/mock"
)

// Filer is minio mock
type Filer struct{ mock.Mock }

func (m *Filer) SaveFile(ctx context.Context, name string, r io.Reader, fileSize int64) error {
	args := m.Called(ctx, name, r, fileSize)
	return args.Error(0)
}

// LoadFile func mock
func (m *Filer) LoadFile(ctx context.Context, fileName string) (io.ReadSeekCloser, error) {
	args := m.Called(ctx, fileName)
	return To[io.ReadSeekCloser](args.Get(0)), args.Error(1)
}

// DB is postgress DB mock
type DB struct{ mock.Mock }

// LoadFile func mock
func (m *DB) InsertRequest(ctx context.Context, req *persistence.ReqData) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

// LoadFile func mock
func (m *DB) InsertStatus(ctx context.Context, req *persistence.Status) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *DB) LoadRequest(ctx context.Context, id string) (*persistence.ReqData, error) {
	args := m.Called(ctx, id)
	return To[*persistence.ReqData](args.Get(0)), args.Error(1)
}
func (m *DB) LoadStatus(ctx context.Context, id string) (*persistence.Status, error) {
	args := m.Called(ctx, id)
	return To[*persistence.Status](args.Get(0)), args.Error(1)
}
func (m *DB) LoadWorkData(ctx context.Context, id string) (*persistence.WorkData, error) {
	args := m.Called(ctx, id)
	return To[*persistence.WorkData](args.Get(0)), args.Error(1)
}
func (m *DB) InsertWorkData(ctx context.Context, data *persistence.WorkData) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}
func (m *DB) UpdateWorkData(ctx context.Context, data *persistence.WorkData) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}
func (m *DB) DeleteWorkData(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *DB) UpdateStatus(ctx context.Context, data *persistence.Status) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}
func (m *DB) LockEmailTable(ctx context.Context, id, lType string) error {
	args := m.Called(ctx, id, lType)
	return args.Error(0)
}
func (m *DB) UnLockEmailTable(ctx context.Context, id, lType string, val int) error {
	args := m.Called(ctx, id, lType, val)
	return args.Error(0)
}

// Sender is postgres queue mock
type Sender struct{ mock.Mock }

func (m *Sender) SendMessage(ctx context.Context, msg amessages.Message, opt *messages.Options) error {
	args := m.Called(ctx, msg, opt)
	return args.Error(0)
}

// Transcriber is transcription client mock
type Transcriber struct{ mock.Mock }

func (m *Transcriber) Upload(ctx context.Context, audioFunc func(context.Context) (*api.UploadData, func(), error)) (string, error) {
	args := m.Called(ctx, audioFunc)
	return args.String(0), args.Error(1)
}

func (m *Transcriber) HookToStatus(ctx context.Context, ID string) (<-chan api.StatusData, func(), error) {
	args := m.Called(ctx, ID)
	return To[<-chan api.StatusData](args.Get(0)), To[func()](args.Get(1)), args.Error(2)
}

func (m *Transcriber) GetStatus(ctx context.Context, ID string) (*api.StatusData, error) {
	args := m.Called(ctx, ID)
	return args.Get(0).(*api.StatusData), args.Error(1)
}

func (m *Transcriber) GetAudio(ctx context.Context, ID string) (*api.FileData, error) {
	args := m.Called(ctx, ID)
	return To[*api.FileData](args.Get(0)), args.Error(1)
}

func (m *Transcriber) GetResult(ctx context.Context, ID, name string) (*api.FileData, error) {
	args := m.Called(ctx, ID, name)
	return args.Get(0).(*api.FileData), args.Error(1)
}

func (m *Transcriber) Clean(ctx context.Context, ID string) error {
	args := m.Called(ctx, ID)
	return args.Error(0)
}

// TranscriberProvider is a provider mock
type TranscriberProvider struct{ mock.Mock }

func (m *TranscriberProvider) Get(key string, allowNew bool) (api.Transcriber, string, error) {
	args := m.Called(key, allowNew)
	return To[api.Transcriber](args.Get(0)), args.String(1), args.Error(2)
}

// To convert interface to object
func To[T interface{}](val interface{}) T {
	if val == nil {
		var res T
		return res
	}
	return val.(T)
}
