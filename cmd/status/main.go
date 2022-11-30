package main

import (
	"context"
	"fmt"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/airenas/roxy/internal/pkg/statusservice"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/gommon/color"
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
		goapp.Log.Fatal().Err(fmt.Errorf("can't init db pool: %w", err))
	}

	dbPool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		goapp.Log.Fatal().Err(fmt.Errorf("can't init db pool: %w", err))
	}
	defer dbPool.Close()

	db, err := postgres.NewDB(dbPool)
	if err != nil {
		goapp.Log.Fatal().Err(fmt.Errorf("can't init db: %w", err))
	}

	data.DB = db

	err = statusservice.StartWebServer(data)
	if err != nil {
		goapp.Log.Fatal().Err(fmt.Errorf("can't start web server: %w", err))
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
