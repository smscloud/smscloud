package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/codegangsta/cli"
	"github.com/xlab/smscloud/wikipedia"
)

var app = cli.NewApp()

func init() {
	app.Name = "wiki"
	app.Usage = "make a request to Wikipedia using ./wiki <text>"
	app.Version = "0.1a"
}

var ErrNotFound = errors.New("wikipedia: not found")

func main() {
	app.Action = func(c *cli.Context) {
		api := wikipedia.NewApi()
		var res *wikipedia.SearchSuggestion

		search := func(lang wikipedia.Language, search string) (desc string, err error) {
			res, err = api.Query(lang, search)
			if err != nil {
				return
			}
			if len(res.Items) < 1 {
				err = ErrNotFound
				return
			}
			desc = res.Items[0].Description
			return
		}

		// search EN first
		desc, err := search(wikipedia.EN, c.Args().First())
		if err != nil {
			// then RU
			if desc, err = search(wikipedia.RU, c.Args().First()); err != nil {
				log.Fatalln(err)
			}
		}
		fmt.Println(desc)
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}
