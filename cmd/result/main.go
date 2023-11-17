package main

import (
	"context"

	"github.com/airenas/async-api/pkg/miniofs"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/airenas/roxy/internal/pkg/result"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/gommon/color"
)

func main() {
	goapp.StartWithDefault()

	printBanner()

	cfg := goapp.Config
	data := &result.Data{}
	data.Port = cfg.GetInt("port")
	var err error

	ctx := context.Background()

	dbConfig, err := pgxpool.ParseConfig(cfg.GetString("db.url"))
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db pool")
	}
	addDBLog(dbConfig)

	dbPool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db pool")
	}
	defer dbPool.Close()

	db, err := postgres.NewDB(dbPool)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init db")
	}

	data.NameProvider = db

	data.Reader, err = miniofs.NewFiler(ctx, miniofs.Options{Bucket: cfg.GetString("filer.bucket"),
		URL: cfg.GetString("filer.url"), User: cfg.GetString("filer.user"), Key: cfg.GetString("filer.key"),
		Secure: cfg.GetBool("filer.https")})
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't init file reader")
	}

	err = result.StartWebServer(data)
	if err != nil {
		goapp.Log.Fatal().Err(err).Msg("can't start web server")
	}
}

func addDBLog(dbConfig *pgxpool.Config) {
	logFunc := goapp.Log.Info().Msg
	dbConfig.BeforeConnect = func(ctx context.Context, cc *pgx.ConnConfig) error {
		logFunc("before connect")
		return nil
	}
	dbConfig.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		logFunc("after connect")
		return nil
	}
	dbConfig.BeforeAcquire = func(ctx context.Context, c *pgx.Conn) bool {
		logFunc("before acquire")
		return true
	}
	dbConfig.AfterRelease = func(c *pgx.Conn) bool {
		logFunc("after release")
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
						   
                         ____     ______ 
   ________  _______  __/ / /_   / ____ \
  / ___/ _ \/ ___/ / / / / __/  / / __ ` + "`" + `/
 / /  /  __(__  ) /_/ / / /_   / / /_/ / 
/_/   \___/____/\__,_/_/\__/   \ \__,_/  v: %s
                                \____/ 	

%s
________________________________________________________                                                 

`
	cl := color.New()
	cl.Printf(banner, cl.Red(version), cl.Green("https://github.com/airenas/roxy"))
}
