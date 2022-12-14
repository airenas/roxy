package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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

type Opts struct {
	failureSender MsgSender
	retryCount    int32
	backoff       gue.Backoff
	timeout       time.Duration
}

// CreateHandler helper func to wrapp gue worker main func
func Create[TM any, SD any](data *SD, hf func(context.Context, *TM, *SD) error, opts *Opts) gue.WorkFunc {
	if opts == nil {
		goapp.Log.Panic().Msg("no opts provided")
	}
	return func(ctx context.Context, j *gue.Job) error {
		goapp.Log.Info().Str("queue", j.Queue).Str("type", j.Type).Int32("errCount", j.ErrorCount).Msg("got msg")

		var m TM
		err := json.Unmarshal(j.Args, &m)
		if err != nil {
			err = fmt.Errorf("could not unmarshal message: %w", err)
		} else {
			wrkCtx, cf := context.WithTimeout(ctx, opts.timeout)
			defer cf()
			err = hf(wrkCtx, &m, data)
			if err != nil {
				goapp.Log.Warn().Err(err).Str("queue", j.Queue).Str("type", j.Type).Msg("fail")
			}
		}
		if err == nil {
			return nil
		}

		// process error
		if j.ErrorCount > opts.retryCount {
			if opts.failureSender != nil {
				if _err := sendStatusChangeFailure(ctx, opts.failureSender, m, err.Error()); _err != nil {
					goapp.Log.Error().Err(_err).Str("queue", j.Queue).Str("type", j.Type).Msg("fail send status failure msg")
				}
				if _err := sendFailure(ctx, opts.failureSender, m); _err != nil {
					goapp.Log.Error().Err(_err).Str("queue", j.Queue).Str("type", j.Type).Msg("fail send failure msg")
				}
			} else {
				goapp.Log.Info().Str("queue", j.Queue).Str("type", j.Type).Msg("skip failure msg")
			}
			return nil
		}
		delay := opts.backoff(int(j.ErrorCount + 1))
		goapp.Log.Info().Str("queue", j.Queue).Str("type", j.Type).Dur("after", delay).Msg("retry after")
		return gue.ErrRescheduleJobIn(delay, err.Error())
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

func sendStatusChangeFailure(ctx context.Context, sender MsgSender, m interface{}, errStr string) error {
	am, ok := m.(messages.ASRMessage)
	if !ok {
		return fmt.Errorf("no ASRMessage")
	}
	goapp.Log.Info().Str("ID", am.ID).Msg("sending failure status change msg")
	return sender.SendMessage(ctx, messages.ASRMessage{
		QueueMessage: amessages.QueueMessage{ID: am.ID, Error: errStr}}, messages.Fail)
}

func DefaultOpts() *Opts {
	return &Opts{retryCount: 3, backoff: DefaultBackoff(), timeout: time.Minute * 15}
}

func DefaultBackoff() gue.Backoff {
	return func(retries int) time.Duration {
		return fullJitter(time.Duration(retries) * time.Second * 10)
	}
}

func NoBackoff() gue.Backoff {
	return func(retries int) time.Duration {
		return 0
	}
}

func DefaultBackoffOrTest(test bool) gue.Backoff {
	if test {
		return NoBackoff()
	}
	return DefaultBackoff()
}

func (o *Opts) WithFailure(failureSender MsgSender) *Opts {
	o.failureSender = failureSender
	return o
}

func (o *Opts) WithTimeout(timeout time.Duration) *Opts {
	o.timeout = timeout
	return o
}

func (o *Opts) WithBackoff(b gue.Backoff) *Opts {
	o.backoff = b
	return o
}

// fullJitter return randomized duration in interval [0, t)
// as suggested by https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
func fullJitter(t time.Duration) time.Duration {
	// `rand` here is used just for backoff jitter,
	// it is not recommended to use rand in favor of crypto/rand, but here `rand` is ok
	rand.Seed(time.Now().UnixMicro())
	return time.Duration(float64(t) * rand.Float64())
}
