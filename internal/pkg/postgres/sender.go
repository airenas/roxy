package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vgarvardt/gue/v5"
	"github.com/vgarvardt/gue/v5/adapter/pgxv5"
)

//Sender performs messages sending using postgress gue
type Sender struct {
	gc *gue.Client
}

//NewSender initializes gue sender
func NewSender(pool *pgxpool.Pool) (*Sender, error) {
	gc, err := gue.NewClient(pgxv5.NewConnPool(pool))
	if err != nil {
		return nil, fmt.Errorf("can't init gue: %w", err)
	}
	return &Sender{gc: gc}, nil
}

//SendMessage sends the message with
func (sender *Sender) SendMessage(ctx context.Context, msg messages.Message, queue string) error {
	goapp.Log.Debug().Str("queue", queue).Msg("Sending message")
	args, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("can't marshal msg: %w", err)
	}

	j := &gue.Job{
		Type:  queue,
		Queue: queue,
		Args:  args,
	}
	if err := sender.gc.Enqueue(ctx, j); err != nil {
		return fmt.Errorf("can't send msg to %s: %w", queue, err)
	}
	goapp.Log.Debug().Msg("Sent")
	return nil
}
