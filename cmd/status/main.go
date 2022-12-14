package main

import (
	"context"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/airenas/roxy/internal/pkg/statusservice"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/gommon/color"
	"github.com/vgarvardt/gue/v5"
	"github.com/vgarvardt/gue/v5/adapter/pgxv5"
)

func main() {
	goapp.StartWithDefault()

	printBanner()

	cfg := goapp.Config
	data := &statusservice.Data{}
	data.Port = cfg.GetInt("port")
	var err error

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

	db, err := postgres.NewDB(dbPool)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db")
	}

	data.DB = db
	wsh := statusservice.NewWSConnKeeper()
	data.WSHandler = wsh

	hData := &statusservice.HandlerData{}
	hData.DB = db
	hData.WorkerCount = cfg.GetInt("worker.count")
	hData.WSHandler = wsh
	hData.GueClient, err = gue.NewClient(pgxv5.NewConnPool(dbPool))
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init gue")
	}

	goapp.Log.Info().Msg("starting handler")
	ctx, cancelFunc := context.WithCancel(context.Background())
	doneCh, err := statusservice.StartStatusHandler(ctx, hData)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't start status handler service")
	}

	goapp.Log.Info().Msg("starting web service")
	if err := statusservice.StartWebServer(data); err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't start web server")
	}
	goapp.Log.Info().Msg("exit web service")
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
						   
         __        __            
   _____/ /_____ _/ /___  _______
  / ___/ __/ __ ` + "`" + `/ __/ / / / ___/
 (__  ) /_/ /_/ / /_/ /_/ (__  ) 
/____/\__/\__,_/\__/\__,_/____/   v: %s

%s
________________________________________________________                                                 

`
	cl := color.New()
	cl.Printf(banner, cl.Red(version), cl.Green("https://github.com/airenas/roxy"))
}
