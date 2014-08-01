package googl

import (
	"net/url"

	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/urlshortener/v1"
)

type Shortener struct {
	transport *jwt.Transport
	svc       *urlshortener.Service
}

func NewShortener(issuer string, key []byte) (s *Shortener, err error) {
	s = &Shortener{}
	token := jwt.NewToken(issuer, urlshortener.UrlshortenerScope, key)
	if s.transport, err = jwt.NewTransport(token); err != nil {
		return
	}
	s.svc, err = urlshortener.New(s.transport.Client())
	return
}

func (s *Shortener) Short(u *url.URL) (short *url.URL, err error) {
	tmp := &urlshortener.Url{
		LongUrl: u.String(),
	}
	if tmp, err = s.svc.Url.Insert(tmp).Do(); err != nil {
		return
	}
	return url.ParseRequestURI(tmp.Id)
}
