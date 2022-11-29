package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/vgarvardt/gue/v5"
	// "github.com/airenas/big-tts/internal/pkg/messages"
	// "github.com/airenas/big-tts/internal/pkg/status"
	// "github.com/airenas/big-tts/internal/pkg/utils"
	// "github.com/airenas/go-app/pkg/goapp"
	// "github.com/pkg/errors"
	// "github.com/streadway/amqp"
)

//MsgSender provides send msg functionality
type MsgSender interface {
	SendMessage(context.Context, amessages.Message, string) error
}

//DB provides persistnce functionality
type DB interface {
	LoadRequest(ctx context.Context, id string) (*persistence.ReqData, error)
	LoadWorkData(ctx context.Context, id string) (*persistence.WorkData, error)
	// LoadWorkData(ctx context.Context, id string) (*persistence.WorkData, error)
	SaveWorkData(ctx context.Context, wrkData *persistence.WorkData) error
}

//Filer retrieves files
type Filer interface {
	LoadFile(ctx context.Context, fileName string) (io.ReadCloser, error)
}

//StatusSaver persists data to DB
type StatusSaver interface {
	Save(ID string, status, err string) error
}

//Transcriber provides transcription
type Transcriber interface {
	Upload(ctx context.Context, audio *tapi.UploadData) (string, error)
	HookToStatus(ctx context.Context, ID string) (<-chan tapi.StatusData, func(), error)
}

// ServiceData keeps data required for service work
type ServiceData struct {
	GueClient   *gue.Client
	WorkerCount int
	Queue       string
	MsgSender   MsgSender
	DB          DB
	Filer       Filer
	Transcriber Transcriber
}

//StartWorkerService starts the event queue listener service to listen for events
//returns channel for tracking if all jobs are finished
func StartWorkerService(ctx context.Context, data *ServiceData) (chan struct{}, error) {
	if err := validate(data); err != nil {
		return nil, err
	}
	goapp.Log.Info().Str("queue", data.Queue).Msg("Starting listen for messages")

	wm := gue.WorkMap{
		data.Queue: workHandler(data),
	}

	pool, err := gue.NewWorkerPool(
		data.GueClient, wm, data.WorkerCount,
		gue.WithPoolQueue(data.Queue),
		gue.WithPoolLogger(newGueLoggerAdapter()),
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

func workHandler(data *ServiceData) gue.WorkFunc {
	return func(ctx context.Context, j *gue.Job) error {
		var m messages.ASRMessage
		if err := json.Unmarshal(j.Args, &m); err != nil {
			return fmt.Errorf("could not unmarshal message: %w", err)
		}
		goapp.Log.Info().Str("id", m.ID).Int32("errCount", j.ErrorCount).Msg("got msg")
		if j.ErrorCount > 2 {
			goapp.Log.Error().Int32("time", j.ErrorCount).Str("lastError", j.LastError.String).Msg("msg failed, will not retry")
			return nil
		}
		return handleASR(ctx, &m, data)
	}
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
	_ = req
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
		extId, err := upload(ctx, req, data)
		if err != nil {
			return fmt.Errorf("can't upload: %w", err)
		}
		wd = &persistence.WorkData{ID: req.ID, ExternalID: extId, Created: time.Now()}
		err = data.DB.SaveWorkData(ctx, wd)
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
	// download audio
	// download results
	return nil
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
				finish, err := processStatus(&d, ID, data)
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

func processStatus(statusData *tapi.StatusData, ID string, data *ServiceData) (bool, error) {
	goapp.Log.Info().Str("status", statusData.Status).Str("ID", ID).Msg("status")
	if statusData.Error != "" {
		return statusData.Status == "COMPLETED", fmt.Errorf("err: %s, %s", statusData.ErrorCode, statusData.Error)
	}
	return statusData.Status == "COMPLETED", nil
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
	if data.Queue == "" {
		return fmt.Errorf("no queue")
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

// func listenQueue(ctx context.Context, q <-chan amqp.Delivery, f prFunc, data *ServiceData, cancelF func()) {
// 	defer cancelF()
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			goapp.Log.Infof("Exit queue func")
// 			return
// 		case d, ok := <-q:
// 			{
// 				if !ok {
// 					goapp.Log.Infof("Stopped listening queue")
// 					return
// 				}
// 				err := processMsg(&d, f, data)
// 				if err != nil {
// 					goapp.Log.Error(err)
// 				}
// 			}
// 		}
// 	}
// }

// func processMsg(d *amqp.Delivery, f prFunc, data *ServiceData) error {
// 	goapp.Log.Infof("Got msg: %s", d.RoutingKey)
// 	var message messages.TTSMessage
// 	if err := json.Unmarshal(d.Body, &message); err != nil {
// 		d.Nack(false, false)
// 		return errors.Wrap(err, "can't unmarshal message "+string(d.Body))
// 	}
// 	redeliver, err := f(&message, data)
// 	if err != nil {
// 		goapp.Log.Errorf("Can't process message %s\n%s", d.MessageId, string(d.Body))
// 		goapp.Log.Error(err)
// 		select {
// 		case <-data.StopCtx.Done():
// 			goapp.Log.Infof("Cancel msg process")
// 			return nil
// 		default:
// 		}
// 		requeue := redeliver && !d.Redelivered
// 		if !requeue {
// 			errInt := data.StatusSaver.Save(message.ID, "", err.Error())
// 			if errInt != nil {
// 				goapp.Log.Error(errInt)
// 			}
// 			errInt = data.InformMsgSender.Send(newInformMessage(&message, amessages.InformTypeFailed), messages.Inform, "")
// 			if errInt != nil {
// 				goapp.Log.Error(errInt)
// 			}
// 			if needToRestoreUsage(err) && d.RoutingKey != messages.Fail && message.Error == "" {
// 				failMsg := messages.NewMessageFrom(&message)
// 				failMsg.Error = err.Error()
// 				err = data.MsgSender.Send(failMsg, messages.Fail, "")
// 				if err != nil {
// 					goapp.Log.Error(err)
// 				}
// 			} else {
// 				goapp.Log.Info("NonRestorableError - do not send msg for restoring usage")
// 			}
// 		}
// 		return d.Nack(false, requeue) // redeliver for first time
// 	}
// 	return d.Ack(false)
// }

// func needToRestoreUsage(err error) bool {
// 	var errTest *utils.ErrNonRestorableUsage
// 	return !errors.As(err, &errTest)
// }

//synthesize starts the synthesize process
// workflow:
// 1. set status to WORKING
// 2. send inform msg
// 3. Send split msg
// func listenUpload(message *messages.TTSMessage, data *ServiceData) (bool, error) {
// 	goapp.Log.Infof("Got %s msg :%s", messages.Upload, message.ID)
// 	err := data.StatusSaver.Save(message.ID, status.Uploaded.String(), "")
// 	if err != nil {
// 		return true, err
// 	}
// 	err = data.InformMsgSender.Send(newInformMessage(message, amessages.InformTypeStarted), messages.Inform, "")
// 	if err != nil {
// 		return true, err
// 	}
// 	return true, data.MsgSender.Send(messages.NewMessageFrom(message), messages.Split, "")
// }

// func split(message *messages.TTSMessage, data *ServiceData) (bool, error) {
// 	goapp.Log.Infof("Got %s msg :%s", messages.Split, message.ID)
// 	err := data.StatusSaver.Save(message.ID, status.Split.String(), "")
// 	if err != nil {
// 		return true, err
// 	}
// 	resMsg := messages.NewMessageFrom(message)
// 	err = data.Splitter.Do(data.StopCtx, message)
// 	if err != nil {
// 		return true, err
// 	}
// 	return true, data.MsgSender.Send(resMsg, messages.Synthesize, "")
// }

// func synthesize(message *messages.TTSMessage, data *ServiceData) (bool, error) {
// 	goapp.Log.Infof("Got %s msg :%s", messages.Synthesize, message.ID)
// 	err := data.StatusSaver.Save(message.ID, status.Synthesize.String(), "")
// 	if err != nil {
// 		return true, err
// 	}
// 	resMsg := messages.NewMessageFrom(message)
// 	err = data.Synthesizer.Do(data.StopCtx, message)
// 	if err != nil {
// 		return true, err
// 	}
// 	return true, data.MsgSender.Send(resMsg, messages.Join, "")
// }

// func join(message *messages.TTSMessage, data *ServiceData) (bool, error) {
// 	goapp.Log.Infof("Got %s msg :%s", messages.Join, message.ID)
// 	err := data.StatusSaver.Save(message.ID, status.Join.String(), "")
// 	if err != nil {
// 		return true, err
// 	}
// 	err = data.Joiner.Do(data.StopCtx, message)
// 	if err != nil {
// 		return true, err
// 	}
// 	err = data.StatusSaver.Save(message.ID, status.Completed.String(), "")
// 	if err != nil {
// 		return true, err
// 	}
// 	return true, data.InformMsgSender.Send(newInformMessage(message, amessages.InformTypeFinished), messages.Inform, "")
// }

// func restoreUsage(message *messages.TTSMessage, data *ServiceData) (bool, error) {
// 	goapp.Log.Infof("Got %s msg :%s", messages.Fail, message.ID)
// 	return true, data.UsageRestorer.Do(data.StopCtx, message)
// }

// func newInformMessage(msg *messages.TTSMessage, it string) *amessages.InformMessage {
// 	return &amessages.InformMessage{QueueMessage: amessages.QueueMessage{ID: msg.ID, Tags: msg.Tags},
// 		Type: it, At: time.Now().UTC()}
// }
