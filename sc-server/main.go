package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"

	"github.com/bitly/go-nsq"
	"github.com/codegangsta/cli"
	"github.com/xlab/smscloud/googl"
	"github.com/xlab/smscloud/misc"
	"github.com/xlab/smscloud/wikipedia"
	"github.com/xlab/smscloud/wolfram"
	"github.com/xlab/smsru"
)

const (
	pubTopic   = "notifications"
	subTopic   = "messages"
	subChannel = "sc_server"
)

const (
	latinSize = 306
	cyrSize   = 134
	urlLen    = 21
)

const (
	reqPending int8 = iota
	reqDone
	reqError
)

const approxSmsCost = 0.70
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

type credConfig struct {
	GooglePrivateKey  string `json:"google_private_key"`
	GoogleClientEmail string `json:"google_client_email"`
	WolframPrivateKey string `json:"wolfram_private_key"`
	SmsruPrivateKey   string `json:"smsru_private_key"`
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
	smsApi   *smsru.Api
}

type Request struct {
	Id            int64
	Text          string `sql:"size:160"`
	Address       string `sql:"size:20"`
	Service       string `sql:"size:20"`
	ServiceReply  string
	ShortUrl      string `sql:"size:20"`
	RequestStatus int8
	Timestamp     time.Time
	OpTimestamp   time.Time
}

func NewMessageHandler(cfg *handlerConfig) (h *MessageHandler, err error) {
	h = &MessageHandler{
		wikiApi: wikipedia.NewApi(),
		wolfApi: wolfram.NewApi(cfg.Credentials.WolframPrivateKey, originCountry),
		smsApi:  smsru.NewApi(cfg.Credentials.SmsruPrivateKey),
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
		log.Println("error storing message:", err)
		m.notifyError()
		return
	}
	log.Printf("stored message %x", msg.UUID)
	m.notifyReceived()
	// save request updates on return
	defer func(r *Request) {
		if err = m.db.Save(r).Error; err != nil {
			log.Printf("error saving query %x: %s", msg.UUID, err.Error())
			m.notifyError()
		}
	}(&req)
	switch msg.Origin {
	case "wolfram":
		reply, err := m.queryWolfram(msg.Text)
		if err != nil {
			req.RequestStatus = reqError
			log.Printf("error querying %x: %s", msg.UUID, err.Error())
			m.notifyError()
			return nil
		}
		if short, err := m.googlApi.Short(wolframURL(msg.Text)); err != nil {
			log.Printf("error shorting url for %x: %s", msg.UUID, err.Error())
			m.notifyError()
		} else {
			req.ShortUrl = short.String()
		}
		req.RequestStatus = reqDone
		req.ServiceReply = wrapReply(reply)
		m.notifySuccess()
	case "wikipedia":
		reply, uri, err := m.queryWikipedia(wikipedia.EN, msg.Text)
		if err != nil || len(reply) < 1 {
			reply, uri, err = m.queryWikipedia(wikipedia.RU, msg.Text)
		}
		if err != nil {
			req.RequestStatus = reqError
			log.Printf("error querying %x: %s", msg.UUID, err.Error())
			m.notifyError()
			return nil
		}
		if short, err := m.googlApi.Short(genericURL(uri)); err != nil {
			log.Printf("error shorting url for %x: %s", msg.UUID, err.Error())
			m.notifyError()
		} else {
			req.ShortUrl = short.String()
		}
		req.RequestStatus = reqDone
		req.ServiceReply = wrapReply(reply)
		m.notifySuccess()
	}
	if err = m.sendReply(&req); err != nil {
		log.Println("error sending reply:", err)
		m.notifyError()
		return nil
	}
	if r, err := m.getReserve(); err != nil {
		log.Println("failed to get reserve:", err)
		m.notifyError()
	} else {
		m.notifyReserve(r)
	}
	return
}

func (m *MessageHandler) getReserve() (r int, err error) {
	var balance float32
	if balance, err = m.smsApi.MyBalance(); err != nil {
		return
	}
	return int(balance / approxSmsCost), nil
}

func (m *MessageHandler) notifyError() (err error) {
	n := misc.Notification{Kind: misc.NotifyError}
	body, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if err = m.stats.Publish(pubTopic, body); err != nil {
		log.Println(err)
	}
	return
}

func (m *MessageHandler) notifySuccess() (err error) {
	n := misc.Notification{Kind: misc.NotifySuccess}
	body, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if err = m.stats.Publish(pubTopic, body); err != nil {
		log.Println(err)
	}
	return
}

func (m *MessageHandler) notifyReceived() (err error) {
	n := misc.Notification{Kind: misc.NotifyReceived}
	body, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if err = m.stats.Publish(pubTopic, body); err != nil {
		log.Println(err)
	}
	return
}

func (m *MessageHandler) notifyReserve(reserve int) (err error) {
	n := misc.Notification{
		Kind:  misc.NotifyReserve,
		Value: strconv.Itoa(reserve),
	}
	body, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if err = m.stats.Publish(pubTopic, body); err != nil {
		log.Println(err)
	}
	return
}

func (m *MessageHandler) queryWolfram(input string) (reply string, err error) {
	var res *wolfram.QueryResult
	if res, err = m.wolfApi.Query(input); err != nil {
		if err == wolfram.ErrUnknown {
			return "", nil // nothing computed
		}
		return "", err
	}
	if len(res.Pods) < 2 || len(res.Pods[1].SubPods) < 1 {
		return
	}
	reply = res.Pods[1].SubPods[0].Plaintext
	return
}

func (m *MessageHandler) queryWikipedia(lang wikipedia.Language, input string) (reply, uri string, err error) {
	var res *wikipedia.SearchSuggestion
	if res, err = m.wikiApi.Query(lang, input); err != nil {
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
	v.Set("i", input)
	u, _ := url.ParseRequestURI(wolframUserURI)
	u.RawQuery = v.Encode()
	return u
}

func genericURL(uri string) (u *url.URL) {
	u, _ = url.ParseRequestURI(uri)
	return
}

func (m *MessageHandler) sendError(req *Request) error {
	t := req.OpTimestamp.Format(`2 Jan 15:04`)
	text := fmt.Sprintf("Внутренняя ошибка обработки запроса от %s", t)
	sms := smsru.Sms{
		To:   req.Address,
		Text: text,
		Test: true,
	}
	_, err := m.smsApi.SmsSend(&sms)
	return err
}

func (m *MessageHandler) sendReply(req *Request) error {
	var text string
	if len(req.ServiceReply) < 1 {
		t := req.OpTimestamp.Format(`2 Jan 15:04`)
		text = fmt.Sprintf("На ваш запрос от %s не удалось получить ответ %s", t, req.ShortUrl)
	} else {
		text = req.ServiceReply
		if isLatin(text) {
			text = text + " " + req.ShortUrl
		}
	}
	sms := smsru.Sms{
		To:   req.Address,
		Text: text,
	}
	cost, n, err := m.smsApi.SmsCost(&sms)
	if err != nil {
		return err
	}
	log.Println("reply to", req.Address, "is:", text)
	log.Println("sent", n, "messages, total cost", cost)
	if _, err = m.smsApi.SmsSend(&sms); err != nil {
		return err
	}
	return nil
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
		" ( ) ", " ", " ( or  ) ", " ", "; ; ", "", "(, ", "(", " | ", "|",
	)
	reply = r.Replace(reply)
	reply = cutStr(reply, latinSize-urlLen)
	if !isLatin(reply) {
		reply = cutStr(reply, cyrSize)
	}
	reply = strings.TrimSpace(reply)
	return reply
}

func cutStr(str string, n int) string {
	runes := []rune(str)
	if n < len(str) {
		return string(runes[0:n])
	}
	return str
}

func isLatin(text string) bool {
	for _, r := range text {
		if !unicode.In(r, unicode.Latin) {
			return false
		}
	}
	return true
}
