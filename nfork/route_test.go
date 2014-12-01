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

func TestRoute_Forward(t *testing.T) {

	s0 := &TestService{T: t, Name: "s0"}
	server0 := httptest.NewServer(s0)
	defer server0.Close()

	s1 := &TestService{T: t, Name: "s1", Code: http.StatusCreated}
	server1 := httptest.NewServer(s1)
	defer server1.Close()

	s2 := &TestService{T: t, Name: "s2", Sleep: 100 * time.Millisecond}
	server2 := httptest.NewServer(s2)
	defer server2.Close()

	route := &Route{
		Name:    "route",
		Timeout: 50 * time.Millisecond,
		Outbound: map[string]string{
			"s0": server0.URL,
			"s1": server1.URL,
			"s2": server2.URL,
		},
		Active: "s1",
	}
	serverRoute := httptest.NewServer(route)
	defer serverRoute.Close()

	ExpectRoute(t, serverRoute, "GET", "a", "r00", http.StatusCreated, "s1")
	ExpectRoute(t, serverRoute, "PUT", "a/b", "r01", http.StatusCreated, "s1")
	ExpectRoute(t, serverRoute, "POST", "a/b/c", "r02", http.StatusCreated, "s1")
	s0.Expect("{GET /a r00}", "{PUT /a/b r01}", "{POST /a/b/c r02}")
	s1.Expect("{GET /a r00}", "{PUT /a/b r01}", "{POST /a/b/c r02}")
	s2.Expect("{GET /a r00}", "{PUT /a/b r01}", "{POST /a/b/c r02}")
}

func BenchmarkRoute_1(b *testing.B) {
	RouteBench(b, 1)
}

func BenchmarkRoute_2(b *testing.B) {
	RouteBench(b, 2)
}

func BenchmarkRoute_4(b *testing.B) {
	RouteBench(b, 4)
}

func BenchmarkRoute_8(b *testing.B) {
	RouteBench(b, 8)
}

func RouteBench(b *testing.B, routes int) {

	klog.SetPrinter(klog.NilPrinter)

	route := &Route{Name: "bob", IdleConnections: 32, Outbound: make(map[string]string)}
	routeServer := httptest.NewServer(route)
	defer routeServer.Close()

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

	for i := 0; i < routes; i++ {
		name := fmt.Sprintf("s%d", i)
		servers = append(servers, httptest.NewServer(handler))
		route.Outbound[name] = servers[len(servers)-1].URL
		route.Active = name
	}

	route.Validate()

	if _, err := http.Get(routeServer.URL); err != nil {
		b.Fatalf("FAIL: %s", err.Error())
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		http.Get(routeServer.URL)
	}

	// Avoid timing the calls to server.Close()
	b.StopTimer()
}
