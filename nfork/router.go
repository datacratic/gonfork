// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/gorest/rest"

	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

const RouterPathPrefix = "/v1/routes"

type Router struct {
	Routes []*Route

	initialize sync.Once

	mutex  sync.Mutex
	routes unsafe.Pointer
}

func (router *Router) Init() {
	router.initialize.Do(router.init)
}

func (router *Router) init() {
	state := newRouterState()

	for _, route := range router.Routes {

		in := router.normalize(route.Inbound)

		if err := route.Validate(); err != nil {
			log.Panicf("invalid route '%s': %s", in, err)
		}

		if _, ok := state.Routes[in]; ok {
			log.Panicf("duplicate route '%s'", in)
		}

		state.Routes[in] = route
	}

	router.publish(state)
}

func (router *Router) ServeHTTP(writer http.ResponseWriter, httpReq *http.Request) {
	router.Init()

	state := router.state()
	in := router.normalize(httpReq.URL.Path)

	route, ok := state.Routes[in]
	if !ok {
		http.Error(writer, fmt.Sprintf("unknown route '%s'", httpReq.URL.Path), http.StatusNotFound)
		return
	}

	route.ServeHTTP(writer, httpReq)
}

func (router *Router) RESTRoutes() rest.Routes {
	prefix := RouterPathPrefix

	return rest.Routes{
		rest.NewRoute("GET", prefix+"/", router.ListRoutes),
		rest.NewRoute("POST", prefix+"/", router.AddRoute),

		rest.NewRoute("GET", prefix+"/:inbound", router.GetRoute),
		rest.NewRoute("DELETE", prefix+"/:inbound", router.RemoveRoute),

		rest.NewRoute("PUT", prefix+"/:inbound/:outbound", router.AddOutbound),
		rest.NewRoute("DELETE", prefix+"/:inbound/:outbound", router.RemoveOutbound),

		rest.NewRoute("PUT", prefix+"/:inbound/:outbound/activate", router.Activate),

		rest.NewRoute("GET", prefix+"/stats", router.Stats),
		rest.NewRoute("GET", prefix+"/:inbound/stats", router.InboundStats),
		rest.NewRoute("GET", prefix+"/:inbound/:outbound/stats", router.OutboundStats),
	}
}

func (router *Router) ListRoutes() (routes []string) {
	router.Init()
	state := router.state()

	for in := range state.Routes {
		routes = append(routes, in)
	}

	return
}

func (router *Router) AddRoute(route *Route) error {
	router.Init()

	router.mutex.Lock()
	defer router.mutex.Unlock()

	state := router.state().Copy()

	if err := state.AddRoute(router.normalize(route.Inbound), route); err != nil {
		return err
	}

	router.publish(state)
	return nil
}

func (router *Router) GetRoute(inbound string) (*Route, error) {
	router.Init()
	state := router.state()

	return state.GetRoute(router.normalize(inbound))
}

func (router *Router) RemoveRoute(inbound string) (err error) {
	router.Init()

	router.mutex.Lock()
	defer router.mutex.Unlock()

	state := router.state().Copy()

	if err := state.RemoveRoute(router.normalize(inbound)); err != nil {
		return err
	}

	router.publish(state)
	return nil
}

func (router *Router) AddOutbound(inbound, outbound string, host string) error {
	router.Init()

	router.mutex.Lock()
	defer router.mutex.Unlock()

	state := router.state().Copy()

	route, err := state.GetRoute(router.normalize(inbound))
	if err != nil {
		return err
	}

	if err := route.Add(outbound, host); err != nil {
		return err
	}

	router.publish(state)
	return nil
}

func (router *Router) RemoveOutbound(inbound, outbound string) error {
	router.Init()

	router.mutex.Lock()
	defer router.mutex.Unlock()

	state := router.state().Copy()

	route, err := state.GetRoute(router.normalize(inbound))
	if err != nil {
		return err
	}

	if err := route.Remove(outbound); err != nil {
		return err
	}

	router.publish(state)
	return nil
}

func (router *Router) Activate(inbound, outbound string) error {
	router.Init()

	router.mutex.Lock()
	defer router.mutex.Unlock()

	state := router.state().Copy()

	route, err := state.GetRoute(router.normalize(inbound))
	if err != nil {
		return err
	}

	if err := route.Activate(outbound); err != nil {
		return err
	}

	router.publish(state)
	return nil
}

func (router *Router) Stats() map[string]map[string]*Stats {
	router.Init()
	state := router.state()

	stats := make(map[string]map[string]*Stats)

	for inbound, route := range state.Routes {
		stats[inbound] = route.ReadStats()
	}

	return stats
}

func (router *Router) InboundStats(inbound string) (map[string]*Stats, error) {
	router.Init()
	state := router.state()

	route, err := state.GetRoute(router.normalize(inbound))
	if err != nil {
		return nil, err
	}

	return route.ReadStats(), nil
}

func (router *Router) OutboundStats(inbound, outbound string) (*Stats, error) {
	router.Init()
	state := router.state()

	route, err := state.GetRoute(router.normalize(inbound))
	if err != nil {
		return nil, err
	}

	stats, ok := route.ReadStats()[outbound]
	if !ok {
		return nil, fmt.Errorf("unknown outbound '%s' for inbound '%s'", outbound, inbound)
	}

	return stats, nil
}

func (router *Router) state() *routerState {
	return (*routerState)(atomic.LoadPointer(&router.routes))
}

func (router *Router) publish(state *routerState) {
	atomic.StorePointer(&router.routes, unsafe.Pointer(state))
}

func (router *Router) normalize(path string) string {
	return strings.Trim(path, "/")
}

type routerState struct {
	Routes map[string]*Route
}

func newRouterState() *routerState {
	return &routerState{Routes: make(map[string]*Route)}
}

func (state *routerState) Copy() *routerState {
	newState := newRouterState()

	for in, route := range state.Routes {
		newState.Routes[in] = route.Copy()
	}

	return newState
}

func (state *routerState) GetRoute(inbound string) (*Route, error) {
	if route, ok := state.Routes[inbound]; ok {
		return route, nil
	}

	return nil, fmt.Errorf("inbound '%s' doesn't exist", inbound)
}

func (state *routerState) AddRoute(inbound string, route *Route) error {
	if _, ok := state.Routes[inbound]; ok {
		return fmt.Errorf("inbound '%s' already exists", inbound)
	}

	if err := route.Validate(); err != nil {
		return fmt.Errorf("inbound '%s' is invalid: %s", inbound, err.Error())
	}

	state.Routes[inbound] = route
	return nil
}

func (state *routerState) RemoveRoute(inbound string) error {
	if _, ok := state.Routes[inbound]; !ok {
		return fmt.Errorf("inbound '%s' doesn't exist", inbound)
	}

	delete(state.Routes, inbound)
	return nil
}
