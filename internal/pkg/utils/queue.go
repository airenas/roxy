package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/vgarvardt/gue/v5"
)

// MsgSender provides send msg functionality
type MsgSender interface {
	SendMessage(context.Context, amessages.Message, string) error
}

// CreateHandler helper func to wrapp gue worker main func
func CreateHandler[TM any, SD any](data *SD, hf func(context.Context, *TM, *SD) error, failureSender MsgSender) gue.WorkFunc {
	return func(ctx context.Context, j *gue.Job) error {
		var m TM
		if err := json.Unmarshal(j.Args, &m); err != nil {
			return fmt.Errorf("could not unmarshal message: %w", err)
		}
		goapp.Log.Info().Str("queue", j.Queue).Str("type", j.Type).Int32("errCount", j.ErrorCount).Msg("got msg")
		if j.ErrorCount > 2 {
			goapp.Log.Error().Int32("time", j.ErrorCount).Str("lastError", j.LastError.String).Msg("msg failed, will not retry")
			if failureSender != nil {
				if err := sendFailure(ctx, failureSender, m); err != nil {
					goapp.Log.Error().Err(err).Str("queue", j.Queue).Str("type", j.Type).Msg("fail send failure msg")
				}
			} else {
				goapp.Log.Info().Str("queue", j.Queue).Str("type", j.Type).Msg("skip failure msg")
			}
			return nil
		}
		err := hf(ctx, &m, data)
		if err != nil {
			goapp.Log.Warn().Err(err).Str("queue", j.Queue).Str("type", j.Type).Msg("fail")
		}
		return err
	}
}

func sendFailure(ctx context.Context, sender MsgSender, m interface{}) error {
	am, ok := m.(messages.ASRMessage)
	if !ok {
		return fmt.Errorf("no ASRMessage")
	}
	goapp.Log.Info().Str("ID", am.ID).Msg("sending failure msg")
	return sender.SendMessage(ctx, amessages.InformMessage{
		QueueMessage: *amessages.NewQueueMessageFromM(&am.QueueMessage),
		Type:         amessages.InformTypeFailed, At: time.Now()}, messages.Inform)
}
