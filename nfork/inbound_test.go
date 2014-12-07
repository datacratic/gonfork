// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"fmt"
	"github.com/datacratic/goklog/klog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInbound(t *testing.T) {

	s0 := &TestService{T: t, Name: "s0"}
	server0 := httptest.NewServer(s0)
	defer server0.Close()

	s1 := &TestService{T: t, Name: "s1", Code: http.StatusCreated}
	server1 := httptest.NewServer(s1)
	defer server1.Close()

	s2 := &TestService{T: t, Name: "s2", Sleep: 100 * time.Millisecond}
	server2 := httptest.NewServer(s2)
	defer server2.Close()

	inbound := &Inbound{
		Name:    "bob",
		Timeout: 50 * time.Millisecond,
		Outbound: map[string]string{
			"s0": server0.URL,
			"s1": server1.URL,
			"s2": server2.URL,
		},
		Active: "s1",
	}
	server := httptest.NewServer(inbound)
	defer server.Close()

	ExpectInbound(t, server.URL, "GET", "a", "r00", http.StatusCreated, "s1")
	ExpectInbound(t, server.URL, "PUT", "a/b", "r01", http.StatusCreated, "s1")
	ExpectInbound(t, server.URL, "POST", "a/b/c", "r02", http.StatusCreated, "s1")
	s0.Expect("{GET /a r00}", "{PUT /a/b r01}", "{POST /a/b/c r02}")
	s1.Expect("{GET /a r00}", "{PUT /a/b r01}", "{POST /a/b/c r02}")
	s2.Expect("{GET /a r00}", "{PUT /a/b r01}", "{POST /a/b/c r02}")
}

func BenchmarkInbound_1(b *testing.B) {
	InboundBench(b, 1)
}

func BenchmarkInbound_2(b *testing.B) {
	InboundBench(b, 2)
}

func BenchmarkInbound_4(b *testing.B) {
	InboundBench(b, 4)
}

func BenchmarkInbound_8(b *testing.B) {
	InboundBench(b, 8)
}

func InboundBench(b *testing.B, inbounds int) {

	klog.SetPrinter(klog.NilPrinter)

	inbound := &Inbound{Name: "bob", IdleConnections: 32, Outbound: make(map[string]string)}
	server := httptest.NewServer(inbound)
	defer server.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		w.Write(nil)
	})

	var servers []*httptest.Server
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	for i := 0; i < inbounds; i++ {
		name := fmt.Sprintf("s%d", i)
		servers = append(servers, httptest.NewServer(handler))
		inbound.Outbound[name] = servers[len(servers)-1].URL
		inbound.Active = name
	}

	inbound.Validate()

	if _, err := http.Get(server.URL); err != nil {
		b.Fatalf("FAIL: %s", err.Error())
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		http.Get(server.URL)
	}

	// Avoid timing the calls to server.Close()
	b.StopTimer()
}
