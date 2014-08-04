package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"

	"github.com/bitly/go-nsq"
	"github.com/codegangsta/cli"
	"github.com/xlab/smscloud/googl"
	"github.com/xlab/smscloud/misc"
	"github.com/xlab/smscloud/wikipedia"
	"github.com/xlab/smscloud/wolfram"
)

const (
	subTopic   = "messages"
	subChannel = "sc_server"
)

const (
	latinSize = 306
	cyrSize   = 134
	urlLen    = 13
)

const (
	reqPending int8 = 0
	reqDone
	reqError
)

const originCountry = "Russia"
const wolframUserURI = "http://www.wolframalpha.com/input/"

var supportedServices = map[string]bool{
	"wolfram":   true,
	"wikipedia": true,
}

var app = cli.NewApp()

var (
	nsqLog = log.New(os.Stderr, "sc-server: ", log.LstdFlags)
	dbLog  = log.New(os.Stderr, "sc-server: ", 0)
)

func init() {
	log.SetPrefix("sc-server: ")
	log.SetFlags(log.Lshortfile)
	app.Name = "sc-server"
	app.Usage = "implements service requests and SMS handling"
	app.Version = "0.1"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "n,nsqd",
			Usage: "an nsqd server address",
		},
		cli.StringFlag{
			Name:  "d,db-cfg",
			Value: "database.json",
			Usage: "a database config file",
		},
		cli.StringFlag{
			Name:  "s,sc-cfg",
			Value: "services.json",
			Usage: "a services config file",
		},
		cli.StringFlag{
			Name:  "c,cred-cfg",
			Value: "credentials.json",
			Usage: "an API credentials config file",
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

		hCfg := &handlerConfig{
			NsqAddr:     c.String("nsqd"),
			NsqCfg:      cfg,
			DbCfg:       &dbConfig{},
			Credentials: &credConfig{},
		}
		if err := hCfg.DbCfg.ReadFromFile(c.String("db-cfg")); err != nil {
			log.Fatalln(err)
		}
		if err := hCfg.ServicesCfg.ReadFromFile(c.String("sc-cfg")); err != nil {
			log.Fatalln(err)
		}
		if err := hCfg.Credentials.ReadFromFile(c.String("cred-cfg")); err != nil {
			log.Fatalln(err)
		}
		handler, err := NewMessageHandler(hCfg)
		if err != nil {
			log.Fatalln(err)
		}
		consumer, err := nsq.NewConsumer(subTopic, subChannel, cfg)
		if err != nil {
			log.Fatalln(err)
		}
		consumer.AddHandler(handler)
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

type servicesConfig map[string]string

func (s *servicesConfig) ReadFromFile(name string) error {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, s)
}

type credConfig struct {
	GooglePrivateKey  string
	GoogleClientEmail string
	WolframPrivateKey string
}

func (c *credConfig) ReadFromFile(name string) error {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, c)
}

type dbConfig struct {
	Addr     string `json:"address"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

func (d *dbConfig) ReadFromFile(name string) error {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, d)
}

func (d *dbConfig) DataSourceName() string {
	connectStr := "postgres://%s:%s@%s/%s?sslmode=disable"
	return fmt.Sprintf(connectStr, d.User, d.Password, d.Addr, d.DBName)
}

type handlerConfig struct {
	NsqAddr     string
	NsqCfg      *nsq.Config
	DbCfg       *dbConfig
	Credentials *credConfig
}

type MessageHandler struct {
	stats    *nsq.Producer
	db       gorm.DB
	wolfApi  *wolfram.Api
	wikiApi  *wikipedia.Api
	googlApi *googl.Shortener
}

type Request struct {
	Id            int64
	Text          string `sql:"size:160"`
	Address       string `sql:"size:20"`
	Service       string `sql:"size:20"`
	ServiceReply  string
	ShortURL      string `sql:"size:20"`
	RequestStatus int8
	Timestamp     time.Time
	OpTimestamp   time.Time
}

func NewMessageHandler(cfg *handlerConfig) (h *MessageHandler, err error) {
	h = &MessageHandler{
		services: cfg.ServicesCfg,
		wikiApi:  wikipedia.NewApi(),
		wolfApi:  wolfram.NewApi(cfg.Credentials.WolframPrivateKey, originCountry),
	}
	if h.googlApi, err = googl.NewShortener(
		cfg.Credentials.GoogleClientEmail,
		[]byte(cfg.Credentials.GooglePrivateKey),
	); err != nil {
		return
	}
	if h.db, err = gorm.Open("postgres", cfg.DbCfg.DataSourceName()); err != nil {
		return nil, err
	}
	if err = h.db.AutoMigrate(Request{}).Error; err != nil {
		return nil, err
	}
	if h.stats, err = nsq.NewProducer(cfg.NsqAddr, cfg.NsqCfg); err != nil {
		return nil, err
	}
	return
}

func (m *MessageHandler) HandleMessage(nmsg *nsq.Message) (err error) {
	var msg misc.Message
	if err = json.Unmarshal(nmsg.Body, &msg); err != nil {
		return
	}
	if !supportedServices[msg.Origin] {
		nmsg.Finish()
		return errors.New("message for unsupported service: " + msg.Origin)
	}
	req := Request{
		Text:          msg.Text,
		Address:       msg.Address,
		Service:       msg.Origin,
		RequestStatus: reqPending,
		Timestamp:     time.Time(msg.Timestamp),
		OpTimestamp:   time.Time(msg.OpTimestamp),
	}
	if err = m.db.Create(&req).Error; err != nil {
		return
	}
	log.Printf("stored message %x", msg.UUID)
	defer func(r *Request) {
		if err = m.db.Save(r).Error; err != nil {
			log.Printf("error saving query %x: %s", msg.UUID, err.Error())
		}
	}(&req)
	switch msg.Origin {
	case "wolfram":
		reply, err := m.queryWolfram(msg.Text)
		if err != nil {
			req.RequestStatus = reqError
			log.Printf("error querying %x: %s", msg.UUID, err.Error())
			return
		}
		if short, err := m.googlApi.Short(wolframURL(input)); err != nil {
			log.Printf("error shorting url for %x: %s", msg.UUID, err.Error())
		} else {
			req.ShortURL = short.String()
		}
		req.RequestStatus = reqDone
		req.ServiceReply = wrapReply(reply)
	case "wikipedia":
		reply, uri, err := m.queryWikipedia(msg.Text)
		if err != nil {
			req.RequestStatus = reqError
			log.Printf("error querying %x: %s", msg.UUID, err.Error())
			return
		}
		if short, err := m.googlApi.Short(genericURL(uri)); err != nil {
			log.Printf("error shorting url for %x: %s", msg.UUID, err.Error())
		} else {
			req.ShortURL = short.String()
		}
		req.RequestStatus = reqDone
		req.ServiceReply = wrapReply(reply)
	}
	// TODO:
	// send reply
	// send notification
	// check everything
	// do the web part
	return
}

func (m *MessageHandler) queryWolfram(input string) (reply string, err error) {
	var res *wolfram.QueryResult
	if res, err = m.wolfApi.Query(input); err != nil {
		err = errors.New("unable to query wolfram: " + err.Error())
		return
	}
	if len(res.Pods) < 2 || len(res.Pods[1].SubPods) < 1 {
		return
	}
	reply = res.Pods[1].SubPods[0].Plaintext
	return
}

func (m *MessageHandler) queryWikipedia(lang wikipedia.Language, input string) (reply, uri string, err error) {
	var res *wikipedia.SearchSuggestion
	if res, err = m.wikiApi.Query(input); err != nil {
		err = errors.New("unable to query wikipedia: " + err.Error())
		return
	}
	if len(res.Items) < 1 {
		return
	}
	reply = res.Items[0].Description
	uri = res.Items[0].URL
	return
}

func wolframURL(input string) *url.URL {
	v := url.Values{}
	v.Set("i", msg.Text)
	u, _ := url.ParseRequestURI(wolframUserURI)
	u.RawQuery = v.Encode()
	return u
}

func genericURL(uri string) (u *url.URL) {
	u, _ = url.ParseRequestURI(uri)
	return
}

// wrapReply is a very dumb sanitizer.
func wrapReply(reply string) string {
	if len(reply) < 1 {
		return reply
	}
	r := strings.NewReplacer(
		"  ", " ", ". ", ".", ", ", ",", "; ", ";",
		"  ", " ", " .", ".", " ,", ",", " ;", ";",
		" () ", " ", "()", "", " - ", " ", " – ", " ", " — ", " ",
		" ( ) ", " ", " ( or  ) ", " ", "; ; ", "", "(, ", "(",
	)
	reply = r.Replace(reply)
	reply = cutStr(reply, latinSize-urlLen)
	if len(reply != len([]byte(reply))) {
		reply = cutStr(reply, cyrSize-urlLen)
	}
	reply = strings.TrimSpace(reply)
	return reply
}

func cutStr(str string, n int) string {
	runes := []rune(str)
	return string(runes[0:n])
}
