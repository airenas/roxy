package main

import (
	"context"
	"time"

	aclean "github.com/airenas/async-api/pkg/clean"
	"github.com/airenas/async-api/pkg/miniofs"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/clean"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/gommon/color"
)

func main() {
	goapp.StartWithDefault()
	cfg := goapp.Config

	data := &clean.Data{}
	data.Port = cfg.GetInt("port")

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

	dbCleaner, err := postgres.NewCleaner(dbPool)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db")
	}

	fsCleaner, err := miniofs.NewFiler(ctx, miniofs.Options{Bucket: cfg.GetString("filer.bucket"),
		URL: cfg.GetString("filer.url"), User: cfg.GetString("filer.user"), Key: cfg.GetString("filer.key"),
		Secure: cfg.GetBool("filer.https")})
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init file cleaner")
	}

	tData := aclean.TimerData{}
	tData.IDsProvider, err = postgres.NewDBIdsProvider(dbPool, cfg.GetDuration("timer.expire"))
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init IDs provider")
	}

	printBanner()

	cleaner := &aclean.CleanerGroup{}
	cleaner.Jobs = append(cleaner.Jobs, fsCleaner)
	cleaner.Jobs = append(cleaner.Jobs, dbCleaner)

	data.Cleaner = cleaner

	tData.RunEvery = cfg.GetDuration("timer.runEvery")
	tData.Cleaner = cleaner

	goapp.Log.Info().Dur("duration", cfg.GetDuration("timer.expire")).Msg("expire")

	ctxTimer, cancelFunc := context.WithCancel(ctx)
	doneCh, err := aclean.StartCleanTimer(ctxTimer, &tData)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't start timer")
	}
	err = clean.StartWebServer(data)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't start web server")
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
	banner :=
		`
    ____  ____ _  ____  __
   / __ \/ __ \ |/ /\ \/ /
  / /_/ / / / /   /  \  / 
 / _, _/ /_/ /   |   / /  
/_/ |_|\____/_/|_|  /_/   
					   
        __                                   
  _____/ /__  ____ _____           _  __     
 / ___/ / _ \/ __ ` + "`" + `/ __ \   ______| |/_/_____
/ /__/ /  __/ /_/ / / / /  /_____/>  </_____/
\___/_/\___/\__,_/_/ /_/        /_/|_|   v: %s
	
%s
________________________________________________________

`
	cl := color.New()
	cl.Printf(banner, cl.Red(version), cl.Green("https://github.com/airenas/roxy"))
}
