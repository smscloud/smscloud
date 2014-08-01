package wolfram

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/xlab/api"
)

const baseURI = "http://api.wolframalpha.com/v2"

var (
	ErrUnknown = errors.New("wolfram: unknown error")
	ErrEmpty   = errors.New("wolfram: empty input")
)

type SubPod struct {
	Title     string `xml:"title,attr"`
	Plaintext string `xml:"plaintext"`
}

type Pod struct {
	Title   string    `xml:"title,attr"`
	ID      string    `xml:"id,attr"`
	Primary bool      `xml:"primary,attr"`
	SubPods []*SubPod `xml:"subpod"`
}

type QueryResult struct {
	IsSuccess bool    `xml:"success,attr"`
	IsError   bool    `xml:"error,attr"`
	Timing    float32 `xml:"timing,attr"`
	Err       *Err    `xml:"error"`
	Pods      []*Pod  `xml:"pod"`
}

type Err struct {
	Code    int    `xml:"code"`
	Message string `xml:"msg"`
}

func (e *Err) Error() error {
	return errors.New("wolfram: " + e.Message)
}

type Api struct {
	Key      string
	Location string
	*api.Api
}

func NewApi(key string, loc string) *Api {
	return &Api{
		Key:      key,
		Location: loc,
		Api:      api.MustNew(baseURI),
	}
}

func (a *Api) Query(input string) (res *QueryResult, err error) {
	if len(input) < 1 {
		err = ErrEmpty
		return
	}
	var cli http.Client
	var req *http.Request
	var resp *http.Response
	args := url.Values{}
	args.Set("input", input)
	if req, err = a.request(api.GET, "/query", args); err != nil {
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
	res = &QueryResult{}
	if err = xml.Unmarshal(data, res); err != nil {
		return
	}
	if res.IsError {
		return nil, res.Err.Error()
	}
	if !res.IsSuccess {
		return nil, ErrUnknown
	}
	return
}

func (a *Api) request(m api.Method, res string, args url.Values) (*http.Request, error) {
	base := a.values()
	for k := range args {
		base.Add(k, args.Get(k))
	}
	return a.Request(m, res, base)
}

func (a *Api) values() url.Values {
	args := url.Values{}
	args.Set("appid", a.Key)
	args.Set("location", a.Location)
	args.Set("format", "plaintext")
	args.Set("podindex", "1,2") // only interpretation and result
	args.Set("width", "300")
	args.Set("primary", "true")
	args.Set("reinterpret", "true")
	return args
}
