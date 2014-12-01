// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/gorest/rest"

	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"
)

const (
	ControllerReadTimeout  = 1 * time.Second
	ControllerWriteTimeout = 1 * time.Second
)

type Controller struct {
	Routes []*Route

	routes map[string]*Route
}

func (control *Controller) RESTRoutes() rest.Routes {
	prefix := "/v1"
	return rest.Routes{
		rest.NewRoute(prefix, "GET", control.List),
		rest.NewRoute(prefix+"/stats", "GET", control.ReadStats),

		rest.NewRoute(prefix+"/:route", "GET", control.ListRoute),
		rest.NewRoute(prefix+"/:route/stats", "GET", control.ReadRouteStats),
	}
}

func (control *Controller) init() {
	if len(control.Routes) == 0 {
		log.Fatal("no routes setup")
	}

	control.routes = make(map[string]*Route)
	for i, route := range control.Routes {

		if route == nil {
			log.Fatalf("nil route at index %d", i)

		} else if err := route.Validate(); err != nil {
			log.Fatalf("invalid route at index %d: %s", i, err.Error())
		}

		route.Init()
		control.routes[route.Name] = route
	}
}

func (control *Controller) ReadStats() map[string]map[string]*Stats {
	stats := make(map[string]map[string]*Stats)
	for name, route := range control.routes {
		stats[name] = route.ReadStats()
	}

	return stats
}

func (control *Controller) ReadRouteStats(name string) (map[string]*Stats, error) {
	route, ok := control.routes[name]
	if !ok {
		return nil, fmt.Errorf("unknown route '%s'", name)
	}

	return route.ReadStats(), nil
}

func (control *Controller) List() []*Route { return control.Routes }

func (control *Controller) ListRoute(name string) (*Route, error) {
	if route, ok := control.routes[name]; ok {
		return route, nil
	}
	return nil, fmt.Errorf("unknown route '%s'", name)
}

func (control *Controller) Start() {
	control.init()

	for _, route := range control.routes {
		routeServer := &http.Server{
			Addr:         route.Listen,
			Handler:      route,
			ReadTimeout:  route.Timeout,
			WriteTimeout: route.Timeout,
		}

		go func() { log.Fatal(routeServer.ListenAndServe()) }()
	}

	rest.AddService(control)
}
