package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/api"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/status"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/airenas/roxy/internal/pkg/utils"
	"github.com/airenas/roxy/internal/pkg/utils/handler"
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
	GetStatus(ctx context.Context, ID string) (*tapi.StatusData, error)
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
	Testing     bool
}

const (
	wrkQueuePrefix = messages.Work + ":"
	wrkStatusQueue = "wrk-status"
	wrkStatusClean = "wrk-clean"
	wrkStatusFail  = "wrk-fail"
	wrkUpload      = "wrk-upload"
)

// StartWorkerService starts the event queue listener service to listen for events
// returns channel for tracking if all jobs are finished
func StartWorkerService(ctx context.Context, data *ServiceData) (chan struct{}, error) {
	if err := validate(data); err != nil {
		return nil, err
	}
	goapp.Log.Info().Int("workers", data.WorkerCount).Msg("Starting listen for messages")
	if data.Testing {
		goapp.Log.Warn().Msg("SERVICE IN TEST MODE")
	}

	wm := gue.WorkMap{
		wrkUpload: handler.Create(data, handleASR, handler.DefaultOpts().WithFailure(data.MsgSender).
			WithTimeout(time.Minute*120).WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
		wrkStatusQueue: handler.Create(data, handleStatus, handler.DefaultOpts().WithFailure(data.MsgSender).
			WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
		wrkStatusClean: handler.Create(data, handleClean, handler.DefaultOpts().WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
		wrkStatusFail:  handler.Create(data, handleFailure, handler.DefaultOpts().WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
	}

	pool, err := gue.NewWorkerPool(
		data.GueClient, wm, data.WorkerCount,
		gue.WithPoolQueue(messages.Work),
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
	goapp.Log.Info().Str("ID", m.ID).Msg("handling asr")
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
		name := filepath.Join(m.ID, m.ID+".mp3") // only such case now supported
		err = data.Filer.SaveFile(ctx, name, bytes.NewReader(f.Content))
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
	status.Error = utils.ToSQLStr(m.Error)
	status.ErrorCode = utils.ToSQLStr(m.ErrorCode)
	status.Progress.Int32 = int32(m.Progress)
	status.Status = m.Status
	status.RecognizedText = utils.ToSQLStr(m.RecognizedText)
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
		err := data.MsgSender.SendMessage(ctx, amessages.InformMessage{
			QueueMessage: *amessages.NewQueueMessageFromM(&m.QueueMessage),
			Type:         getInformFinishType(m.Error), At: time.Now()}, messages.Inform)
		if err != nil {
			return fmt.Errorf("can't send msg: %w", err)
		}
	}
	return nil
}

func getInformFinishType(errStr string) string {
	if errStr == "" {
		return amessages.InformTypeFinished
	}
	return amessages.InformTypeFailed
}

func handleFailure(ctx context.Context, m *messages.ASRMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Msg("handling failure")
	goapp.Log.Info().Str("ID", m.ID).Msg("load status")
	statusRec, err := data.DB.LoadStatus(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load status: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msgf("loaded %v", statusRec)
	if statusRec.Error.String != "" {
		goapp.Log.Info().Str("ID", m.ID).Msg("error set - ignore")
	}
	statusRec.Error = utils.ToSQLStr(m.Error)
	statusRec.ErrorCode = utils.ToSQLStr(status.Failure.String())
	if err := data.DB.UpdateStatus(ctx, statusRec); err != nil {
		return fmt.Errorf("can't save status: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("Status update completed")
	goapp.Log.Info().Str("ID", m.ID).Msg("send status change")
	err = data.MsgSender.SendMessage(ctx, messages.ASRMessage{
		QueueMessage: amessages.QueueMessage{ID: m.ID}}, messages.StatusChange)
	if err != nil {
		return fmt.Errorf("can't send msg: %w", err)
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
	manualCheck := time.Second * 20
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
		case <-time.After(manualCheck):
			{
				goapp.Log.Info().Msg("manual status check")
				d, err := data.Transcriber.GetStatus(ctx, extID)
				if err != nil {
					return fmt.Errorf("can't get status: %w", err)
				}
				finish, err := processStatus(ctx, d, extID, ID, data)
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
		RecognizedText:   statusData.RecognizedText,
		ExternalID:       extID,
	}, wrkQueuePrefix+wrkStatusQueue)
	if err != nil {
		return false, fmt.Errorf("can't send msg: %w", err)
	}
	return isCompleted(statusData.Status, statusData.Error), nil
}

func upload(ctx context.Context, req *persistence.ReqData, data *ServiceData) (string, error) {
	goapp.Log.Info().Str("ID", req.ID).Msg("load file")
	filesMap := map[string]io.Reader{}
	files := []io.ReadCloser{}
	defer func() {
		for _, r := range files {
			_ = r.Close()
		}
	}()
	for _, f := range req.FileNames {
		file, err := data.Filer.LoadFile(ctx, utils.MakeFileName(req.ID, f))
		if err != nil {
			return "", fmt.Errorf("can't load file: %w", err)
		}
		files = append(files, file)
		filesMap[f] = file
		goapp.Log.Info().Str("ID", req.ID).Msg("loaded")
	}
	goapp.Log.Info().Str("ID", req.ID).Msg("uploading")
	extID, err := data.Transcriber.Upload(ctx, &tapi.UploadData{Params: prepareParams(req.Params), Files: filesMap})
	if err != nil {
		return "", fmt.Errorf("can't upload: %w", err)
	}
	return extID, nil
}

// prepareParams drops email
func prepareParams(in map[string]string) map[string]string {
	res := map[string]string{}
	for k, v := range in {
		if k != api.PrmEmail {
			res[k] = v
		}
	}
	return res
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
