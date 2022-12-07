package inform

import (
	"context"
	"fmt"
	"time"

	"github.com/airenas/async-api/pkg/inform"
	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/utils"
	"github.com/jordan-wright/email"
	"github.com/vgarvardt/gue/v5"
)

// Sender send emails
type Sender interface {
	Send(email *email.Email) error
}

// EmailMaker prepares the email
type EmailMaker interface {
	Make(data *inform.Data) (*email.Email, error)
}

// EmailRetriever return the email by ID
type EmailRetriever interface {
	GetEmail(ID string) (string, error)
}

// DB tracks email sending process
// It is used to quarantee not to send the emails twice
type DB interface {
	LockEmailTable(context.Context, string, string) error
	UnLockEmailTable(context.Context, string, string, *int) error
	LoadRequest(ctx context.Context, id string) (*persistence.ReqData, error)
}

// MsgSender provides send msg functionality
type MsgSender interface {
	SendMessage(context.Context, amessages.Message, string) error
}

// ServiceData keeps data required for service work
type ServiceData struct {
	GueClient   *gue.Client
	WorkerCount int
	EmailSender Sender
	EmailMaker  EmailMaker
	DB          DB
	Location    *time.Location
}

// StartWorkerService starts the event queue listener service to listen for inform events
// returns channel for tracking when all jobs are finished
func StartWorkerService(ctx context.Context, data *ServiceData) (chan struct{}, error) {
	if err := validate(data); err != nil {
		return nil, err
	}
	goapp.Log.Info().Msg("Starting listen for messages")

	wm := gue.WorkMap{
		messages.Inform: utils.CreateHandler(data, handleInform),
	}

	pool, err := gue.NewWorkerPool(
		data.GueClient, wm, data.WorkerCount,
		gue.WithPoolQueue(messages.Upload),
		gue.WithPoolLogger(utils.NewGueLoggerAdapter()),
		gue.WithPoolPollInterval(500*time.Millisecond),
		gue.WithPoolPollStrategy(gue.RunAtPollStrategy),
		gue.WithPoolID("asr-inform"),
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

func handleInform(ctx context.Context, m *amessages.InformMessage, data *ServiceData) error {
	goapp.Log.Info().Str("ID", m.ID).Msg("handling")

	mailData := inform.Data{}
	mailData.ID = m.ID
	mailData.MsgTime = toLocalTime(data, m.At)
	mailData.MsgType = m.Type

	var err error
	req, err := data.DB.LoadRequest(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("can't retrieve email: %w", err)
	}
	if req.Email == "" {
		goapp.Log.Info().Msg("No email, skip")
		return nil
	}

	mailData.Email = req.Email

	email, err := data.EmailMaker.Make(&mailData)
	if err != nil {
		return fmt.Errorf("can't prepare email: %w", err)
	}

	err = data.DB.LockEmailTable(ctx, mailData.ID, mailData.MsgType)
	if err != nil {
		return fmt.Errorf("can't lock mail table: %w", err)
	}
	var unlockValue = 0
	defer data.DB.UnLockEmailTable(ctx, mailData.ID, mailData.MsgType, &unlockValue)

	err = data.EmailSender.Send(email)
	if err != nil {
		return fmt.Errorf("can't send email: %w", err)
	}
	unlockValue = 2
	return nil
}

func validate(data *ServiceData) error {
	if data.GueClient == nil {
		return fmt.Errorf("no gue client")
	}
	if data.WorkerCount < 1 {
		return fmt.Errorf("no worker count provided")
	}
	if data.EmailMaker == nil {
		return fmt.Errorf("no EmailMaker")
	}
	if data.EmailSender == nil {
		return fmt.Errorf("no EmailSender")
	}
	if data.DB == nil {
		return fmt.Errorf("no DB")
	}
	return nil
}

func toLocalTime(data *ServiceData, t time.Time) time.Time {
	if data.Location != nil {
		return t.In(data.Location)
	}
	return t
}
