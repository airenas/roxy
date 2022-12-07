package statusservice

import (
	"context"
	"fmt"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/utils"
	"github.com/vgarvardt/gue/v5"
)

// StatusDB provides persistance functionality
type StatusDB interface {
	LoadStatus(ctx context.Context, id string) (*persistence.Status, error)
}

// HandlerData keeps data required for handler
type HandlerData struct {
	GueClient   *gue.Client
	WorkerCount int
	DB          StatusDB
	WSHandler   WSConnHandler
}

// StartStatusHandler starts the event queue listener for status events
// returns channel for tracking if all jobs are finished
func StartStatusHandler(ctx context.Context, data *HandlerData) (chan struct{}, error) {
	if err := validateHandler(data); err != nil {
		return nil, err
	}
	goapp.Log.Info().Msg("Starting listen for messages")

	wm := gue.WorkMap{
		messages.StatusChange: utils.CreateHandler(data, handleStatus, nil),
	}

	pool, err := gue.NewWorkerPool(
		data.GueClient, wm, data.WorkerCount,
		gue.WithPoolQueue(messages.StatusChange),
		gue.WithPoolLogger(utils.NewGueLoggerAdapter()),
		gue.WithPoolPollInterval(500*time.Millisecond),
		gue.WithPoolPollStrategy(gue.RunAtPollStrategy),
		gue.WithPoolID("status-worker"),
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

func handleStatus(ctx context.Context, m *messages.ASRMessage, data *HandlerData) error {
	goapp.Log.Info().Str("ID", m.ID).Msg("handling status change event")

	conns, found := data.WSHandler.GetConnections(m.ID)
	if found {
		st, err := data.DB.LoadStatus(ctx, m.ID)
		if err != nil {
			return fmt.Errorf("cannot get status for ID %s: %w", m.ID, err)
		}
		if st == nil {
			return fmt.Errorf("no status for ID %s", m.ID)
		}
		res := &result{ID: m.ID, ErrorCode: st.ErrorCode.String, Error: st.Error.String,
			Status: st.Status, Progress: st.Progress.Int32,
			AudioReady: st.AudioReady, AvailableResults: st.AvailableResults}
		for _, c := range conns {
			if err := sendMsg(c, res); err != nil {
				goapp.Log.Error().Err(err).Send()
			}
		}
	} else {
		goapp.Log.Debug().Str("ID", m.ID).Msg("no connections found")
	}
	return nil
}

func sendMsg(c WsConn, res *result) error {
	goapp.Log.Debug().Str("ID", res.ID).Msg("Sending result to websockket")
	err := c.WriteJSON(res)
	if err != nil {
		return fmt.Errorf("cannot write to websockket: %w", err)
	}
	goapp.Log.Debug().Str("ID", res.ID).Msg("sent msg to websockket")
	return nil
}

func validateHandler(data *HandlerData) error {
	if data.GueClient == nil {
		return fmt.Errorf("no gue client")
	}
	if data.WorkerCount < 1 {
		return fmt.Errorf("no worker count provided")
	}
	if data.DB == nil {
		return fmt.Errorf("no DB")
	}
	if data.WSHandler == nil {
		return fmt.Errorf("no WSHandler")
	}
	return nil
}
