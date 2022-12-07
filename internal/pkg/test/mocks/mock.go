package mocks

import (
	"context"
	"io"

	"github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/stretchr/testify/mock"
)

// Filer is minio mock
type Filer struct{ mock.Mock }

func (m *Filer) SaveFile(ctx context.Context, name string, r io.Reader) error {
	args := m.Called(ctx, name, r)
	return args.Error(0)
}

// LoadFile func mock
func (m *Filer) LoadFile(ctx context.Context, fileName string) (io.ReadSeekCloser, error) {
	args := m.Called(ctx, fileName)
	return to[io.ReadSeekCloser](args.Get(0)), args.Error(1)
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
	return args.Get(0).(*persistence.ReqData), args.Error(1)
}
func (m *DB) LoadStatus(ctx context.Context, id string) (*persistence.Status, error) {
	args := m.Called(ctx, id)
	return to[*persistence.Status](args.Get(0)), args.Error(1)
}
func (m *DB) LoadWorkData(ctx context.Context, id string) (*persistence.WorkData, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*persistence.WorkData), args.Error(1)
}
func (m *DB) InsertWorkData(ctx context.Context, data *persistence.WorkData) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}
func (m *DB) UpdateStatus(ctx context.Context, data *persistence.Status) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

// Sender is postgres queue mock
type Sender struct{ mock.Mock }

func (m *Sender) SendMessage(ctx context.Context, msg messages.Message, queue string) error {
	args := m.Called(ctx, msg, queue)
	return args.Error(0)
}

// Transcriber is transcription client mock
type Transcriber struct{ mock.Mock }

func (m *Transcriber) Upload(ctx context.Context, audio *api.UploadData) (string, error) {
	args := m.Called(ctx, audio)
	return args.String(0), args.Error(1)
}

func (m *Transcriber) HookToStatus(ctx context.Context, ID string) (<-chan api.StatusData, func(), error) {
	args := m.Called(ctx, ID)
	return args.Get(0).(<-chan api.StatusData), args.Get(1).(func()), args.Error(1)
}

func (m *Transcriber) GetAudio(ctx context.Context, ID string) (*api.FileData, error) {
	args := m.Called(ctx, ID)
	return args.Get(0).(*api.FileData), args.Error(1)
}

func (m *Transcriber) GetResult(ctx context.Context, ID, name string) (*api.FileData, error) {
	args := m.Called(ctx, ID, name)
	return args.Get(0).(*api.FileData), args.Error(1)
}

func (m *Transcriber) Clean(ctx context.Context, ID string) error {
	args := m.Called(ctx, ID)
	return args.Error(0)
}

func to[T interface{}](val interface{}) T {
	if val == nil {
		var res T
		return res
	}
	return val.(T)
}
