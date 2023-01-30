package worker

import (
	"bytes"
	"context"
	"errors"
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
	SendMessage(context.Context, amessages.Message, *messages.Options) error
}

// DB provides persistnce functionality
type DB interface {
	LoadRequest(ctx context.Context, id string) (*persistence.ReqData, error)
	LoadStatus(ctx context.Context, id string) (*persistence.Status, error)
	LoadWorkData(ctx context.Context, id string) (*persistence.WorkData, error)
	InsertWorkData(context.Context, *persistence.WorkData) error
	UpdateStatus(context.Context, *persistence.Status) error
	UpdateWorkData(context.Context, *persistence.WorkData) error
}

// Filer retrieves files
type Filer interface {
	LoadFile(ctx context.Context, fileName string) (io.ReadSeekCloser, error)
	SaveFile(ctx context.Context, name string, r io.Reader, fileSize int64) error
}

type UsageRestorer interface {
	Do(ctx context.Context, msgID, reqID, errStr string) error
}

// TranscriberProvider provides transcriber
type TranscriberProvider interface {
	Get(key string, allowOther bool) (tapi.Transcriber, string, error)
}

// ServiceData keeps data required for service work
type ServiceData struct {
	GueClient        *gue.Client
	WorkerCount      int // ASR worker
	WorkerOtherCount int // for handling statuses, etc
	MsgSender        MsgSender
	DB               DB
	Filer            Filer
	TranscriberPr    TranscriberProvider
	UsageRestorer    UsageRestorer
	Testing          bool
	RetryDelay       time.Duration
}

const (
	wrkQueuePrefix  = messages.Work + ":"
	wrkStatusQueue  = "wrk-status"
	wrkStatusClean  = "wrk-clean"
	wrkStatusFail   = "wrk-fail"
	wrkUpload       = "wrk-upload"
	wrkRestoreUsage = "wrk-restore-usage"
)

// StartWorkerService starts the event queue listener service to listen for events
// returns channel for tracking if all jobs are finished
func StartWorkerService(ctx context.Context, data *ServiceData) (chan struct{}, error) {
	if err := validate(data); err != nil {
		return nil, err
	}
	goapp.Log.Info().Int("workers", data.WorkerCount).Int("workersOther", data.WorkerOtherCount).Msg("Starting listen for messages")
	if data.Testing {
		goapp.Log.Warn().Msg("SERVICE IN TEST MODE")
	}
	goapp.Log.Info().Dur("delay", data.RetryDelay).Msg("cfg: retry on no transcriber after")

	wm := gue.WorkMap{
		wrkUpload: handler.Create(data, handleASR, handler.DefaultOpts[messages.ASRMessage]().WithFailure(asrFailureHandler(data)).
			WithTimeout(time.Minute*120).WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
	}

	ctxInt, cf := context.WithCancel(ctx)
	defer func () { cf() } ()
	ecASR, err := startPool(ctxInt, data, data.WorkerCount, wm, "asr-worker")
	if err != nil {
		return nil, fmt.Errorf("could not start pool: %w", err)
	}
	wmOther := gue.WorkMap{
		wrkStatusQueue: handler.Create(data, handleStatus, handler.DefaultOpts[messages.StatusMessage]().WithFailure(statusFailureHandler(data)).
			WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
		wrkStatusClean:  handler.Create(data, handleClean, handler.DefaultOpts[messages.CleanMessage]().WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
		wrkStatusFail:   handler.Create(data, handleFailure, handler.DefaultOpts[messages.ASRMessage]().WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
		wrkRestoreUsage: handler.Create(data, handleRestoreUsage, handler.DefaultOpts[messages.ASRMessage]().WithBackoff(handler.DefaultBackoffOrTest(data.Testing))),
	}
	ecOther, err := startPool(ctxInt, data, data.WorkerOtherCount, wmOther, "asr-worker-other")
	if err != nil {
		return nil, fmt.Errorf("could not start pool: %w", err)
	}
	res := make(chan struct{}, 1)
	cfInt := cf
	cf = func() {}
	go func() {
		// wait for any pool to finish
		select {
		case <-ecASR:
		case <-ecOther:
		}
		// drop context
		cfInt()
		// wait for both pool to finish
		<-ecASR
		<-ecOther
		goapp.Log.Info().Msg("pool workers finished")
		close(res)
	}()
	return res, nil
}

func startPool(ctx context.Context, data *ServiceData, count int, wm gue.WorkMap, name string) (<-chan struct{}, error) {
	pool, err := gue.NewWorkerPool(
		data.GueClient, wm, count,
		gue.WithPoolQueue(messages.Work),
		gue.WithPoolLogger(utils.NewGueLoggerAdapter()),
		gue.WithPoolPollInterval(500*time.Millisecond),
		gue.WithPoolPollStrategy(gue.RunAtPollStrategy),
		gue.WithPoolID(name),
	)
	if err != nil {
		return nil, fmt.Errorf("could not build gue workers pool: %w", err)
	}
	res := make(chan struct{}, 1)
	go func() {
		goapp.Log.Info().Int("workers", count).Str("name", name).Msg("Starting pool")
		if err := pool.Run(ctx); err != nil {
			goapp.Log.Error().Err(err).Msg("pool error")
		}
		goapp.Log.Info().Str("name", name).Msg("exit workers")
		close(res)
	}()
	return res, nil
}

func handleASR(ctx context.Context, m *messages.ASRMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Msg("handling asr")
	err := data.MsgSender.SendMessage(ctx, &amessages.InformMessage{
		QueueMessage: *amessages.NewQueueMessageFromM(&m.QueueMessage),
		Type:         amessages.InformTypeStarted, At: time.Now()}, messages.DefaultOpts(messages.Inform))
	if err != nil {
		return fmt.Errorf("can't send msg: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("load request")
	req, err := data.DB.LoadRequest(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load request: %w", err)
	}
	goapp.Log.Debug().Str("ID", m.ID).Msgf("loaded %v", req)
	goapp.Log.Info().Str("ID", m.ID).Msg("load work data")
	wd, err := data.DB.LoadWorkData(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load work data: %w", err)
	}
	goapp.Log.Debug().Str("ID", m.ID).Msgf("loaded %v", wd)
	oldSrv := ""
	if wd != nil {
		oldSrv = utils.FromSQLStr(wd.Transcriber)
	}
	transcriber, trSrv, err := data.TranscriberPr.Get(oldSrv, true)
	if err != nil {
		return fmt.Errorf("can't get transcriber: %w", err)
	}
	if transcriber == nil {
		goapp.Log.Info().Str("ID", m.ID).Msg("no available transcribers")
		maxWaitTime := time.Hour * 12
		if req.Created.Add(maxWaitTime).Before(time.Now()) {
			goapp.Log.Info().Str("ID", m.ID).Msg("already to late")
			return fmt.Errorf("no available transcriber in %s, added %s", maxWaitTime.String(), req.Created.Format(time.RFC3339))
		}
		// sleep 1 min
		err := data.MsgSender.SendMessage(ctx, m, messages.DefaultOpts(messages.Upload).Delay(data.RetryDelay))
		if err != nil {
			return fmt.Errorf("can't send msg: %w", err)
		}
		return nil
	}

	if wd == nil {
		extID, err := upload(ctx, req, transcriber, data)
		if err != nil {
			return fmt.Errorf("can't upload: %w", err)
		}
		wd = &persistence.WorkData{ID: req.ID, ExternalID: extID, Created: time.Now(),
			Transcriber: utils.ToSQLStr(trSrv), TryCount: 1}
		err = data.DB.InsertWorkData(ctx, wd)
		if err != nil {
			return fmt.Errorf("can't save work data: %w", err)
		}
	} else if oldSrv != trSrv {
		if wd.TryCount > 3 {
			goapp.Log.Info().Str("ID", m.ID).Msg("too many retries")
			return fmt.Errorf("too many retries: %d", wd.TryCount)
		}
		goapp.Log.Info().Str("ID", m.ID).Str("old", oldSrv).Str("new", trSrv).Msgf("try new srv")
		extID, err := upload(ctx, req, transcriber, data)
		if err != nil {
			return fmt.Errorf("can't upload: %w", err)
		}
		wd.ExternalID = extID
		wd.Transcriber = utils.ToSQLStr(trSrv)
		wd.TryCount += 1
		err = data.DB.UpdateWorkData(ctx, wd)
		if err != nil {
			return fmt.Errorf("can't update work data: %w", err)
		}
	} else {
		if wd.TryCount > 5 {
			goapp.Log.Info().Str("ID", m.ID).Msg("too many retries for same transcriber")
			return fmt.Errorf("too many retries: %d", wd.TryCount)
		}
		wd.TryCount += 1
		err = data.DB.UpdateWorkData(ctx, wd)
		if err != nil {
			return fmt.Errorf("can't update work data: %w", err)
		}
	}
	// wait for finish
	err = waitStatus(ctx, wd, data)
	if err != nil {
		return fmt.Errorf("can't wait for finish: %w", err)
	}
	goapp.Log.Info().Str("ID", wd.ID).Msg("Transcription completed")
	return nil
}

func asrFailureHandler(data *ServiceData) func(context.Context, *messages.ASRMessage, error) error {
	return func(ctx context.Context, m *messages.ASRMessage, err error) error {
		if _err := sendStatusChangeFailure(ctx, data.MsgSender, m.ID, err.Error()); _err != nil {
			goapp.Log.Error().Err(_err).Str("ID", m.ID).Msg("fail send status failure msg")
		}
		if _err := sendFailure(ctx, data.MsgSender, m.ID); _err != nil {
			goapp.Log.Error().Err(_err).Str("ID", m.ID).Msg("fail send failure msg")
		}
		return nil
	}
}

type errTranscriber struct {
	err error
}

func (e *errTranscriber) Error() string {
	return fmt.Sprintf("transcriber error: %v", e.err)
}

func (e *errTranscriber) Unwrap() error {
	return e.err
}

func statusFailureHandler(data *ServiceData) func(context.Context, *messages.StatusMessage, error) error {
	return func(ctx context.Context, m *messages.StatusMessage, err error) error {
		tErr := &errTranscriber{}
		if errors.As(err, &tErr) {
			goapp.Log.Info().Str("ID", m.ID).Msg("retry transcription - transcriber result retrieve error")
			err := data.MsgSender.SendMessage(ctx, &messages.ASRMessage{
				QueueMessage: amessages.QueueMessage{ID: m.ID}}, messages.DefaultOpts(messages.Upload))
			if err != nil {
				return fmt.Errorf("can't send msg: %w", err)
			}
		} else {
			if _err := sendStatusChangeFailure(ctx, data.MsgSender, m.ID, err.Error()); _err != nil {
				goapp.Log.Error().Err(_err).Str("ID", m.ID).Msg("fail send status failure msg")
			}
			if _err := sendFailure(ctx, data.MsgSender, m.ID); _err != nil {
				goapp.Log.Error().Err(_err).Str("ID", m.ID).Msg("fail send failure msg")
			}
		}
		return nil
	}
}

func sendFailure(ctx context.Context, sender MsgSender, ID string) error {
	goapp.Log.Info().Str("ID", ID).Msg("sending failure msg")
	return sender.SendMessage(ctx, &amessages.InformMessage{
		QueueMessage: amessages.QueueMessage{ID: ID},
		Type:         amessages.InformTypeFailed, At: time.Now()}, messages.DefaultOpts(messages.Inform))
}

func sendStatusChangeFailure(ctx context.Context, sender MsgSender, ID string, errStr string) error {
	goapp.Log.Info().Str("ID", ID).Msg("sending failure status change msg")
	return sender.SendMessage(ctx, &messages.ASRMessage{
		QueueMessage: amessages.QueueMessage{ID: ID, Error: errStr}}, messages.DefaultOpts(messages.Fail))
}

func handleStatus(ctx context.Context, m *messages.StatusMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Str("extID", m.ExternalID).Str("status", m.Status).Msg("handling")
	transcriber, _, err := data.TranscriberPr.Get(m.Transcriber, false)
	if err != nil {
		return fmt.Errorf("can't get active transcriber by `%s`: %w", m.Transcriber, err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("load status")
	status, err := data.DB.LoadStatus(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load status: %w", err)
	}
	// do not update if status is older
	// but make exception if status update was long time ago - maybe retry
	if int32(m.Progress) < utils.FromSQLInt32OrZero(status.Progress) && status.Updated.After(time.Now().Add(-30*time.Minute)) {
		goapp.Log.Warn().Str("ID", m.ID).Str("status", m.Status).Int32("oldProgress", utils.FromSQLInt32OrZero(status.Progress)).Int("progress", m.Progress).Msg("obsolete")
		return nil
	}

	goapp.Log.Debug().Str("ID", m.ID).Msgf("loaded %v", status)
	if m.AudioReady && !status.AudioReady {
		goapp.Log.Info().Str("ID", m.ExternalID).Msg("get audio")
		f, err := transcriber.GetAudio(ctx, m.ExternalID)
		if err != nil {
			return &errTranscriber{err: fmt.Errorf("can't get audio: %w", err)}
		}
		name := filepath.Join(m.ID, m.ID+".mp3") // only such case now supported
		err = data.Filer.SaveFile(ctx, name, bytes.NewReader(f.Content), int64(len(f.Content)))
		if err != nil {
			return fmt.Errorf("can't save file: %w", err)
		}
		status.AudioReady = true
	}
	if len(m.AvailableResults) != len(status.AvailableResults) {
		for _, fn := range m.AvailableResults {
			goapp.Log.Info().Str("ID", m.ID).Str("file", fn).Msg("get data")
			f, err := transcriber.GetResult(ctx, m.ExternalID, fn)
			if err != nil {
				return &errTranscriber{err: fmt.Errorf("can't get data: %w", err)}
			}
			err = data.Filer.SaveFile(ctx, fmt.Sprintf("%s/%s", m.ID, f.Name), bytes.NewReader(f.Content), int64(len(f.Content)))
			if err != nil {
				return fmt.Errorf("can't save file: %w", err)
			}
		}
		status.AvailableResults = m.AvailableResults
	}
	status.Error = utils.ToSQLStr(m.Error)
	status.ErrorCode = utils.ToSQLStr(m.ErrorCode)
	status.Progress = utils.ToSQLInt32(int32(m.Progress))
	status.Status = m.Status
	status.RecognizedText = utils.ToSQLStr(limit(m.RecognizedText, 200))
	if err := data.DB.UpdateStatus(ctx, status); err != nil {
		return fmt.Errorf("can't save status: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("Status update completed")
	err = data.MsgSender.SendMessage(ctx, &messages.ASRMessage{
		QueueMessage: amessages.QueueMessage{ID: m.ID}}, messages.DefaultOpts(messages.StatusChange))
	if err != nil {
		return fmt.Errorf("can't send msg: %w", err)
	}
	if isCompleted(m.Status, m.Error) {
		err = data.MsgSender.SendMessage(ctx, &messages.CleanMessage{
			QueueMessage: amessages.QueueMessage{ID: m.ID},
			ExternalID:   m.ExternalID, Transcriber: m.Transcriber},
			messages.DefaultOpts(wrkQueuePrefix+wrkStatusClean))
		if err != nil {
			return fmt.Errorf("can't send msg: %w", err)
		}
		err := data.MsgSender.SendMessage(ctx, &amessages.InformMessage{
			QueueMessage: *amessages.NewQueueMessageFromM(&m.QueueMessage),
			Type:         getInformFinishType(m.Error), At: time.Now()}, messages.DefaultOpts(messages.Inform))
		if err != nil {
			return fmt.Errorf("can't send msg: %w", err)
		}
		if m.Error != "" {
			err = data.MsgSender.SendMessage(ctx, &messages.ASRMessage{
				QueueMessage: *amessages.NewQueueMessageFromM(&m.QueueMessage)}, messages.DefaultOpts(wrkQueuePrefix+wrkRestoreUsage))
			if err != nil {
				return fmt.Errorf("can't send msg: %w", err)
			}
		}
	}
	return nil
}

func limit(s string, l int) string {
	if len(s) > l {
		r := []rune(s) // make sure we don't break utf-8
		if len(r) > l {
			return string(r[:l-3]) + "..."
		}
	}
	return s
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
	goapp.Log.Debug().Str("ID", m.ID).Msgf("loaded %v", statusRec)
	if statusRec.Error.String != "" {
		goapp.Log.Info().Str("ID", m.ID).Msg("error set - ignore")
	}
	statusRec.Error = utils.ToSQLStr(m.Error)
	statusRec.ErrorCode = utils.ToSQLStr(status.ServiceError.String())
	if err := data.DB.UpdateStatus(ctx, statusRec); err != nil {
		return fmt.Errorf("can't save status: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("Status update completed")
	goapp.Log.Info().Str("ID", m.ID).Msg("send status change")
	err = data.MsgSender.SendMessage(ctx, &messages.ASRMessage{
		QueueMessage: amessages.QueueMessage{ID: m.ID}}, messages.DefaultOpts(messages.StatusChange))
	if err != nil {
		return fmt.Errorf("can't send msg: %w", err)
	}
	err = data.MsgSender.SendMessage(ctx, &messages.ASRMessage{
		QueueMessage: amessages.QueueMessage{ID: m.ID}}, messages.DefaultOpts(wrkQueuePrefix+wrkRestoreUsage))
	if err != nil {
		return fmt.Errorf("can't send msg: %w", err)
	}
	return nil
}

func handleRestoreUsage(ctx context.Context, m *messages.ASRMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Msg("handling restore")
	req, err := data.DB.LoadRequest(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load request: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("loaded request")
	st, err := data.DB.LoadStatus(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't load status: %w", err)
	}
	goapp.Log.Info().Str("ID", m.ID).Msg("loaded status")
	if status.ECServiceError.String() == utils.FromSQLStr(st.ErrorCode) {
		if err := data.UsageRestorer.Do(ctx, req.ID, req.RequestID, utils.FromSQLStr(st.Error)); err != nil {
			return fmt.Errorf("can't restore: %w", err)
		}
		goapp.Log.Info().Str("ID", m.ID).Msg("restore done")
	} else {
		goapp.Log.Warn().Str("ID", m.ID).Str("errCode", utils.FromSQLStr(st.ErrorCode)).Msg("restore skip")
	}
	return nil
}

func handleClean(ctx context.Context, m *messages.CleanMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Str("extID", m.ExternalID).Msg("handling clean")
	transcriber, _, err := data.TranscriberPr.Get(m.Transcriber, false)
	if err != nil {
		return fmt.Errorf("can't get active transcriber by `%s`: %w", m.Transcriber, err)
	}

	if err := transcriber.Clean(ctx, m.ExternalID); err != nil {
		return fmt.Errorf("can't clean external data: %w", err)
	}
	return nil
}

func isCompleted(st, errStr string) bool {
	return status.From(st) == status.Completed || errStr != ""
}

func waitStatus(ctx context.Context, wd *persistence.WorkData, data *ServiceData) error {
	transcriber, _, err := data.TranscriberPr.Get(utils.FromSQLStr(wd.Transcriber), false)
	if err != nil {
		return fmt.Errorf("can't get transcriber %s: %w", utils.FromSQLStr(wd.Transcriber), err)
	}
	manualCheck := time.Second * 20
	stCh, cf, err := transcriber.HookToStatus(ctx, wd.ExternalID)
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
				finish, err := processStatus(ctx, &d, wd, data)
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
				d, err := transcriber.GetStatus(ctx, wd.ExternalID)
				if err != nil {
					return fmt.Errorf("can't get status: %w", err)
				}
				if isCompleted(d.Status, d.Error) { // send msg only if completed
					finish, err := processStatus(ctx, d, wd, data)
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
}

func processStatus(ctx context.Context, statusData *tapi.StatusData, wd *persistence.WorkData, data *ServiceData) (bool, error) {
	goapp.Log.Info().Str("status", statusData.Status).Str("ID", wd.ID).Msg("status")
	err := data.MsgSender.SendMessage(ctx, &messages.StatusMessage{
		QueueMessage:     amessages.QueueMessage{ID: wd.ID},
		Status:           statusData.Status,
		Error:            statusData.Error,
		Progress:         statusData.Progress,
		ErrorCode:        statusData.ErrorCode,
		AudioReady:       statusData.AudioReady,
		AvailableResults: statusData.AvResults,
		RecognizedText:   statusData.RecognizedText,
		ExternalID:       wd.ExternalID,
		Transcriber:      utils.FromSQLStr(wd.Transcriber),
	}, messages.DefaultOpts(wrkQueuePrefix+wrkStatusQueue))
	if err != nil {
		return false, fmt.Errorf("can't send msg: %w", err)
	}
	return isCompleted(statusData.Status, statusData.Error), nil
}

func upload(ctx context.Context, req *persistence.ReqData, transcriber tapi.Transcriber, data *ServiceData) (string, error) {
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
	extID, err := transcriber.Upload(ctx, &tapi.UploadData{Params: prepareParams(req.Params), Files: filesMap})
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
	if data.WorkerOtherCount < 1 {
		return fmt.Errorf("no worker other count provided")
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
	if data.TranscriberPr == nil {
		return fmt.Errorf("no TranscriberProvider")
	}
	if data.UsageRestorer == nil {
		return fmt.Errorf("no UsageRestorer")
	}
	return nil
}
