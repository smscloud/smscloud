package misc

import "time"

type Message struct {
	UUID        []byte
	Origin      string // receiver's name
	Text        string
	Address     string // sender's address
	Timestamp   time.Time
	OpTimestamp time.Time
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
