// Copyright 2017 Shunsuke Michii. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitor

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/harukasan/gostarter"
)

type HTTPMonitorConfig struct {
	Method             string
	URL                *url.URL
	Header             http.Header
	Body               []byte
	ExpectedStatusCode int
	ExpectedBody       []byte
	Interval           time.Duration
	Retry              int
}

func NewHTTPMonitor(config *HTTPMonitorConfig) *HTTPMonitor {
	if config.Method == "" {
		config.Method = "GET"
	}
	if config.Header == nil {
		config.Header = make(http.Header)
	}
	if config.Interval <= 0 {
		config.Interval = time.Second
	}
	if config.URL.Scheme == "" {
		config.URL.Scheme = "http"
	}

	return &HTTPMonitor{
		config: config,
	}
}

type checkFailed struct {
	err error
}

func (c *checkFailed) Error() string {
	return c.err.Error()
}

type HTTPMonitor struct {
	config *HTTPMonitorConfig
}

func (m *HTTPMonitor) Run(ctx context.Context, nw, addr string, trigger func(error) error) error {
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(proto, addr string) (conn net.Conn, err error) {
				return net.Dial(nw, addr)
			},
		},
	}
	if m.config.URL.Host == "" {
		m.config.URL.Host = addr
	}

	count := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.config.Interval):
		}

		ce := m.check(client)
		if checkfailed, ok := ce.(*checkFailed); ok {
			count++
			if count > m.config.Retry {
				return trigger(checkfailed.err)
			}
		} else {
			count = 0
		}
	}
}

func (m *HTTPMonitor) check(client *http.Client) error {
	debug := os.Getenv("GOSTARTER_MONITOR_DEBUG") != ""

	req := &http.Request{
		Method:     m.config.Method,
		URL:        m.config.URL,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     m.config.Header,
		Host:       m.config.URL.Host,
	}
	if m.config.Body != nil {
		req.Body = ioutil.NopCloser(bytes.NewReader(m.config.Body))
		req.ContentLength = int64(len(m.config.Body))
	}

	res, err := client.Do(req)
	if err != nil {
		return &checkFailed{err}
	}
	defer res.Body.Close()

	if debug {
		log.Printf("monitor status: %d %s", res.StatusCode, res.Status)
	}

	if expected := m.config.ExpectedStatusCode; expected > 0 {
		if res.StatusCode != expected {
			return &checkFailed{fmt.Errorf("wrong status code: %d, but want: %d", res.StatusCode, expected)}
		}
	}

	if expected := m.config.ExpectedBody; expected != nil {
		gotBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		if !bytes.Equal(gotBody, expected) {
			return &checkFailed{fmt.Errorf("wrong body: \"%v\"", string(gotBody))}
		}
	}

	return nil
}

var _ gostarter.Monitor = &HTTPMonitor{}
