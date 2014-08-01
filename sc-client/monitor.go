package main

import (
	"log"
	"time"

	"github.com/xlab/at"
	"github.com/xlab/smscloud/misc"
)

const (
	BalanceCheckInterval = time.Minute
	DeviceCheckInterval  = time.Second * 10
)

type State uint8

const (
	NoDeviceState State = iota
	ReadyState
)

type Monitor struct {
	// Balance is the balance reply we've got with an USSD query.
	Balance string
	// Ready signals if device is ready.
	Ready bool

	name        string
	cmdPort     string
	notifyPort  string
	balanceUSSD string

	dev          *at.Device
	stateChanged chan State
	messages     chan<- *misc.Message
	checkTimer   *time.Timer
	uptimeBase   time.Time
}

func (m *Monitor) Name() string {
	return m.name
}

func (m *Monitor) DeviceState() *at.DeviceState {
	return m.dev.State
}

func (m *Monitor) Uptime() time.Duration {
	if m.uptimeBase.IsZero() {
		return 0
	}
	return time.Since(m.uptimeBase)
}

type MonitorConfig struct {
	ModemName       string `json:"name"`
	CommandPortPath string `json:"command_port_path"`
	NotifyPortPath  string `json:"notify_port_path"`
	BalanceUSSD     string `json:"balance_ussd"`
}

func NewMonitor(messages chan<- *misc.Message, cfg *MonitorConfig) *Monitor {
	return &Monitor{
		name:        cfg.ModemName,
		cmdPort:     cfg.CommandPortPath,
		notifyPort:  cfg.NotifyPortPath,
		balanceUSSD: cfg.BalanceUSSD,

		messages:     messages,
		stateChanged: make(chan State, 10),
	}
}

func (m *Monitor) devStop() {
	if m.dev != nil {
		m.dev.Close()
	}
}

func (m *Monitor) Run() (err error) {
	m.checkTimer = time.NewTimer(DeviceCheckInterval)
	defer m.checkTimer.Stop()
	defer m.devStop()

	go func() {
		for {
			<-m.checkTimer.C
			if err := m.openDevice(); err != nil {
				m.checkTimer.Reset(DeviceCheckInterval)
				continue
			} else {
				m.checkTimer.Stop()
				m.stateChanged <- ReadyState
			}
		}
	}()

	if err := m.openDevice(); err != nil {
		m.stateChanged <- NoDeviceState
	} else {
		m.stateChanged <- ReadyState
		m.checkTimer.Stop()
	}

	for s := range m.stateChanged {
		switch s {
		case NoDeviceState:
			m.Balance = ""
			m.Ready = false
			m.uptimeBase = time.Time{}
			log.Printf("sc-client: waiting for device %s", m.dev.NotifyPort)
			m.checkTimer.Reset(DeviceCheckInterval)
		case ReadyState:
			log.Printf("sc-client: device connected %s", m.dev.NotifyPort)
			m.Ready = true
			m.uptimeBase = time.Now()
			go func() {
				m.dev.Watch()
				m.stateChanged <- NoDeviceState
			}()
			go func() {
				m.dev.SendUSSD(m.balanceUSSD)
				t := time.NewTicker(BalanceCheckInterval)
				defer t.Stop()
				for {
					select {
					case <-m.dev.Closed():
						return
					case ussd := <-m.dev.UssdReply():
						m.Balance = string(ussd)
					case msg := <-m.dev.IncomingSms():
						uuid, err := misc.GenUUID()
						if err != nil {
							log.Fatalln(err)
						}
						wrap := &misc.Message{
							UUID:      uuid,
							Origin:    m.name,
							Timestamp: time.Now(),
							Msg:       msg,
						}
						m.messages <- wrap
					case <-t.C:
						m.dev.SendUSSD(m.balanceUSSD)
					}
				}
			}()
		}
	}

	return
}

func (m *Monitor) openDevice() (err error) {
	m.dev = &at.Device{
		CommandPort: m.cmdPort,
		NotifyPort:  m.notifyPort,
	}
	if err = m.dev.Open(); err != nil {
		return
	}
	if err = m.dev.Init(at.DeviceE173()); err != nil {
		return
	}
	return
}
