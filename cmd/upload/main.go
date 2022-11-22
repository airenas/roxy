package main

import (
	"context"
	"fmt"

	"github.com/airenas/async-api/pkg/miniofs"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/airenas/roxy/internal/pkg/upload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/gommon/color"
)

func main() {
	goapp.StartWithDefault()

	printBanner()

	cfg := goapp.Config
	data := &upload.Data{}
	data.Port = cfg.GetInt("port")
	var err error

	ctx := context.Background()

	dbConfig, err := pgxpool.ParseConfig(cfg.GetString("db.url"))
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init db pool: %w", err))
	}
	add_db_log(dbConfig)

	dbPool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init db pool: %w", err))
	}
	defer dbPool.Close()

	db, err := postgres.NewDB(dbPool)
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init db: %w", err))
	}

	data.DBSaver = db

	data.Saver, err = miniofs.NewFiler(ctx, miniofs.Options{Bucket: cfg.GetString("filer.bucket"),
		URL: cfg.GetString("filer.url"), User: cfg.GetString("filer.user"), Key: cfg.GetString("filer.key")})
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init file saver: %w", err))
	}

	data.MsgSender, err = postgres.NewSender(dbPool)
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init gue sender: %w", err))
	}

	err = upload.StartWebServer(data)
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't start web server: %w", err))
	}
}

func add_db_log(dbConfig *pgxpool.Config) {
	log_f := goapp.Log.Info
	dbConfig.BeforeConnect = func(ctx context.Context, cc *pgx.ConnConfig) error {
		log_f("before connect")
		return nil
	}
	dbConfig.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		log_f("after connect")
		return nil
	}
	dbConfig.BeforeAcquire = func(ctx context.Context, c *pgx.Conn) bool {
		log_f("before acquire")
		return true
	}
	dbConfig.AfterRelease = func(c *pgx.Conn) bool {
		log_f("after release")
		return true
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

                 __                __
    __  ______  / /___  ____ _____/ /
   / / / / __ \/ / __ \/ __ ` + "`" + `/ __  / 
  / /_/ / /_/ / / /_/ / /_/ / /_/ /  
  \__,_/ .___/_/\____/\__,_/\__,_/   v: %s
      /_/                           
	
%s
________________________________________________________                                                 

`
	cl := color.New()
	cl.Printf(banner, cl.Red(version), cl.Green("https://github.com/airenas/roxy"))
}
