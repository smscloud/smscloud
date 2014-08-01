package main

import (
	"fmt"
	"log"
	"os"

	"github.com/codegangsta/cli"
	"github.com/xlab/smscloud/wolfram"
)

const apiKey = "A6393P-2Y4J5AA4UW"

var app = cli.NewApp()

func init() {
	app.Name = "wolq"
	app.Usage = "make a request to Wikipedia using ./wolq <text>"
	app.Version = "0.1a"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "k, key",
			Value: apiKey,
			Usage: "wolfram API key",
		},
	}
}

func main() {
	app.Action = func(c *cli.Context) {
		api := wolfram.NewApi(c.String("key"), "Russia")
		res, err := api.Query(c.Args().First())
		if err != nil {
			log.Fatalln(err)
		}
		if len(res.Pods) < 2 || len(res.Pods[1].SubPods) < 1 {
			log.Fatalln("wolfram: unable to compute any result")
		}
		fmt.Println(res.Pods[1].SubPods[0].Plaintext)
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}
