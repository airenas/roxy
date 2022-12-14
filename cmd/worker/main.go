package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/airenas/async-api/pkg/miniofs"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/airenas/roxy/internal/pkg/transcriber"
	"github.com/airenas/roxy/internal/pkg/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/gommon/color"
	"github.com/vgarvardt/gue/v5"
	"github.com/vgarvardt/gue/v5/adapter/pgxv5"
)

func main() {
	goapp.StartWithDefault()
	cfg := goapp.Config

	data := &worker.ServiceData{}
	ctx := context.Background()

	dbConfig, err := pgxpool.ParseConfig(cfg.GetString("db.url"))
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db pool")
	}

	goapp.Log.Info().Int32("max_conn", dbConfig.MaxConns).Int32("min_conn", dbConfig.MinConns).Msg("db info")

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
	data.Testing = cfg.GetBool("worker.testing")
	data.MsgSender, err = postgres.NewSender(dbPool)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init gue sender")
	}
	data.Filer, err = miniofs.NewFiler(ctx, miniofs.Options{Bucket: cfg.GetString("filer.bucket"),
		URL: cfg.GetString("filer.url"), User: cfg.GetString("filer.user"), Key: cfg.GetString("filer.key")})
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init filer")
	}
	db, err := postgres.NewDB(dbPool)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db")
	}

	data.DB = db

	data.Transcriber, err = transcriber.NewClient(cfg.GetString("transcriber.uploadUrl"),
		cfg.GetString("transcriber.statusUrl"), cfg.GetString("transcriber.resultUrl"), cfg.GetString("transcriber.cleanUrl"))
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init transcriber")
	}

	printBanner()

	ctx, cancelFunc := context.WithCancel(context.Background())
	// data.StopCtx = ctx
	doneCh, err := worker.StartWorkerService(ctx, data)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't start worker service")
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
 /_/ |_|\____/_/|_|  /_/   v: %s
						   
                      __            
 _      ______  _____/ /_____  _____
| | /| / / __ \/ ___/ //_/ _ \/ ___/
| |/ |/ / /_/ / /  / ,< /  __/ /    
|__/|__/\____/_/  /_/|_|\___/_/     
							  
%s
________________________________________________________

`
	cl := color.New()
	cl.Printf(banner, cl.Red(version), cl.Green("https://github.com/airenas/roxy"))
}
