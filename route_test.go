// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"fmt"
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

	s2 := &TestService{T: t, Name: "s2", Sleep: 10 * time.Millisecond}
	server2 := httptest.NewServer(s2)
	defer server2.Close()

	route := &Route{Inbound: "route", Timeout: 5 * time.Millisecond}
	serverRoute := httptest.NewServer(route)
	defer serverRoute.Close()

	route.TestAdd(t, "s0", server0.URL+"/s0")
	route.TestActivate(t, "s0")
	route.TestValidate(t)

	ExpectRoute(t, serverRoute, "GET", "", "r00", http.StatusOK, "s0")
	ExpectRoute(t, serverRoute, "PUT", "", "r01", http.StatusOK, "s0")
	ExpectRoute(t, serverRoute, "POST", "", "r02", http.StatusOK, "s0")
	s0.Expect("{GET /s0 r00}", "{PUT /s0 r01}", "{POST /s0 r02}")

	route.TestAdd(t, "s1", server1.URL+"/s1/a")
	route.TestValidate(t)

	ExpectRoute(t, serverRoute, "GET", "", "r10", http.StatusOK, "s0")
	ExpectRoute(t, serverRoute, "PUT", "", "r11", http.StatusOK, "s0")
	ExpectRoute(t, serverRoute, "POST", "", "r12", http.StatusOK, "s0")
	s0.Expect("{GET /s0 r10}", "{PUT /s0 r11}", "{POST /s0 r12}")
	s1.Expect("{GET /s1/a r10}", "{PUT /s1/a r11}", "{POST /s1/a r12}")

	route.TestActivate(t, "s1")
	route.TestValidate(t)

	ExpectRoute(t, serverRoute, "GET", "", "r20", http.StatusCreated, "s1")
	ExpectRoute(t, serverRoute, "PUT", "", "r21", http.StatusCreated, "s1")
	ExpectRoute(t, serverRoute, "POST", "", "r22", http.StatusCreated, "s1")
	s0.Expect("{GET /s0 r20}", "{PUT /s0 r21}", "{POST /s0 r22}")
	s1.Expect("{GET /s1/a r20}", "{PUT /s1/a r21}", "{POST /s1/a r22}")

	route.TestAdd(t, "s2", server2.URL+"/")
	route.TestValidate(t)

	ExpectRoute(t, serverRoute, "GET", "", "r30", http.StatusCreated, "s1")
	ExpectRoute(t, serverRoute, "PUT", "", "r31", http.StatusCreated, "s1")
	s0.Expect("{GET /s0 r30}", "{PUT /s0 r31}")
	s1.Expect("{GET /s1/a r30}", "{PUT /s1/a r31}")
	s2.Expect("{GET / r30}", "{PUT / r31}")

	route.TestActivate(t, "s2")
	route.TestValidate(t)

	ExpectRouteTimeout(t, serverRoute, "GET", "", "r40")
	ExpectRouteTimeout(t, serverRoute, "PUT", "", "r41")
	s0.Expect("{GET /s0 r40}", "{PUT /s0 r41}")
	s1.Expect("{GET /s1/a r40}", "{PUT /s1/a r41}")
	s2.Expect("{GET / r40}", "{PUT / r41}")

	route.TestActivate(t, "s1")
	route.TestRemove(t, "s2")
	route.TestValidate(t)

	ExpectRoute(t, serverRoute, "GET", "", "r50", http.StatusCreated, "s1")
	ExpectRoute(t, serverRoute, "PUT", "", "r51", http.StatusCreated, "s1")
	s0.Expect("{GET /s0 r50}", "{PUT /s0 r51}")
	s1.Expect("{GET /s1/a r50}", "{PUT /s1/a r51}")
	s2.Expect()
}

func (route *Route) TestValidate(t *testing.T) {
	if err := route.Validate(); err != nil {
		t.Errorf("FAIL(route.%s): validate() -> %s", route.Inbound, err.Error())
	}
}

func (route *Route) TestAdd(t *testing.T, outbound string, rawURL string) {
	if err := route.Add(outbound, rawURL); err != nil {
		t.Errorf("FAIL(route.%s): add(%s, %s) -> %s", route.Inbound, outbound, rawURL, err.Error())
	}
}

func (route *Route) TestRemove(t *testing.T, outbound string) {
	if err := route.Remove(outbound); err != nil {
		t.Errorf("FAIL(route.%s): remove(%s) -> %s", route.Inbound, outbound, err.Error())
	}
}

func (route *Route) TestActivate(t *testing.T, outbound string) {
	if err := route.Activate(outbound); err != nil {
		t.Errorf("FAIL(route.%s): activate(%s) -> %s", route.Inbound, outbound, err.Error())
	}
}

func ExpectRoute(t *testing.T, server *httptest.Server, method, path, req string, expCode int, expResp string) {
	resp, body, err := SendTo(server, method, path, req)
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

func ExpectRouteTimeout(t *testing.T, server *httptest.Server, method, path, req string) {
	resp, _, err := SendTo(server, method, path, req)
	if err != nil {
		t.Errorf("FAIL(send.%s): post failed -> %s", req, err.Error())
		return
	}

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("FAIL(send.%s): unexpected code -> %d != %d", req, resp.StatusCode, http.StatusServiceUnavailable)
	}
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

	route := &Route{Inbound: "bob"}
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
		route.Add(name, servers[len(servers)-1].URL)
		route.Activate(name)
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
