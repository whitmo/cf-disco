package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/apcera/nats"
	"github.com/op/go-logging"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
	"flag"
)

type Report struct {
	Type        string
	Host        string
	Uid         string
	Index       int
	Credentials [2]string
}

type HealthzOuput struct {
	ok bool
}

type VarzOuput struct {

}

type Requester struct {
	http *http.Client
}

var checkType string

var log *logging.Logger

func main() {
	init_flags()
	flag.Parse()
	dtype := flag.Arg(1)
	ip := flag.Arg(2)
	nsrv := flag.Arg(0)

	log = logging_setup("cdisco")

	rep, err := listen_for_report(nsrv, ip, dtype)
	if err != nil {
		log.Error("%s", fmt.Errorf("%s", err))
		os.Exit(1)
	}
	client := &http.Client{}
	reqr := &Requester{http: client}

	var out string
	var err_ error
	if checkType == "varz" {
		varz, e := reqr.check_varz(&rep)
		err_ = e
		out = string(varz)
	} else {
		healthz, e := reqr.check_healthz(&rep)	
		err_ = e
		if healthz {
			out = "ok"
		} else {
			out = "not ok"
			os.Exit(1)
		}
	}
	if err_ != nil {
		//fmt.Errorf("%s", err)
		log.Error("%s %s", out, err)
		os.Exit(1)
	}
	fmt.Println(out)
}




func read_subs(results chan Report, ip string, dtype string, poll func(time.Duration)(*nats.Msg, error), timeout time.Duration) {
	var result Report
	for {
		m, _ := poll(time.Second)
		if m != nil {
			rep, err := decode(m)
			if err != nil {
				log.Error("%s", err)
			}
			if rep.match(ip, dtype) {
				result = rep
				break
			}
		}

	}
	results <- result
	return 
}


// Connects to nats and waits for component announcement matching our ip & type
func listen_for_report(nsrv string, ip string, dtype string) (Report, error) {
	var DISCOVER = "vcap.component.discover"
	var SHARED_INBOX = "juju.component.announce"

	nc, _ := nats.Connect(nsrv)
	sub, _ := nc.SubscribeSync(SHARED_INBOX)
	err := nc.PublishRequest(DISCOVER, SHARED_INBOX, []byte(""))
	if err != nil {
		return Report{}, errors.New("Failed Publishing")
	}

	results := make(chan Report, 1)
	go read_subs(results, ip, dtype, sub.NextMsg, time.Second)

	select {
	case res := <-results:
		return res, nil
	case <-time.After(time.Second * 5):
		return Report{}, errors.New("Timeout")
	}
}

func (r *Requester) httpget(host string, path string, creds [2]string) (*http.Response, error) {
	url := fmt.Sprintf("http://%s/%s", host, path)
	log.Debug(url)
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(string(creds[0]), string(creds[1]))
	resp, err := r.http.Do(req)
	return resp, err
}

func (r *Requester) check_healthz(report *Report) (bool, error) {
	resp, err := r.httpget(report.Host, "healthz", report.Credentials)
	if err != nil {
		log.Error("httpget: %s", resp, err)
		return false, err
	}

	if resp.StatusCode != 200 {
		return false, errors.New(resp.Status)
	}

	body, read_err := ioutil.ReadAll(resp.Body)
	if read_err != nil {
		return false, read_err
	}

	if string(body) != "ok" {
		return false, nil
	}

	return true, nil
}

func (r *Requester) check_varz(report *Report) ([]byte, error) {
	resp, err := r.httpget(report.Host, "varz", report.Credentials)
	empty_bs := []byte("")
	if err != nil {
		return empty_bs, err
	}
	body, read_err := ioutil.ReadAll(resp.Body)
	if read_err != nil {
		return []byte(""), read_err
	}
	if resp.StatusCode != 200 {
		return body, errors.New(resp.Status)
	}
	return body, nil
}

func (rep *Report) match(ip string, dtype string) bool {
	host := strings.Split(rep.Host, ":")
	if host[0] == ip && dtype == rep.Type {
		return true
	}
	return false
}

func decode(msg *nats.Msg) (Report, error) {
	var report Report
	err := json.Unmarshal(msg.Data, &report)
	return report, err
}

func init_flags(){
	const (
		defaultCheck = "healthz"
		usage        = "which check to make: varz or healthz"
	)
	flag.StringVar(&checkType, "check", defaultCheck, usage)
}


func logging_setup(name string) *logging.Logger {
	var log = logging.MustGetLogger(name)
	var format = logging.MustStringFormatter(
		"%{color}%{time:15:04:05.000000} %{shortfile} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}",
	)
	lback := logging.NewLogBackend(os.Stderr, "", 0)
	lbackf := logging.NewBackendFormatter(lback, format)
	logging.SetBackend(lbackf)
	return log
}

// // errorString is a trivial implementation of error.
// type errorString struct {
// 	s string
// }

// func (e *errorString) Error() string {
// 	return e.s
// }

// // New returns an error that formats as the given text.
// func New(text string) error {
// 	return &errorString{text}
// }
