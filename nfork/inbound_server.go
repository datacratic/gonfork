// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/goklog/klog"

	"net"
	"net/http"
	"sync/atomic"
	"unsafe"
)

// InboundServer wraps an Inbound object into an HTTP server and allows the
// Inbound to be safely manipulated using a copy-on-write scheme.
//
// InboundServer currently assumes that the various management functions are
// synchronized externally.
type InboundServer struct {
	listener net.Listener
	inbound  unsafe.Pointer
}

// NewInboundServer creates and starts a new HTTP server associated with the
// given Inbound.
func NewInboundServer(inbound *Inbound) (*InboundServer, error) {
	server := new(InboundServer)

	if err := inbound.Validate(); err != nil {
		return nil, err
	}
	inbound.Init()

	server.setInbound(inbound)

	listener, err := net.Listen("tcp", inbound.Listen)
	if err != nil {
		klog.KPrintf(klog.Keyf("%s.listen", inbound.Name), "unable to listen on %s: %s", inbound.Listen, err)
		return nil, err
	}
	server.listener = listener

	go func() {
		err := http.Serve(tcpKeepAliveListener{listener.(*net.TCPListener)}, server)
		klog.KPrintf(klog.Keyf("%s.close", server.getInbound().Name), "server closed with: %s", err)
	}()

	return server, nil
}

// Close closes the HTTP server releasing all associated resources.
func (server *InboundServer) Close() {
	server.listener.Close()
}

// ServeHTTP forwards the given HTTP request to the managed inbound.
func (server *InboundServer) ServeHTTP(writer http.ResponseWriter, httpReq *http.Request) {
	server.getInbound().ServeHTTP(writer, httpReq)
}

// List returns the managed inbound.
func (server *InboundServer) List() *Inbound {
	return server.getInbound()
}

// ReadStats calls ReadStats on the managed inbound.
func (server *InboundServer) ReadStats() map[string]*Stats {
	return server.getInbound().ReadStats()
}

// ReadOutboundStats calls ReadOutboundStats on the managed inbound.
func (server *InboundServer) ReadOutboundStats(outbound string) (*Stats, error) {
	return server.getInbound().ReadOutboundStats(outbound)
}

// AddOutbound calls AddOutbound on the managed inbound.
func (server *InboundServer) AddOutbound(outbound, addr string) error {
	inbound := server.getInbound().Copy()

	if err := inbound.AddOutbound(outbound, addr); err != nil {
		return err
	}

	server.setInbound(inbound)
	return nil
}

// RemoveOutbound calls RemoveOutbound on the managed inbound.
func (server *InboundServer) RemoveOutbound(outbound string) error {
	inbound := server.getInbound().Copy()

	if err := inbound.RemoveOutbound(outbound); err != nil {
		return err
	}

	server.setInbound(inbound)
	return nil
}

// ActivateOutbound calls ActivateOutbound on the managed inbound.
func (server *InboundServer) ActivateOutbound(outbound string) error {
	inbound := server.getInbound().Copy()

	if err := inbound.ActivateOutbound(outbound); err != nil {
		return err
	}

	server.setInbound(inbound)
	return nil
}

func (server *InboundServer) setInbound(inbound *Inbound) {
	atomic.StorePointer(&server.inbound, unsafe.Pointer(inbound))
}

func (server *InboundServer) getInbound() *Inbound {
	return (*Inbound)(atomic.LoadPointer(&server.inbound))
}
