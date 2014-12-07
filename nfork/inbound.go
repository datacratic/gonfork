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

// DefaultInboundTimeout is used if no timeout is set for an inbound.
const DefaultInboundTimeout = 1 * time.Second

// Inbound duplicates HTTP requests to a set of outbounds and only forwards the
// HTTP response of an active outbound. All other responses are dropped.
//
// Stats are also gathered for each outbound and each outbounds can also be
// configured to timeout.
type Inbound struct {
	// Name is the name associated with this inbound.
	Name string

	// Listen defines which interface and port this inbound should listen on.
	Listen string

	// Outbound maps a set of outbound names to the address where HTTP requests
	// should be redirected to. Addresses should be of the for
	// <scheme>://<host>:<port>.
	Outbound map[string]string

	// Active defines the name of the outbound whose response will be forwarded
	// back upstream. All other outbound responses are dropped.
	Active string

	// Timeout defines the timeout allowed for all outbounds. If the timeout
	// expires for the active outbound, TimeoutCode is sent back upstream.
	Timeout time.Duration

	// TimeoutCode is HTTP status code sent back upstream if a timeout occurs on
	// the active outbbound.
	TimeoutCode int

	// Client is the http.Client which will be used to forward HTTP requests to
	// all outbounds.
	Client *http.Client

	// IdleConnections defines the maximum size of the connection pool which
	// will be used by the http client for this inbound. Setting this will
	// overwrite the transport of the Client if it is set.
	IdleConnections int

	initialize sync.Once

	stats map[string]*StatsRecorder
}

// Copy returns a copy of the inbound object.
func (inbound *Inbound) Copy() *Inbound {
	newInbound := &Inbound{
		Name: inbound.Name,

		Listen:   inbound.Listen,
		Active:   inbound.Active,
		Outbound: make(map[string]string),

		Timeout:         inbound.Timeout,
		TimeoutCode:     inbound.TimeoutCode,
		IdleConnections: inbound.IdleConnections,

		Client: inbound.Client,
		stats:  make(map[string]*StatsRecorder),
	}

	for outbound, addr := range inbound.Outbound {
		newInbound.Outbound[outbound] = addr
	}

	for outbound, stats := range inbound.stats {
		newInbound.stats[outbound] = stats
	}

	return newInbound
}

// Validate returns an error if one of the Inbound invariants are not satisfied.
func (inbound *Inbound) Validate() error {
	if len(inbound.Name) == 0 {
		inbound.Name = inbound.Listen
	}

	if len(inbound.Listen) == 0 {
		return fmt.Errorf("missing listen host")
	}

	if len(inbound.Outbound) == 0 {
		return fmt.Errorf("no outbound in '%s'", inbound.Name)
	}

	if len(inbound.Active) == 0 {
		return fmt.Errorf("no active outbound in '%s'", inbound.Name)
	}

	if _, ok := inbound.Outbound[inbound.Active]; !ok {
		return fmt.Errorf("active outbound '%s' doesn't exist in '%s'", inbound.Active, inbound.Name)
	}

	return nil
}

// Init initializes the object. Inbounds are lazily initialized so calling this
// is optional.
func (inbound *Inbound) Init() {
	inbound.initialize.Do(inbound.init)
}

func (inbound *Inbound) init() {
	if inbound.Timeout == 0 {
		inbound.Timeout = DefaultInboundTimeout
	}

	if inbound.TimeoutCode == 0 {
		inbound.TimeoutCode = http.StatusServiceUnavailable
	}

	if inbound.Client == nil {
		inbound.Client = new(http.Client)
	}

	if inbound.IdleConnections > 0 {
		inbound.Client.Transport = httpTransport(inbound.IdleConnections)
	} else {
		inbound.IdleConnections = http.DefaultMaxIdleConnsPerHost
	}

	if inbound.Client.Timeout == 0 {
		inbound.Client.Timeout = inbound.Timeout
	}

	if inbound.stats == nil {
		inbound.stats = make(map[string]*StatsRecorder)
	}

	for outbound := range inbound.Outbound {
		inbound.stats[outbound] = new(StatsRecorder)
	}
}

// ReadStats returns the stats associated with each outbounds.
func (inbound *Inbound) ReadStats() map[string]*Stats {
	stats := make(map[string]*Stats)

	for outbound, recorder := range inbound.stats {
		stats[outbound] = recorder.Read()
	}

	return stats
}

// ReadOutboundStats returns the stats associated with a given outbound.
func (inbound *Inbound) ReadOutboundStats(outbound string) (*Stats, error) {
	if _, ok := inbound.Outbound[outbound]; !ok {
		return nil, fmt.Errorf("unknown outbound '%s' for inbound '%s'", outbound, inbound.Name)
	}

	return inbound.stats[outbound].Read(), nil
}

// AddOutbound adds a new outbound associated with the given address. If the
// outbound already exists, it is overridden.
func (inbound *Inbound) AddOutbound(outbound, addr string) error {
	inbound.Outbound[outbound] = addr
	inbound.stats[outbound] = new(StatsRecorder)
	return nil
}

// RemoveOutbound removes the given outbound. An error is returned if the
// outbound doesn't exist.
func (inbound *Inbound) RemoveOutbound(outbound string) error {
	if _, ok := inbound.Outbound[outbound]; !ok {
		return fmt.Errorf("unknown outbound '%s' for inbound '%s'", outbound, inbound.Name)
	}

	if outbound == inbound.Active {
		return fmt.Errorf("can't remove active outbound '%s' for inbound '%s'", outbound, inbound.Name)
	}

	delete(inbound.Outbound, outbound)
	delete(inbound.stats, outbound)

	return nil
}

// ActivateOutbound activates the given outbound.
func (inbound *Inbound) ActivateOutbound(outbound string) error {
	if _, ok := inbound.Outbound[outbound]; !ok {
		return fmt.Errorf("unknown outbound '%s' for inbound '%s'", outbound, inbound.Name)
	}

	inbound.Active = outbound

	return nil
}

// ServeHTTP forwards the given HTTP request to all the outbounds and forwards
// the response of the active outbound back upstream. All other responses are
// dropped.
func (inbound *Inbound) ServeHTTP(writer http.ResponseWriter, httpReq *http.Request) {
	inbound.Init()

	body, err := ioutil.ReadAll(httpReq.Body)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	httpReq.Header.Set("X-Nfork", "true")

	var activeHost string

	for outbound, host := range inbound.Outbound {
		if outbound != inbound.Active {
			go inbound.forward(outbound, httpReq, host, body)
		} else {
			activeHost = host
		}
	}

	if len(activeHost) == 0 {
		log.Panicf("no active outbound '%s'", inbound.Active)
	}

	respHead, respBody, err := inbound.forward(inbound.Active, httpReq, activeHost, body)
	if err != nil {
		http.Error(writer, err.Error(), inbound.TimeoutCode)
		return
	}

	writerHeader := writer.Header()
	for key, val := range respHead.Header {
		writerHeader[key] = val
	}

	writer.WriteHeader(respHead.StatusCode)
	writer.Write(respBody)
}

func (inbound *Inbound) record(outbound string, event Event) {
	stats, ok := inbound.stats[outbound]
	if !ok {
		log.Panicf("no stats for outbound '%s'", outbound)
	}
	stats.Record(event)
}

func (inbound *Inbound) parseAddr(addr string) (host, scheme string) {
	if i := strings.Index(addr, "://"); i >= 0 {
		return addr[i+3:], addr[:i]
	}
	return addr, "http"
}

func (inbound *Inbound) forward(
	outbound string, oldReq *http.Request, addr string, body []byte) (*http.Response, []byte, error) {

	t0 := time.Now()

	host, scheme := inbound.parseAddr(addr)

	newReq := new(http.Request)
	*newReq = *oldReq

	newReq.URL = new(url.URL)
	*newReq.URL = *oldReq.URL

	newReq.Host = host
	newReq.URL.Host = host
	newReq.URL.Scheme = scheme
	newReq.RequestURI = ""
	newReq.Body = ioutil.NopCloser(bytes.NewReader(body))

	resp, err := inbound.Client.Do(newReq)
	if err != nil {
		return nil, nil, inbound.error("send", outbound, err, t0)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, nil, inbound.error("recv", outbound, err, t0)
	}

	inbound.record(outbound, Event{Response: resp.StatusCode, Latency: time.Since(t0)})
	return resp, respBody, nil
}

func (inbound *Inbound) error(title, outbound string, err error, t0 time.Time) error {

	if urlErr, ok := err.(*url.Error); ok {
		return inbound.error(title, outbound, urlErr.Err, t0)

	} else if netErr, ok := err.(*net.OpError); ok {
		if netErr.Op == "dial" {
			if errno, ok := netErr.Err.(syscall.Errno); ok && errno == syscall.ECONNREFUSED {
				klog.KPrintf(klog.Keyf("%.%s.%s.timeout", inbound.Name, outbound, title), "%T -> %v", err, err)
				inbound.record(outbound, Event{Timeout: true, Latency: time.Since(t0)})
				return err
			}
		}
		return inbound.error(title, outbound, netErr.Err, t0)
	}

	// I hate this but net and net/http provides no useful errors or indicators
	// to that a request ended up in a timeout. Furthermore, most of the errors
	// are either not exported or are just randomly created as string. In other
	// words, this is a crappy interface that needs to be fixed bad.
	switch err.Error() {

	case "use of closed network connection": // net.errClosing
		fallthrough
	case "net/http: request canceled while waiting for connection":
		klog.KPrintf(klog.Keyf("%s.%s.%s.timeout", inbound.Name, outbound, title), "%T -> %v", err, err)
		inbound.record(outbound, Event{Timeout: true, Latency: time.Since(t0)})
		return err
	}

	klog.KPrintf(klog.Keyf("%s.%s.%s.error", inbound.Name, outbound, title), "%T -> %v", err, err)
	inbound.record(outbound, Event{Error: true, Latency: time.Since(t0)})
	return err
}

// UnmarshalJSON defines a custom JSON format for the encoding/json package.
func (inbound *Inbound) UnmarshalJSON(body []byte) (err error) {
	var inboundJSON struct {
		Name string `json:"name"`

		Listen   string            `json:"listen"`
		Outbound map[string]string `json:"out"`
		Active   string            `json:"active"`

		Timeout     string `json:"timeout,omitempty"`
		TimeoutCode int    `json:"timeoutCode,omitempty"`

		IdleConnections int `json:"idleConn"`
	}

	if err = json.Unmarshal(body, &inboundJSON); err != nil {
		return
	}

	inbound.Name = inboundJSON.Name

	inbound.Listen = inboundJSON.Listen
	inbound.Outbound = inboundJSON.Outbound
	inbound.Active = inboundJSON.Active

	if inbound.Timeout, err = time.ParseDuration(inboundJSON.Timeout); err != nil {
		return
	}
	inbound.TimeoutCode = inboundJSON.TimeoutCode

	inbound.IdleConnections = inboundJSON.IdleConnections

	return
}

// MarshalJSON defines a custom JSON format for the encoding/json package.
func (inbound *Inbound) MarshalJSON() ([]byte, error) {
	var inboundJSON struct {
		Name string `json:"name"`

		Listen   string            `json:"listen"`
		Active   string            `json:"active"`
		Outbound map[string]string `json:"out"`

		Timeout     string `json:"timeout,omitempty"`
		TimeoutCode int    `json:"timeoutCode,omitempty"`

		IdleConnections int `json:"idleConn"`
	}

	inboundJSON.Name = inbound.Name

	inboundJSON.Listen = inbound.Listen
	inboundJSON.Outbound = inbound.Outbound
	inboundJSON.Active = inbound.Active

	inboundJSON.Timeout = inbound.Timeout.String()
	inboundJSON.TimeoutCode = inbound.TimeoutCode

	inboundJSON.IdleConnections = inbound.IdleConnections

	return json.Marshal(&inboundJSON)
}
