// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/goklog/klog"

	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

const DefaultRouteTimeout = 100 * time.Millisecond

type Route struct {
	Name string

	Listen   string
	Active   string
	Outbound map[string]string

	Stats map[string]*StatsRecorder

	Timeout         time.Duration
	TimeoutCode     int
	IdleConnections int

	Client *http.Client

	initialize sync.Once
}

func (route *Route) Validate() error {
	if len(route.Name) == 0 {
		route.Name = route.Listen
	}

	if len(route.Listen) == 0 {
		return fmt.Errorf("missing listen host")
	}

	if len(route.Outbound) == 0 {
		return fmt.Errorf("no outbound in '%s'", route.Name)
	}

	if len(route.Active) == 0 {
		return fmt.Errorf("no active outbound in '%s'", route.Name)
	}

	if _, ok := route.Outbound[route.Active]; !ok {
		return fmt.Errorf("active outbound '%s' doesn't exist in '%s'", route.Active, route.Name)
	}

	return nil
}

func (route *Route) Init() {
	route.initialize.Do(route.init)
}

func (route *Route) init() {
	if route.Timeout == 0 {
		route.Timeout = DefaultRouteTimeout
	}

	if route.TimeoutCode == 0 {
		route.TimeoutCode = http.StatusServiceUnavailable
	}

	if route.Client == nil {
		route.Client = new(http.Client)
	}

	if route.IdleConnections > 0 {
		// Copied and tweaked from http.DefaultTransport.
		route.Client.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
			MaxIdleConnsPerHost: route.IdleConnections,
		}
	} else {
		route.IdleConnections = http.DefaultMaxIdleConnsPerHost
	}

	if route.Client.Timeout == 0 {
		route.Client.Timeout = route.Timeout
	}

	if route.Stats == nil {
		route.Stats = make(map[string]*StatsRecorder)
	}

	for outbound := range route.Outbound {
		route.Stats[outbound] = new(StatsRecorder)
	}
}

func (route *Route) ReadStats() map[string]*Stats {
	stats := make(map[string]*Stats)

	if route.Stats != nil {
		for outbound, recorder := range route.Stats {
			stats[outbound] = recorder.Read()
		}
	}

	return stats
}

func (route *Route) ServeHTTP(writer http.ResponseWriter, httpReq *http.Request) {
	route.Init()

	body, err := ioutil.ReadAll(httpReq.Body)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	httpReq.Header.Set("X-Nfork", "true")

	var activeHost string

	for outbound, host := range route.Outbound {
		if outbound != route.Active {
			go route.forward(outbound, httpReq, host, body)
		} else {
			activeHost = host
		}
	}

	if len(activeHost) == 0 {
		log.Panicf("no active outbound '%s'", route.Active)
	}

	respHead, respBody, err := route.forward(route.Active, httpReq, activeHost, body)
	if err != nil {
		http.Error(writer, err.Error(), route.TimeoutCode)
		return
	}

	writerHeader := writer.Header()
	for key, val := range respHead.Header {
		writerHeader[key] = val
	}

	writer.WriteHeader(respHead.StatusCode)
	writer.Write(respBody)
}

func (route *Route) record(outbound string, event Event) {
	stats, ok := route.Stats[outbound]
	if !ok {
		log.Panicf("no stats for outbound '%s'", outbound)
	}
	stats.Record(event)
}

func (route *Route) parseAddr(addr string) (host, scheme string) {
	if i := strings.Index(addr, "://"); i >= 0 {
		return addr[i+3:], addr[:i]
	}
	return addr, "http"
}

func (route *Route) forward(
	outbound string, oldReq *http.Request, addr string, body []byte) (*http.Response, []byte, error) {

	t0 := time.Now()

	host, scheme := route.parseAddr(addr)

	newReq := new(http.Request)
	*newReq = *oldReq

	newReq.URL = new(url.URL)
	*newReq.URL = *oldReq.URL

	newReq.Host = host
	newReq.URL.Host = host
	newReq.URL.Scheme = scheme
	newReq.RequestURI = ""
	newReq.Body = ioutil.NopCloser(bytes.NewReader(body))

	resp, err := route.Client.Do(newReq)
	if err != nil {
		return nil, nil, route.error("send", outbound, err, t0)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, nil, route.error("recv", outbound, err, t0)
	}

	route.record(outbound, Event{Response: resp.StatusCode, Latency: time.Since(t0)})
	return resp, respBody, nil
}

func (route *Route) error(title, outbound string, err error, t0 time.Time) error {

	if urlErr, ok := err.(*url.Error); ok {
		return route.error(title, outbound, urlErr.Err, t0)

	} else if netErr, ok := err.(*net.OpError); ok {
		if netErr.Op == "dial" {
			if errno, ok := netErr.Err.(syscall.Errno); ok && errno == syscall.ECONNREFUSED {
				klog.KPrintf(klog.Keyf("%s.%s.timeout", outbound, title), "%T -> %v", err, err)
				route.record(outbound, Event{Timeout: true, Latency: time.Since(t0)})
				return err
			}
		}
		return route.error(title, outbound, netErr.Err, t0)
	}

	// I hate this but net and net/http provides no useful errors or indicators
	// to that a request ended up in a timeout. Furthermore, most of the errors
	// are either not exported or are just randomly created as string. In other
	// words, this is a crappy interface that needs to be fixed bad.
	switch err.Error() {

	case "use of closed network connection": // net.errClosing
		fallthrough
	case "net/http: request canceled while waiting for connection":
		klog.KPrintf(klog.Keyf("%s.%s.timeout", outbound, title), "%T -> %v", err, err)
		route.record(outbound, Event{Timeout: true, Latency: time.Since(t0)})
		return err
	}

	klog.KPrintf(klog.Keyf("%s.%s.error", outbound, title), "%T -> %v", err, err)
	route.record(outbound, Event{Error: true, Latency: time.Since(t0)})
	return err
}

func (route *Route) UnmarshalJSON(body []byte) (err error) {
	var routeJSON struct {
		Name string `json:"name"`

		Listen   string            `json:"listen"`
		Outbound map[string]string `json:"out"`
		Active   string            `json:"active"`

		Timeout     string `json:"timeout,omitempty"`
		TimeoutCode int    `json:"timeoutCode,omitempty"`

		IdleConnections int `json:"idleConn"`
	}

	if err = json.Unmarshal(body, &routeJSON); err != nil {
		return
	}

	route.Name = routeJSON.Name

	route.Listen = routeJSON.Listen
	route.Outbound = routeJSON.Outbound
	route.Active = routeJSON.Active

	if route.Timeout, err = time.ParseDuration(routeJSON.Timeout); err != nil {
		return
	}
	route.TimeoutCode = routeJSON.TimeoutCode

	route.IdleConnections = routeJSON.IdleConnections

	return
}

func (route *Route) MarshalJSON() ([]byte, error) {
	var routeJSON struct {
		Name string `json:"name"`

		Listen   string            `json:"listen"`
		Active   string            `json:"active"`
		Outbound map[string]string `json:"out"`

		Timeout     string `json:"timeout,omitempty"`
		TimeoutCode int    `json:"timeoutCode,omitempty"`

		IdleConnections int `json:"idleConn"`
	}

	routeJSON.Name = route.Name

	routeJSON.Listen = route.Listen
	routeJSON.Outbound = route.Outbound
	routeJSON.Active = route.Active

	routeJSON.Timeout = route.Timeout.String()
	routeJSON.TimeoutCode = route.TimeoutCode

	routeJSON.IdleConnections = route.IdleConnections

	return json.Marshal(&routeJSON)
}
