// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInboundServer(t *testing.T) {

	s0 := &TestService{T: t, Name: "s0"}
	server0 := httptest.NewServer(s0)
	defer server0.Close()

	s1 := &TestService{T: t, Name: "s1", Code: http.StatusCreated}
	server1 := httptest.NewServer(s1)
	defer server1.Close()

	s2 := &TestService{T: t, Name: "s2", Sleep: 100 * time.Millisecond}
	server2 := httptest.NewServer(s2)
	defer server2.Close()

	listen, URL := AllocatePort()

	inbound := &Inbound{
		Name:     "bob",
		Listen:   listen,
		Timeout:  50 * time.Millisecond,
		Outbound: map[string]string{"s0": server0.URL},
		Active:   "s0",
	}
	server, err := NewInboundServer(inbound)
	if err != nil {
		t.Fatalf("unable to start inbound server: %s", err)
	}
	defer server.Close()

	ExpectInbound(t, URL, "GET", "a", "r0", http.StatusOK, "s0")
	s0.Expect("{GET /a r0}")
	s1.Expect()
	s2.Expect()

	ExpectAddOut(t, server, "s1", server1)
	ExpectInbound(t, URL, "GET", "a", "r1", http.StatusOK, "s0")
	s0.Expect("{GET /a r1}")
	s1.Expect("{GET /a r1}")
	s2.Expect()

	ExpectActivateOut(t, server, "s1")
	ExpectInbound(t, URL, "GET", "a", "r2", http.StatusCreated, "s1")
	s0.Expect("{GET /a r2}")
	s1.Expect("{GET /a r2}")
	s2.Expect()

	ExpectAddOut(t, server, "s2", server2)
	ExpectInbound(t, URL, "GET", "a", "r3", http.StatusCreated, "s1")
	s0.Expect("{GET /a r3}")
	s1.Expect("{GET /a r3}")
	s2.Expect("{GET /a r3}")

	ExpectActivateOut(t, server, "s2")
	ExpectInboundTimeout(t, URL, "GET", "a", "r4")
	s0.Expect("{GET /a r4}")
	s1.Expect("{GET /a r4}")
	s2.Expect("{GET /a r4}")

	ExpectActivateOut(t, server, "s0")
	ExpectInbound(t, URL, "GET", "a", "r5", http.StatusOK, "s0")
	s0.Expect("{GET /a r5}")
	s1.Expect("{GET /a r5}")
	s2.Expect("{GET /a r5}")

	ExpectRemoveOut(t, server, "s2")
	ExpectInbound(t, URL, "GET", "a", "r6", http.StatusOK, "s0")
	s0.Expect("{GET /a r6}")
	s1.Expect("{GET /a r6}")
	s2.Expect()
}

func ExpectAddOut(t *testing.T, server *InboundServer, outb string, outServer *httptest.Server) {
	if err := server.AddOutbound(outb, outServer.URL); err != nil {
		t.Errorf("FAIL(add): unable to add {%s, %s} -> %s", outb, outServer.URL, err)
	}
}

func ExpectRemoveOut(t *testing.T, server *InboundServer, outb string) {
	if err := server.RemoveOutbound(outb); err != nil {
		t.Errorf("FAIL(remove): unable to remove '%s' -> %s", outb, err)
	}
}

func ExpectActivateOut(t *testing.T, server *InboundServer, outb string) {
	if err := server.ActivateOutbound(outb); err != nil {
		t.Errorf("FAIL(activate): unable to activate '%s' -> %s", outb, err)
	}
}
