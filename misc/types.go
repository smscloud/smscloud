package misc

import (
	"time"

	"github.com/xlab/at/sms"
)

type Message struct {
	UUID      []byte
	Origin    string // receiver's name
	Timestamp time.Time
	Msg       *sms.Message
}

type Notification struct {
	Kind  NotifyType
	Value string // frontend JS value = who cares
}

type NotifyType int

const (
	NotifyReceived NotifyType = iota
	NotifySuccess
	NotifyReserve
	NotifyError
)

func (n NotifyType) String() string {
	switch n {
	case NotifyReceived:
		return "Message received"
	case NotifySuccess:
		return "Query succeed"
	case NotifyReserve:
		return "Reserved amount changed"
	case NotifyError:
		return "Error occured"
	default:
		return "Unknown"
	}
}
