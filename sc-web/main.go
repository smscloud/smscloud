package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/boltdb/bolt"
	"github.com/codegangsta/cli"
	"github.com/xlab/smscloud/misc"
)

var app = cli.NewApp()

var statsBucket = []byte("stats")

var (
	nsqLog = log.New(os.Stderr, "sc-server: ", log.LstdFlags)
	dbLog  = log.New(os.Stderr, "sc-server: ", 0)
)

var (
	ErrNotCounter     = errors.New("only counters can be incremented")
	ErrNotValue       = errors.New("only string values can be set")
	ErrBucketNotFound = errors.New("bucket not found")
)

func init() {
	log.SetPrefix("sc-web: ")
	log.SetFlags(log.Lshortfile)
	app.Name = "sc-web"
	app.Usage = "web service for smscloud"
	app.Version = "0.1"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "n,nsqd",
			Usage: "an nsqd server address",
		},
		cli.StringFlag{
			Name:  "d,db",
			Value: "state.db",
			Usage: "a name for the state db",
		},
		cli.IntFlag{
			Name:  "w,ws-port",
			Value: 15650,
			Usage: "a port for websocket connections",
		},
	}
}

func main() {
	app.Action = func(c *cli.Context) {
		if !c.IsSet("nsqd") {
			log.Fatalln("no nsqd address is set")
		}
		http.Handle("/", http.FileServer(http.Dir("/Users/xlab/Documents/Coding/Web/smscloud.org")))
		//http.Handle("/status", websocket.Handler(CounterServer))
		if err := http.ListenAndServe(":80", nil); err != nil {
			log.Fatalln(err)
		}
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}

type State struct {
	db *bolt.DB
}

func getTypeID(kind misc.NotifyType) []byte {
	switch kind {
	case misc.NotifySuccess:
		return []byte("success")
	case misc.NotifyReserve:
		return []byte("reserve")
	case misc.NotifyReceived:
		return []byte("received")
	case misc.NotifyError:
		return []byte("error")
	}
	panic("sc-web: unknown notify type id")
}

func isCounter(kind misc.NotifyType) bool {
	switch kind {
	case misc.NotifySuccess, misc.NotifyReceived, misc.NotifyError:
		return true
	}
	return false
}

func isValue(kind misc.NotifyType) bool {
	return kind == misc.NotifyReserve
}

func (s *State) IncCounter(kind misc.NotifyType) error {
	if !isCounter(kind) {
		return ErrNotCounter
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(statsBucket)
		if err != nil {
			return err
		}
		id := getTypeID(kind)
		c := Counter(0)
		c.FromBytes(bucket.Get(id))
		c += 1
		if err := bucket.Put(id, c.Bytes()); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func (s *State) SetValue(kind misc.NotifyType, value string) error {
	if !isValue(kind) {
		return ErrNotValue
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(statsBucket)
		if err != nil {
			return err
		}
		if err := bucket.Put(getTypeID(kind), []byte(value)); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func (s *State) GetCounter(kind misc.NotifyType) (Counter, error) {
	if !isCounter(kind) {
		return Counter(0), ErrNotCounter
	}
	var c Counter
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(statsBucket)
		if bucket == nil {
			return ErrBucketNotFound
		}
		return c.FromBytes(bucket.Get(getTypeID(kind)))
	})
	if err != nil {
		return Counter(0), err
	}
	return c, nil
}

func (s *State) GetValue(kind misc.NotifyType) (string, error) {
	if !isValue(kind) {
		return "", ErrNotValue
	}
	var v string
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(statsBucket)
		if bucket == nil {
			return ErrBucketNotFound
		}
		v = string(bucket.Get(getTypeID(kind)))
		return nil
	})
	if err != nil {
		return "", err
	}
	return v, nil
}

type Counter uint64

func (c Counter) Bytes() []byte {
	buf := make([]byte, 8)
	n := binary.PutUvarint(buf, uint64(c))
	return buf[:n]
}

func (c *Counter) FromBytes(b []byte) (err error) {
	var i uint64
	if i, err = binary.ReadUvarint(bytes.NewReader(b)); err != nil {
		return
	}
	*c = Counter(i)
	return
}
