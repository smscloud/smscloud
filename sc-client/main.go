package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/bitly/go-nsq"
	"github.com/codegangsta/cli"
	"github.com/xlab/smscloud/misc"
)

const smsMsgThroughput = 200

const (
	pubTopic = "messages"
)

var app = cli.NewApp()
var nsqLog = log.New(os.Stderr, "sc-client: ", log.LstdFlags)

func init() {
	log.SetPrefix("sc-client: ")
	log.SetFlags(log.Lshortfile)
	app.Name = "sc-client"
	app.Usage = "communicates with modems and handles incomig SMS"
	app.Version = "0.1"
	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "p, port",
			Value: 5051,
			Usage: "a port to bind the web inteface",
		},
		cli.StringFlag{
			Name:  "d, dir",
			Value: "modems",
			Usage: "a directory where modem configuration files should be searched",
		},
		cli.StringFlag{
			Name:  "n,nsqd",
			Usage: "a nsqd server address",
		},
	}
}

var supportedNames = map[string]bool{
	"wolfram":   true,
	"wikipedia": true,
}

func main() {
	app.Action = func(c *cli.Context) {
		if !c.IsSet("nsqd") {
			log.Fatalln("no nsqd address is set")
		}

		// nsq setup
		cfg := nsq.NewConfig()
		cfg.UserAgent = "sc-client/0.1"
		producer, err := nsq.NewProducer(c.String("nsqd"), cfg)
		if err != nil {
			log.Fatalln(err)
		}
		producer.SetLogger(nsqLog, nsq.LogLevelDebug)

		// modems handler setup
		hdl, err := newMonitorHandler(c.String("dir"))
		if err != nil {
			log.Fatalln(err)
		}
		go func() {
			for msg := range hdl.messages {
				body, err := json.Marshal(msg)
				if err != nil {
					log.Printf("failed to marshal msg %x", msg.UUID)
					continue
				}
				if err = producer.Publish(pubTopic, body); err != nil {
					log.Printf("failed to publish msg %x", msg.UUID)
				}
			}
		}()
		hdl.Handle(c.Int("port"))
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}

type monitorHandler struct {
	mons     []*Monitor
	messages chan *misc.Message
}

func (m *monitorHandler) Handle(port int) {
	var wg sync.WaitGroup
	for _, mon := range m.mons {
		wg.Add(1)
		go func(mon *Monitor) {
			if err := mon.Run(); err != nil {
				log.Println(err)
			}
			wg.Done()
		}(mon)
		http.Handle("/"+strings.ToLower(mon.Name()), mon)
	}
	go func() {
		log.Printf("spawning web interface at http://localhost:%d", port)
		if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
			log.Fatalln(err)
		}
	}()
	wg.Wait()
	close(m.messages)
	log.Println("sc-client: all monitors halted")
}

func newMonitorHandler(dir string) (handler *monitorHandler, err error) {
	handler = &monitorHandler{}
	handler.messages = make(chan *misc.Message, smsMsgThroughput)
	var list []os.FileInfo
	if list, err = ioutil.ReadDir(dir); err != nil {
		err = errors.New("sc-client: unable to read config dir " + dir)
		return
	}
	for i := range list {
		path := filepath.Join(dir, list[i].Name())
		if filepath.Ext(path) != ".json" {
			continue
		}
		if cfg, err := loadConfig(path); err != nil {
			continue
		} else {
			handler.mons = append(handler.mons, NewMonitor(handler.messages, cfg))
		}
	}
	if len(handler.mons) < 1 {
		return nil, errors.New("sc-client: no modem configs found in " + dir)
	}
	return
}

func loadConfig(path string) (cfg *MonitorConfig, err error) {
	var buf []byte
	if buf, err = ioutil.ReadFile(path); err != nil {
		return
	}
	cfg = &MonitorConfig{}
	if err = json.Unmarshal(buf, &cfg); err != nil {
		log.Println(err)
		return
	}
	if !supportedNames[cfg.ModemName] {
		err = errors.New("sc-client: unsupported modem name " + cfg.ModemName)
		log.Println(err)
		return
	}
	return
}
