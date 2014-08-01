package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/bitly/go-nsq"
	"github.com/codegangsta/cli"
	"github.com/davecgh/go-spew/spew"
	"github.com/xlab/smscloud/misc"
)

const (
	subTopic   = "messages"
	subChannel = "sc_server"
)

var app = cli.NewApp()
var nsqLog = log.New(os.Stderr, "sc-server: ", log.LstdFlags)

func init() {
	app.Name = "sc-server"
	app.Usage = "implements service requests and SMS handling"
	app.Version = "0.1"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "n,nsqd",
			Usage: "a nsqd server address",
		},
	}
}

func main() {
	app.Action = func(c *cli.Context) {
		if !c.IsSet("nsqd") {
			log.Fatalln("no nsqd address is set")
		}
		cfg := nsq.NewConfig()
		cfg.UserAgent = "sc-server/0.1"
		consumer, err := nsq.NewConsumer(subTopic, subChannel, cfg)
		if err != nil {
			log.Fatalln(err)
		}
		consumer.AddHandler(nsq.HandlerFunc(handleIncomingSMS))
		consumer.SetLogger(nsqLog, nsq.LogLevelDebug)
		if err = consumer.ConnectToNSQD(c.String("nsqd")); err != nil {
			log.Fatalln(err)
		}
		<-consumer.StopChan
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}

func handleIncomingSMS(nmsg *nsq.Message) (err error) {
	var msg misc.Message
	if err = json.Unmarshal(nmsg.Body, &msg); err != nil {
		return
	}

	spew.Dump(msg)
	return
}
