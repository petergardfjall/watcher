package ping

import (
	"github.com/petergardfjall/watcher/config"

	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
)

// HTTPPinger is a Pinger that checks endpoints using the HTTP(S) protocol.
type HTTPPinger struct {
	Check config.HTTPCheck
}

// NewHTTPPinger creates a new pinger that checks endpoints using the HTTP(S)
// protocol.
func NewHTTPPinger(httpConfig *config.Pinger) (Pinger, error) {
	log.Debugf("setting up http pinger ...")
	var httpCheck config.HTTPCheck
	err := json.Unmarshal(httpConfig.Check, &httpCheck)
	if err != nil {
		return nil, fmt.Errorf("http pinger: illegal check: %s", err)
	}
	log.Debugf("http check: %#v", httpCheck)
	if err := httpCheck.Validate(); err != nil {
		return nil, fmt.Errorf("http pinger: invalid check: %s", err)
	}

	httpPinger := HTTPPinger{Check: httpCheck}
	return &httpPinger, nil

}

// Ping checks the health of the endpoint configured for this HTTPPinger.
func (httpPinger *HTTPPinger) Ping() (result Result, output *bytes.Buffer) {
	timeout := defaultHTTPTimeout
	if httpPinger.Check.Timeout != nil {
		timeout = httpPinger.Check.Timeout.Duration
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !httpPinger.Check.VerifyCert,
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{Timeout: timeout, Transport: transport}

	req, err := http.NewRequest("GET", httpPinger.Check.URL, nil)
	if err != nil {
		result = Result{StatusNOK, fmt.Errorf("ping failed: %s", err)}
		output = nil
		return
	}

	if httpPinger.Check.BasicAuth != nil {
		req.SetBasicAuth(
			httpPinger.Check.BasicAuth.Username,
			httpPinger.Check.BasicAuth.Password)
	}

	response, err := client.Do(req)
	if err != nil {
		result = Result{StatusNOK, fmt.Errorf("ping failed: %s", err)}
		output = nil
		return
	}
	defer response.Body.Close()

	expectedCode := httpPinger.Check.Expect.StatusCode
	if expectedCode != response.StatusCode {
		result = Result{StatusNOK, fmt.Errorf("expected status code (%d) differs from actual (%d)", expectedCode, response.StatusCode)}
		output = nil
		return
	}

	body, err := ioutil.ReadAll(response.Body)
	result = Result{StatusOK, nil}
	output = bytes.NewBuffer(body)
	return
}
