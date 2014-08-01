package misc

import (
	"crypto/rand"
	"errors"
)

// http://www.ashishbanerjee.com/home/go/go-generate-uuid
func GenUUID() ([]byte, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, errors.New("uuid gen failed: " + err.Error())
	}
	// TODO: verify the two lines implement RFC 4122 correctly
	buf[8] = 0x80 // variant bits see page 5
	buf[4] = 0x40 // version 4 Pseudo Random, see page 7
	return buf, nil
}
