package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/airenas/roxy/internal/pkg/utils"
	"github.com/vgarvardt/gue/v5"
)

// MsgSender provides send msg functionality
type MsgSender interface {
	SendMessage(context.Context, amessages.Message, string) error
}

// DB provides persistnce functionality
type DB interface {
	LoadRequest(ctx context.Context, id string) (*persistence.ReqData, error)
	LoadStatus(ctx context.Context, id string) (*persistence.Status, error)
	LoadWorkData(ctx context.Context, id string) (*persistence.WorkData, error)
	InsertWorkData(context.Context, *persistence.WorkData) error
	UpdateStatus(context.Context, *persistence.Status) error
}

// Filer retrieves files
type Filer interface {
	LoadFile(ctx context.Context, fileName string) (io.ReadSeekCloser, error)
	SaveFile(ctx context.Context, name string, r io.Reader) error
}

// Transcriber provides transcription
type Transcriber interface {
	Upload(ctx context.Context, audio *tapi.UploadData) (string, error)
	HookToStatus(ctx context.Context, ID string) (<-chan tapi.StatusData, func(), error)
	GetAudio(ctx context.Context, ID string) (*tapi.FileData, error)
	GetResult(ctx context.Context, ID, name string) (*tapi.FileData, error)
	Clean(ctx context.Context, ID string) error
}

// ServiceData keeps data required for service work
type ServiceData struct {
	GueClient   *gue.Client
	WorkerCount int
	MsgSender   MsgSender
	DB          DB
	Filer       Filer
	Transcriber Transcriber
}

const (
	wrkQueuePrefix = messages.Upload + ":"
	wrkStatusQueue = "wrk-status"
	wrkStatusClean = "wrk-clean"
)

// StartWorkerService starts the event queue listener service to listen for events
// returns channel for tracking if all jobs are finished
func StartWorkerService(ctx context.Context, data *ServiceData) (chan struct{}, error) {
	if err := validate(data); err != nil {
		return nil, err
	}
	goapp.Log.Info().Msg("Starting listen for messages")

	wm := gue.WorkMap{
		messages.Upload: utils.CreateHandler(data, handleASR),
		wrkStatusQueue:  utils.CreateHandler(data, handleStatus),
		wrkStatusClean:  utils.CreateHandler(data, handleClean),
	}

	pool, err := gue.NewWorkerPool(
		data.GueClient, wm, data.WorkerCount,
		gue.WithPoolQueue(messages.Upload),
		gue.WithPoolLogger(utils.NewGueLoggerAdapter()),
		gue.WithPoolPollInterval(500*time.Millisecond),
		gue.WithPoolPollStrategy(gue.RunAtPollStrategy),
		gue.WithPoolID("asr-worker"),
	)
	if err != nil {
		return nil, fmt.Errorf("could not build gue workers pool: %w", err)
	}
	res := make(chan struct{}, 1)
	go func() {
		goapp.Log.Info().Msg("Starting workers")
		if err := pool.Run(ctx); err != nil {
			goapp.Log.Error().Err(err).Msg("pool error")
		}
		goapp.Log.Info().Msg("Pool workers finished")
		res <- struct{}{}
	}()
	return res, nil
}

func handleASR(ctx context.Context, m *messages.ASRMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Msg("handling")
	err := data.MsgSender.SendMessage(ctx, amessages.InformMessage{
		QueueMessage: *amessages.NewQueueMessageFromM(&m.QueueMessage),
		Type:         amessages.InformTypeStarted, At: time.Now()}, messages.Inform)
	if err != nil {
		return fmt.Errorf("can't send msg: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("load request")
	req, err := data.DB.LoadRequest(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load request: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msgf("loaded %v", req)
	goapp.Log.Info().Str("ID", m.ID).Msg("load work data")
	wd, err := data.DB.LoadWorkData(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load work data: %w", err)
	}
	if wd == nil {
		extID, err := upload(ctx, req, data)
		if err != nil {
			return fmt.Errorf("can't upload: %w", err)
		}
		wd = &persistence.WorkData{ID: req.ID, ExternalID: extID, Created: time.Now()}
		err = data.DB.InsertWorkData(ctx, wd)
		if err != nil {
			return fmt.Errorf("can't save work data: %w", err)
		}
	} else {
		goapp.Log.Info().Str("ID", m.ID).Msgf("loaded %v", wd)
	}
	// wait for finish
	err = waitStatus(ctx, wd.ID, wd.ExternalID, data)
	if err != nil {
		return fmt.Errorf("can't wait for finish: %w", err)
	}
	goapp.Log.Info().Str("ID", wd.ID).Msg("Transcription completed")
	return nil
}

func handleStatus(ctx context.Context, m *messages.StatusMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Str("extID", m.ExternalID).Msg("handling")
	goapp.Log.Info().Str("ID", m.ID).Msg("load status")
	status, err := data.DB.LoadStatus(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load status: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msgf("loaded %v", status)
	if m.AudioReady && !status.AudioReady {
		goapp.Log.Info().Str("ID", m.ExternalID).Msg("get audio")
		f, err := data.Transcriber.GetAudio(ctx, m.ExternalID)
		if err != nil {
			return fmt.Errorf("can't get audio: %w", err)
		}
		err = data.Filer.SaveFile(ctx, f.Name, bytes.NewReader(f.Content))
		if err != nil {
			return fmt.Errorf("can't save file: %w", err)
		}
		status.AudioReady = true
	}
	if len(m.AvailableResults) != len(status.AvailableResults) {
		for _, fn := range m.AvailableResults {
			goapp.Log.Info().Str("ID", m.ID).Str("file", fn).Msg("get data")
			f, err := data.Transcriber.GetResult(ctx, m.ExternalID, fn)
			if err != nil {
				return fmt.Errorf("can't get data: %w", err)
			}
			err = data.Filer.SaveFile(ctx, fmt.Sprintf("%s/%s", m.ID, f.Name), bytes.NewReader(f.Content))
			if err != nil {
				return fmt.Errorf("can't save file: %w", err)
			}
		}
		status.AvailableResults = m.AvailableResults
	}
	status.Error.String = m.Error
	status.ErrorCode.String = m.ErrorCode
	status.Progress.Int32 = int32(m.Progress)
	status.Status = m.Status
	if err := data.DB.UpdateStatus(ctx, status); err != nil {
		return fmt.Errorf("can't save status: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("Status update completed")
	err = data.MsgSender.SendMessage(ctx, messages.ASRMessage{
		QueueMessage: amessages.QueueMessage{ID: m.ID}}, messages.StatusChange)
	if err != nil {
		return fmt.Errorf("can't send msg: %w", err)
	}
	if isCompleted(m.Status, m.Error) {
		err = data.MsgSender.SendMessage(ctx, messages.CleanMessage{
			QueueMessage: amessages.QueueMessage{ID: m.ID}, ExternalID: m.ExternalID},
			wrkQueuePrefix+wrkStatusClean)
		if err != nil {
			return fmt.Errorf("can't send msg: %w", err)
		}
	}
	return nil
}

func handleClean(ctx context.Context, m *messages.CleanMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Str("extID", m.ExternalID).Msg("handling")
	err := data.Transcriber.Clean(ctx, m.ExternalID)
	if err != nil {
		return fmt.Errorf("can't clean external data: %w", err)
	}
	return nil
}

func isCompleted(st, errStr string) bool {
	return st == "COMPLETED" || errStr != ""
}

func waitStatus(ctx context.Context, ID, extID string, data *ServiceData) error {
	stCh, cf, err := data.Transcriber.HookToStatus(ctx, extID)
	if err != nil {
		return fmt.Errorf("can't hook to status: %w", err)
	}
	defer cf()
	for {
		select {
		case <-ctx.Done():
			goapp.Log.Info().Msg("exit status channel loop")
			return nil
		case d, ok := <-stCh:
			{
				if !ok {
					goapp.Log.Info().Msg("closed status channel")
					return nil
				}
				finish, err := processStatus(ctx, &d, extID, ID, data)
				if err != nil {
					return fmt.Errorf("can't process status: %w", err)
				}
				if finish {
					return nil
				}
			}
		}
	}
}

func processStatus(ctx context.Context, statusData *tapi.StatusData, extID, ID string, data *ServiceData) (bool, error) {
	goapp.Log.Info().Str("status", statusData.Status).Str("ID", ID).Msg("status")
	err := data.MsgSender.SendMessage(ctx, messages.StatusMessage{
		QueueMessage:     amessages.QueueMessage{ID: ID},
		Status:           statusData.Status,
		Error:            statusData.Error,
		Progress:         statusData.Progress,
		ErrorCode:        statusData.ErrorCode,
		AudioReady:       statusData.AudioReady,
		AvailableResults: statusData.AvResults,
		ExternalID:       extID,
	}, wrkQueuePrefix+wrkStatusQueue)
	if err != nil {
		return false, fmt.Errorf("can't send msg: %w", err)
	}
	return isCompleted(statusData.Status, statusData.Error), nil
}

func upload(ctx context.Context, req *persistence.ReqData, data *ServiceData) (string, error) {
	goapp.Log.Info().Str("ID", req.ID).Msg("load file")
	file, err := data.Filer.LoadFile(ctx, req.ID+".wav")
	_ = req
	if err != nil {
		return "", fmt.Errorf("can't load file: %w", err)
	}
	defer file.Close()
	goapp.Log.Info().Str("ID", req.ID).Msg("loaded")
	goapp.Log.Info().Str("ID", req.ID).Msg("uploading")
	extID, err := data.Transcriber.Upload(ctx, &tapi.UploadData{Params: req.Params, Files: map[string]io.Reader{req.ID + ".wav": file}})
	if err != nil {
		return "", fmt.Errorf("can't upload: %w", err)
	}
	return extID, nil
}

func validate(data *ServiceData) error {
	if data.GueClient == nil {
		return fmt.Errorf("no gue client")
	}
	if data.WorkerCount < 1 {
		return fmt.Errorf("no worker count provided")
	}
	if data.MsgSender == nil {
		return fmt.Errorf("no msg sender")
	}
	if data.Filer == nil {
		return fmt.Errorf("no Filer")
	}
	if data.DB == nil {
		return fmt.Errorf("no DB")
	}
	if data.Transcriber == nil {
		return fmt.Errorf("no Transcriber")
	}
	return nil
}
