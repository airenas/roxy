package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	ainform "github.com/airenas/async-api/pkg/inform"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/inform"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/gommon/color"
	"github.com/vgarvardt/gue/v5"
	"github.com/vgarvardt/gue/v5/adapter/pgxv5"
)

func main() {
	goapp.StartWithDefault()
	cfg := goapp.Config

	data := &inform.ServiceData{}
	ctx := context.Background()

	dbConfig, err := pgxpool.ParseConfig(cfg.GetString("db.url"))
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db pool")
	}

	dbPool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db pool")
	}
	defer dbPool.Close()

	data.GueClient, err = gue.NewClient(pgxv5.NewConnPool(dbPool))
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init gue")
	}
	data.WorkerCount = cfg.GetInt("worker.count")

	data.EmailMaker, err = ainform.NewTemplateEmailMaker(cfg)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init email maker")
	}

	location := cfg.GetString("worker.location")
	if location != "" {
		data.Location, err = time.LoadLocation(location)
		if err != nil {
			goapp.Log.Fatal().Err(err).Msg("can't init location")
		}
		goapp.Log.Info().Str("local", time.Now().In(data.Location).Format(time.RFC3339)).Msg("time")
	}

	if cfg.GetString("smtp.fakeUrl") == "" {
		goapp.Log.Info().Str("sender", "real").Msg("smtp")
		data.EmailSender, err = ainform.NewSimpleEmailSender(cfg)
		if err != nil {
			goapp.Log.Fatal().Err(err).Msg("can't init email sender")

		}
	} else {
		goapp.Log.Info().Str("sender", "fake").Msg("smtp")
		data.EmailSender, err = inform.NewFakeEmailSender(cfg)
		if err != nil {
			goapp.Log.Fatal().Err(err).Msg("can't init fake email sender")
		}
	}

	db, err := postgres.NewDB(dbPool)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db")
	}

	data.DB = db

	printBanner()

	ctx, cancelFunc := context.WithCancel(context.Background())
	// data.StopCtx = ctx
	doneCh, err := inform.StartWorkerService(ctx, data)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't start inform service")
	}
	/////////////////////// Waiting for terminate
	waitCh := make(chan os.Signal, 2)
	signal.Notify(waitCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-waitCh:
		goapp.Log.Info().Msg("Got exit signal")
	case <-doneCh:
		goapp.Log.Info().Msg("Service exit")
	}
	cancelFunc()
	select {
	case <-doneCh:
		goapp.Log.Info().Msg("All code returned. Now exit. Bye")
	case <-time.After(time.Second * 15):
		goapp.Log.Warn().Msg("Timeout gracefull shutdown")
	}
}

var (
	version = "DEV"
)

func printBanner() {
	banner := `
     ____  ____ _  ____  __
    / __ \/ __ \ |/ /\ \/ /
   / /_/ / / / /   /  \  / 
  / _, _/ /_/ /   |   / /  
 /_/ |_|\____/_/|_|  /_/   
						   
     _       ____                                           
    (_)___  / __/           ____              _________ ___ 
   / / __ \/ /_____________/ __ \____________/ ___/ __ ` + "`" + ` __ \
  / / / / / __/_____/_____/ /_/ /_____/_____/ /  / / / / / /
 /_/_/ /_/_/              \____/           /_/  /_/ /_/ /_/ v: %s

%s
________________________________________________________

`
	cl := color.New()
	cl.Printf(banner, cl.Red(version), cl.Green("https://github.com/airenas/roxy"))
}
