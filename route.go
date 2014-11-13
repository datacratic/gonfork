// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"
)

const DefaultRouteTimeout = 100 * time.Millisecond

type Route struct {
	Inbound  string
	Outbound map[string]*url.URL

	Active  string
	Timeout time.Duration

	Client *http.Client
}

func (route *Route) Copy() *Route {
	newRoute := &Route{
		Inbound:  route.Inbound,
		Outbound: make(map[string]*url.URL),

		Active:  route.Active,
		Timeout: route.Timeout,

		Client: route.Client,
	}

	for outbound, URL := range route.Outbound {
		newRoute.Outbound[outbound] = URL
	}

	return newRoute
}

func (route *Route) Validate() error {
	if len(route.Inbound) == 0 {
		return fmt.Errorf("missing name for inbound")
	}

	if len(route.Outbound) == 0 {
		return fmt.Errorf("no outbound in '%s'", route.Inbound)
	}

	if len(route.Active) == 0 {
		return fmt.Errorf("no active outbound in '%s'", route.Inbound)
	}

	if _, ok := route.Outbound[route.Active]; !ok {
		return fmt.Errorf("active outbound '%s' doesn't exist", route.Active)
	}

	if route.Timeout == 0 {
		route.Timeout = DefaultRouteTimeout
	}

	if route.Client == nil {
		route.Client = new(http.Client)
	}

	if route.Client.Timeout == 0 {
		route.Client.Timeout = route.Timeout
	}

	return nil
}

func (route *Route) Add(outbound string, rawURL string) error {
	if route.Outbound == nil {
		route.Outbound = make(map[string]*url.URL)
	}

	if _, ok := route.Outbound[outbound]; ok {
		return fmt.Errorf("outbound '%s' already exists in '%s'", outbound, route.Inbound)
	}

	URL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	route.Outbound[outbound] = URL
	return nil
}

func (route *Route) Remove(outbound string) error {
	if route.Outbound == nil {
		return fmt.Errorf("outbound '%s' doesn't exists in '%s'", outbound, route.Inbound)
	}

	if _, ok := route.Outbound[outbound]; !ok {
		return fmt.Errorf("outbound '%s' doesn't exists in '%s'", outbound, route.Inbound)
	}

	if outbound == route.Active {
		return fmt.Errorf("can't remove active outbound '%s'", outbound)
	}

	delete(route.Outbound, outbound)
	return nil
}

func (route *Route) Activate(outbound string) error {
	if route.Outbound == nil {
		return fmt.Errorf("outbound '%s' doesn't exists in '%s'", outbound, route.Inbound)
	}

	if _, ok := route.Outbound[outbound]; !ok {
		return fmt.Errorf("outbound '%s' doesn't exists in '%s'", outbound, route.Inbound)
	}

	route.Active = outbound
	return nil
}

func (route *Route) ServeHTTP(writer http.ResponseWriter, httpReq *http.Request) {
	body, err := ioutil.ReadAll(httpReq.Body)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	httpReq.Header.Set("X-Nfork", "true")

	var activeURL *url.URL

	for outbound, URL := range route.Outbound {
		if outbound != route.Active {
			go route.forward(outbound, httpReq, URL, body)
		} else {
			activeURL = URL
		}
	}

	if activeURL == nil {
		log.Panicf("no active outbound '%s'", route.Active)
	}

	respHead, respBody, err := route.forward(route.Active, httpReq, activeURL, body)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusServiceUnavailable)
		return
	}

	writerHeader := writer.Header()
	for key, val := range respHead.Header {
		writerHeader[key] = val
	}

	writer.WriteHeader(respHead.StatusCode)
	writer.Write(respBody)
}

func (route *Route) forward(
	outbound string, oldReq *http.Request, URL *url.URL, body []byte) (*http.Response, []byte, error) {

	newReq := new(http.Request)
	*newReq = *oldReq

	newReq.URL = URL
	newReq.Host = URL.Host
	newReq.RequestURI = ""
	newReq.Body = ioutil.NopCloser(bytes.NewReader(body))

	resp, err := route.Client.Do(newReq)
	if err != nil {
		log.Printf("failed to send request to outbound '%s': %s", outbound, err)
		return nil, nil, err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		log.Printf("failed to read response of outbound '%s': %s", outbound, err)
		return nil, nil, err
	}

	return resp, respBody, nil
}

func (route *Route) UnmarshalJSON(body []byte) (err error) {
	var routeJSON struct {
		Inbound  string            `json:"in"`
		Outbound map[string]string `json:"out"`

		Active  string        `json:"active"`
		Timeout time.Duration `json:"timeout,omitempty"`
	}

	if err = json.Unmarshal(body, &routeJSON); err != nil {
		return
	}

	route.Inbound = routeJSON.Inbound
	route.Outbound = make(map[string]*url.URL)

	for outbound, URL := range routeJSON.Outbound {
		if route.Outbound[outbound], err = url.Parse(URL); err != nil {
			return err
		}
	}

	route.Active = routeJSON.Active
	route.Timeout = routeJSON.Timeout

	return
}

func (route *Route) MarshalJSON() ([]byte, error) {
	var routeJSON struct {
		Inbound  string            `json:"in"`
		Outbound map[string]string `json:"out"`

		Active  string        `json:"active"`
		Timeout time.Duration `json:"timeout,omitempty"`
	}

	routeJSON.Inbound = route.Inbound
	routeJSON.Outbound = make(map[string]string)

	for outbound, URL := range route.Outbound {
		routeJSON.Outbound[outbound] = URL.String()
	}

	routeJSON.Active = route.Active
	routeJSON.Timeout = route.Timeout

	return json.Marshal(&routeJSON)
}
