// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/gorest/rest"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRouter_Forward(t *testing.T) {

	s0 := &TestService{T: t, Name: "s0"}
	server0 := httptest.NewServer(s0)
	defer server0.Close()

	s1 := &TestService{T: t, Name: "s1", Code: http.StatusCreated}
	server1 := httptest.NewServer(s1)
	defer server1.Close()

	s2 := &TestService{T: t, Name: "s2", Sleep: 10 * time.Millisecond}
	server2 := httptest.NewServer(s2)
	defer server2.Close()

	router := new(Router)

	serverControl := new(rest.TestEndpoint)
	serverControl.AddRoutable(router)
	serverControl.ListenAndServe()

	serverRouter := httptest.NewServer(router)
	defer serverRouter.Close()

	PostInbound(t, serverControl, "i0", map[string]string{"s0": server0.URL + "/s0", "s1": server1.URL + "/s1/a"}, "s0")

	ExpectRoute(t, serverRouter, "GET", "i0", "r00", http.StatusOK, "s0")
	ExpectRoute(t, serverRouter, "PUT", "i0", "r01", http.StatusOK, "s0")
	s0.Expect("{GET /s0 r00}", "{PUT /s0 r01}")
	s1.Expect("{GET /s1/a r00}", "{PUT /s1/a r01}")

	PostInbound(t, serverControl, "i1", map[string]string{"s1": server1.URL + "/s1/b", "s2": server2.URL + "/s2"}, "s1")

	ExpectRoute(t, serverRouter, "GET", "i0", "r10", http.StatusOK, "s0")
	ExpectRoute(t, serverRouter, "PUT", "i1", "r11", http.StatusCreated, "s1")
	s0.Expect("{GET /s0 r10}")
	s1.Expect("{GET /s1/a r10}", "{PUT /s1/b r11}")
	s2.Expect("{PUT /s2 r11}")

	PostActivate(t, serverControl, "i0", "s1")
	PostOutbound(t, serverControl, "i1", "s0", server0.URL+"/s0/c")
	DeleteOutbound(t, serverControl, "i1", "s2")

	ExpectRoute(t, serverRouter, "GET", "i0", "r20", http.StatusCreated, "s1")
	ExpectRoute(t, serverRouter, "PUT", "i1", "r21", http.StatusCreated, "s1")
	s0.Expect("{GET /s0 r20}", "{PUT /s0/c r21}")
	s1.Expect("{GET /s1/a r20}", "{PUT /s1/b r21}")
	s2.Expect()
}

func PostInbound(t *testing.T, server *rest.TestEndpoint, inbound string, outbound map[string]string, active string) {
	route := &Route{
		Inbound: inbound,
		Active:  active,
		Timeout: 10 * time.Millisecond,
	}

	for name, url := range outbound {
		route.Add(name, url)
	}

	resp := rest.NewRequest(server.URL(), "POST").
		SetPath("/v1/routes").
		SetBody(route).
		Send()

	if err := resp.GetBody(nil); err != nil {
		t.Errorf("FAIL(post.inbound): failed to post {%s, %v} -> %s", inbound, outbound, err.Error())
	}
}

func PostOutbound(t *testing.T, server *rest.TestEndpoint, inbound, outbound, host string) {

	resp := rest.NewRequest(server.URL(), "PUT").
		SetPath("/v1/routes/%s/%s", inbound, outbound).
		SetBody(host).
		Send()

	if err := resp.GetBody(nil); err != nil {
		t.Errorf("FAIL(post.outbound): failed to post {%s, %s} -> %s", inbound, outbound, err.Error())
	}
}

func DeleteOutbound(t *testing.T, server *rest.TestEndpoint, inbound, outbound string) {

	resp := rest.NewRequest(server.URL(), "DELETE").
		SetPath("/v1/routes/%s/%s", inbound, outbound).
		Send()

	if err := resp.GetBody(nil); err != nil {
		t.Errorf("FAIL(delete.outbound): failed to delete {%s, %s} -> %s", inbound, outbound, err.Error())
	}
}

func PostActivate(t *testing.T, server *rest.TestEndpoint, inbound, outbound string) {

	resp := rest.NewRequest(server.URL(), "PUT").
		SetPath("/v1/routes/%s/%s/activate", inbound, outbound).
		Send()

	if err := resp.GetBody(nil); err != nil {
		t.Errorf("FAIL(post.activate): failed to post {%s, %s} -> %s", inbound, outbound, err.Error())
	}
}

func BenchmarkRouter(b *testing.B) {

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		w.Write(nil)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	route := &Route{Inbound: "i0"}
	route.Add("s0", server.URL)
	route.Activate("s0")
	route.Validate()

	router := &Router{Routes: []*Route{route}}
	router.Init()
	serverRouter := httptest.NewServer(router)
	defer serverRouter.Close()

	if _, err := http.Get(serverRouter.URL + "/i0"); err != nil {
		b.Fatalf("FAIL: %s", err.Error())
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		http.Get(serverRouter.URL + "/i0")
	}

	b.StopTimer()
}
