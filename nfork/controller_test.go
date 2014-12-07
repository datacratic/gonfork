// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestController(t *testing.T) {

	s0 := &TestService{T: t, Name: "s0"}
	server0 := httptest.NewServer(s0)
	defer server0.Close()

	s1 := &TestService{T: t, Name: "s1", Code: http.StatusCreated}
	server1 := httptest.NewServer(s1)
	defer server1.Close()

	s2 := &TestService{T: t, Name: "s2", Sleep: 100 * time.Millisecond}
	server2 := httptest.NewServer(s2)
	defer server2.Close()

	i0, i0URL := NewInbound("i0", "s0", map[string]string{
		"s0": server0.URL,
		"s1": server1.URL,
	})

	i1, i1URL := NewInbound("i1", "s1", map[string]string{
		"s1": server1.URL,
		"s2": server2.URL,
	})

	i2, i2URL := NewInbound("i2", "s2", map[string]string{
		"s0": server0.URL,
		"s1": server1.URL,
		"s2": server2.URL,
	})

	control := NewController([]*Inbound{i0})
	defer control.Close()

	ExpectInbound(t, i0URL, "GET", "a", "r0", http.StatusOK, "s0")
	s0.Expect("{GET /a r0}")
	s1.Expect("{GET /a r0}")
	s2.Expect()

	ExpectAddIn(t, control, i1)
	ExpectInbound(t, i0URL, "GET", "a", "r1", http.StatusOK, "s0")
	ExpectInbound(t, i1URL, "GET", "b", "r1", http.StatusCreated, "s1")
	s0.Expect("{GET /a r1}")
	s1.Expect("{GET /a r1}", "{GET /b r1}")
	s2.Expect("{GET /b r1}")

	ExpectAddIn(t, control, i2)
	ExpectInbound(t, i0URL, "GET", "a", "r2", http.StatusOK, "s0")
	ExpectInbound(t, i1URL, "GET", "b", "r2", http.StatusCreated, "s1")
	ExpectInboundTimeout(t, i2URL, "GET", "c", "r2")
	s0.Expect("{GET /a r2}", "{GET /c r2}")
	s1.Expect("{GET /a r2}", "{GET /b r2}", "{GET /c r2}")
	s2.Expect("{GET /b r2}", "{GET /c r2}")

	ExpectRemoveIn(t, control, "i2")
	ExpectInbound(t, i0URL, "GET", "a", "r3", http.StatusOK, "s0")
	ExpectInbound(t, i1URL, "GET", "b", "r3", http.StatusCreated, "s1")
	s0.Expect("{GET /a r3}")
	s1.Expect("{GET /a r3}", "{GET /b r3}")
	s2.Expect("{GET /b r3}")
}

func NewInbound(name, active string, out map[string]string) (*Inbound, string) {
	listen, URL := AllocatePort()
	return &Inbound{
		Name:     name,
		Listen:   listen,
		Timeout:  50 * time.Millisecond,
		Outbound: out,
		Active:   active,
	}, URL
}

func ExpectAddIn(t *testing.T, control *Controller, inb *Inbound) {
	if err := control.AddInbound(inb); err != nil {
		t.Errorf("FAIL(inbound.add): unable to add '%s' -> %s", inb.Name, err)
	}
}

func ExpectRemoveIn(t *testing.T, control *Controller, inb string) {
	if err := control.RemoveInbound(inb); err != nil {
		t.Errorf("FAIL(inbound.remove): unable to remove '%s' -> %s", inb, err)
	}
}
