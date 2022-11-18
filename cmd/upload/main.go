package main

import (
	"context"
	"fmt"

	"github.com/airenas/async-api/pkg/miniofs"
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/postgres"
	"github.com/airenas/roxy/internal/pkg/upload"
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

	dbPool, err := pgxpool.New(ctx, cfg.GetString("db.url"))
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init db pool: %w", err))
	}
	defer dbPool.Close()

	db, err := postgres.NewDB(dbPool)
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init db: %w", err))
	}

	data.ReqSaver = db

	data.Saver, err = miniofs.NewFiler(ctx, miniofs.Options{Bucket: cfg.GetString("filer.bucket"),
		URL: cfg.GetString("filer.url"), User: cfg.GetString("filer.user"), Key: cfg.GetString("filer.key")})
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't init file saver: %w", err))
	}

	// mongoSessionProvider, err := mng.NewSessionProvider(cfg.GetString("mongo.url"), mongo.GetIndexes(), "tts")
	// if err != nil {
	// 	goapp.Log.Fatal(errors.Wrap(err, "can't init mongo session provider"))
	// }
	// defer mongoSessionProvider.Close()

	// msgChannelProvider, err := rabbit.NewChannelProvider(cfg.GetString("messageServer.url"),
	// 	cfg.GetString("messageServer.user"), cfg.GetString("messageServer.pass"))
	// if err != nil {
	// 	goapp.Log.Fatal(errors.Wrap(err, "can't init rabbitmq channel provider"))
	// }
	// defer msgChannelProvider.Close()
	// err = initQueues(msgChannelProvider)
	// if err != nil {
	// 	goapp.Log.Fatal(errors.Wrap(err, "can't init queues"))
	// }

	// data.MsgSender = rabbit.NewSender(msgChannelProvider)

	err = upload.StartWebServer(data)
	if err != nil {
		goapp.Log.Fatal(fmt.Errorf("can't start web server: %w", err))
	}
}

// func initQueues(prv *rabbit.ChannelProvider) error {
// 	goapp.Log.Info("Initializing queues")
// 	return prv.RunOnChannelWithRetry(func(ch *amqp.Channel) error {
// 		_, err := rabbit.DeclareQueue(ch, prv.QueueName(messages.Upload))
// 		return err
// 	})
// }

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
