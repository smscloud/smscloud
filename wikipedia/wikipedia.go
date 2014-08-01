package wikipedia

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/xlab/api"
)

const baseRU = "http://ru.wikipedia.org/w/api.php"
const baseEN = "http://en.wikipedia.org/w/api.php"

type Language int

const (
	RU Language = iota
	EN
)

var (
	ErrUnknown = errors.New("wikipedia: unknown error")
	ErrEmpty   = errors.New("wikipedia: empty input")
)

type Item struct {
	Image struct {
		Source string `xml:"source,attr"`
		Width  int    `xml:"width,attr"`
		Heigth int    `xml:"height,attr"`
	} `xml:"Image"`
	Text        string `xml:"Text"`
	Description string `xml:"Description"`
	URL         string `xml:"Url"`
}

type SearchSuggestion struct {
	Err   *Err    `xml:"error"`
	Query string  `xml:"Query"`
	Items []*Item `xml:"Section>Item"`
}

type Err struct {
	Code string `xml:"code,attr"`
	Info string `xml:"info,attr"`
}

func (e *Err) Error() error {
	return errors.New("wikipedia: " + e.Info)
}

type Api struct {
	RU *api.Api
	EN *api.Api
}

func NewApi() *Api {
	return &Api{
		RU: api.MustNew(baseRU),
		EN: api.MustNew(baseEN),
	}
}

func (a *Api) Query(lang Language, input string) (res *SearchSuggestion, err error) {
	if len(input) < 1 {
		err = ErrEmpty
		return
	}
	var cli http.Client
	var req *http.Request
	var resp *http.Response
	args := url.Values{}
	args.Set("search", input)
	if req, err = a.request(lang, args); err != nil {
		return
	}
	// do a request
	if resp, err = cli.Do(req); err != nil {
		return
	}
	defer resp.Body.Close()
	var data []byte
	if data, err = ioutil.ReadAll(resp.Body); err != nil {
		return
	}
	// parse result
	res = &SearchSuggestion{}
	if err = xml.Unmarshal(data, res); err != nil {
		return
	}
	if res.Err != nil {
		return nil, res.Err.Error()
	}
	return
}

func (a *Api) request(lang Language, args url.Values) (*http.Request, error) {
	base := a.values()
	for k := range args {
		base.Add(k, args.Get(k))
	}
	if lang == RU {
		return a.RU.Request(api.GET, "", base)
	}
	return a.EN.Request(api.GET, "", base)
}

func (a *Api) values() url.Values {
	args := url.Values{}
	args.Set("format", "xml")
	args.Set("action", "opensearch")
	args.Set("limit", "1") // only first one
	return args
}
