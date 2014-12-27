// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/goklog/klog"
	"github.com/datacratic/gorest/rest"

	"fmt"
	"sync"
)

// Controller manages a set of Inbound objects wrapped in InboundServer objects
// and defines a REST interface to do so.
type Controller struct {

	// Inbounds is the initial list of Inbounds.
	Inbounds []*Inbound

	mutex    sync.Mutex
	inbounds map[string]*InboundServer
}

// NewController returns a new Controller object initialized with the given
// inbounds.
func NewController(inbounds []*Inbound) *Controller {
	controller := &Controller{Inbounds: inbounds}
	controller.Start()
	return controller
}

// RESTRoutes defines the REST inteface for a Controller.
func (control *Controller) RESTRoutes() rest.Routes {
	prefix := "/v1/nfork"
	return rest.Routes{
		rest.NewRoute(prefix, "GET", control.List),
		rest.NewRoute(prefix, "POST", control.AddInbound),

		rest.NewRoute(prefix+"/:inbound", "GET", control.ListInbound),
		rest.NewRoute(prefix+"/:inbound", "DELETE", control.RemoveInbound),

		rest.NewRoute(prefix+"/:inbound/:outbound", "PUT", control.AddOutbound),
		rest.NewRoute(prefix+"/:inbound/:outbound", "DELETE", control.RemoveOutbound),
	}
}

// Start initializes and starts the server associated with the configured
// inbounds.
func (control *Controller) Start() {
	control.inbounds = make(map[string]*InboundServer)

	for i, inbound := range control.Inbounds {
		if inbound == nil {
			klog.KFatalf("controller.error", "nil inbound at index %d", i)
		}

		server, err := NewInboundServer(inbound)
		if err != nil {
			klog.KFatal("controller.error", err.Error())
		}

		control.inbounds[inbound.Name] = server
	}
}

// Close closes the managed inbound servers.
func (control *Controller) Close() {
	for _, server := range control.inbounds {
		server.Close()
	}
	control.inbounds = nil
}

// List returns the Inbound object associated with each inbounds.
func (control *Controller) List() (result []*Inbound) {
	control.mutex.Lock()
	defer control.mutex.Unlock()

	for _, inbound := range control.inbounds {
		result = append(result, inbound.List())
	}

	return
}

// ListInbound returns the Inbound object associated with the given inbound.
func (control *Controller) ListInbound(inbound string) (*Inbound, error) {
	control.mutex.Lock()
	defer control.mutex.Unlock()

	server, ok := control.inbounds[inbound]
	if !ok {
		return nil, fmt.Errorf("unknown inbound '%s'", inbound)
	}

	return server.List(), nil
}

// AddInbound creates a new InboundServer for the given inbound and launches it.
func (control *Controller) AddInbound(inbound *Inbound) error {
	control.mutex.Lock()
	defer control.mutex.Unlock()

	if _, ok := control.inbounds[inbound.Name]; ok {
		return fmt.Errorf("inbound '%s' already exists", inbound.Name)
	}

	server, err := NewInboundServer(inbound)
	if err != nil {
		return fmt.Errorf("unable to add inbound '%s': %s", inbound.Name, err)
	}

	klog.KPrintf("controller.info", "AddInbound(%s, %s)", inbound.Name, inbound.Listen)
	control.inbounds[inbound.Name] = server

	return nil
}

// RemoveInbound kills and removes the given inbound.
func (control *Controller) RemoveInbound(inbound string) error {
	control.mutex.Lock()
	defer control.mutex.Unlock()

	server, ok := control.inbounds[inbound]
	if !ok {
		return fmt.Errorf("unknown inbound '%s'", inbound)
	}

	klog.KPrintf("controller.info", "RemoveInbound(%s)", inbound)

	server.Close()
	delete(control.inbounds, inbound)

	return nil
}

// AddOutbound adds an outbound for the given inbound.
func (control *Controller) AddOutbound(inbound, outbound, addr string) error {
	control.mutex.Lock()
	defer control.mutex.Unlock()

	server, ok := control.inbounds[inbound]
	if !ok {
		return fmt.Errorf("unknown inbound '%s'", inbound)
	}

	klog.KPrintf("controller.info", "AddOutbound(%s, %s, %s)", inbound, outbound, addr)
	return server.AddOutbound(outbound, addr)
}

// RemoveOutbound removes the given outbound for the given inbound.
func (control *Controller) RemoveOutbound(inbound, outbound string) error {
	control.mutex.Lock()
	defer control.mutex.Unlock()

	server, ok := control.inbounds[inbound]
	if !ok {
		return fmt.Errorf("unknown inbound '%s'", inbound)
	}

	klog.KPrintf("controller.info", "RemoveOutbound(%s, %s)", inbound, outbound)
	return server.RemoveOutbound(outbound)
}

// ActivateOutbound activates the given outbound for the given inbound.
func (control *Controller) ActivateOutbound(inbound, outbound string) error {
	control.mutex.Lock()
	defer control.mutex.Unlock()

	server, ok := control.inbounds[inbound]
	if !ok {
		return fmt.Errorf("unknown inbound '%s'", inbound)
	}

	klog.KPrintf("controller.info", "ActivateOutbound(%s, %s)", inbound, outbound)
	return server.ActivateOutbound(outbound)
}
