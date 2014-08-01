package googl

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

var file = "service_account.json"

func TestShort(t *testing.T) {
	buf, err := ioutil.ReadFile(file)
	if !assert.NoError(t, err) {
		return
	}
	cred := struct {
		Issuer string `json:"client_email"`
		Key    string `json:"private_key"`
	}{}
	if !assert.NoError(t, json.Unmarshal(buf, &cred)) {
		return
	}
	svc, err := NewShortener(cred.Issuer, []byte(cred.Key))
	if !assert.NoError(t, err) {
		return
	}
	u, _ := url.ParseRequestURI("http://yandex.ru")
	short, err := svc.Short(u)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, short)
	}
	log.Println(short)
}
