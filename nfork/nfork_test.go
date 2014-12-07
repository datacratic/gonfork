// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/goset"

	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"
)

func AllocatePort() (listen, url string) {
	listen = fmt.Sprintf(":%d", rand.Intn(1000)+23456)
	url = "http://localhost" + listen
	return
}

type TestService struct {
	T    *testing.T
	Name string

	Code  int
	Sleep time.Duration

	initialize sync.Once

	requestC chan string
}

func (service *TestService) Init() {
	service.initialize.Do(service.init)
}

func (service *TestService) init() {
	service.requestC = make(chan string, 100)

	if service.Code == 0 {
		service.Code = http.StatusOK
	}
}

func (service *TestService) ServeHTTP(writer http.ResponseWriter, httpReq *http.Request) {
	service.Init()

	body, err := ioutil.ReadAll(httpReq.Body)
	if err != nil {
		service.T.Errorf("FAIL(service.%s): unable to read body -> %s", service.Name, err.Error())
	}

	if _, ok := httpReq.Header["X-Nfork"]; !ok {
		service.T.Errorf("FAIL(service.%s): missing x-nfork header -> %v", service.Name, httpReq.Header)
	}

	if _, ok := httpReq.Header["X-Test"]; !ok {
		service.T.Errorf("FAIL(service.%s): missing x-test header -> %v", service.Name, httpReq.Header)
	}

	service.requestC <- fmt.Sprintf("{%s %s %s}", httpReq.Method, httpReq.URL.Path, string(body))

	if service.Sleep > 0 {
		time.Sleep(service.Sleep)
	}

	writer.Header().Set("X-Test", "true")
	writer.WriteHeader(service.Code)
	writer.Write([]byte(service.Name))
}

func (service *TestService) Expect(requests ...string) {
	service.Init()

	a := set.NewString()
	b := set.NewString(requests...)

	done := false
	timeoutC := time.After(50 * time.Millisecond)

	for !done {
		select {

		case req := <-service.requestC:
			a.Put(req)

		case <-timeoutC:
			done = true

		}
	}

	if diff := a.Difference(b); len(diff) > 0 {
		service.T.Errorf("FAIL(service.%s): extra values -> %s", service.Name, diff)
	}

	if diff := b.Difference(a); len(diff) > 0 {
		service.T.Errorf("FAIL(service.%s): missing values -> %s", service.Name, diff)
	}
}

func ExpectInbound(t *testing.T, URL, method, path, req string, expCode int, expResp string) {
	resp, body, err := SendTo(URL, method, path, req)
	if err != nil {
		t.Errorf("FAIL(send.%s): post failed -> %s", req, err.Error())
		return
	}

	if resp.StatusCode != expCode {
		t.Errorf("FAIL(send.%s): unexpected code -> %d != %d", req, resp.StatusCode, expCode)
	}

	if body != expResp {
		t.Errorf("FAIL(send.%s): unexpected body -> %s != %s", req, body, expResp)
	}

	if val := resp.Header.Get("X-Test"); val != "true" {
		t.Errorf("FAIL(send.%s): missing or invalid x-test header -> '%s' != 'true'", req, val)
	}
}

func ExpectInboundTimeout(t *testing.T, URL, method, path, req string) {
	resp, _, err := SendTo(URL, method, path, req)
	if err != nil {
		t.Errorf("FAIL(send.%s): post failed -> %s", req, err.Error())
		return
	}

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("FAIL(send.%s): unexpected code -> %d != %d", req, resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func SendTo(URL, method, path, body string) (*http.Response, string, error) {
	path = URL + "/" + path

	req, err := http.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("content-type", "text/plain")
	req.Header.Set("X-Test", "true")

	resp, err := http.DefaultClient.Do(req)

	var respBody []byte
	if err == nil {
		respBody, err = ioutil.ReadAll(resp.Body)
	}

	return resp, string(respBody), err
}
