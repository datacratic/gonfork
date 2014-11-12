// Copyright (c) 2014 Datacratic. All rights reserved.

package main

import (
	"github.com/datacratic/gonfork"

	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var (
	config = flag.String("config", "routes.json", "file containing initial description of routes")

	port        = flag.Uint("port", 9090, "port of the nfork forwarding interface")
	controlPort = flag.Uint("control-port", 9091, "port of the nfork control interface")
)

const (
	RoutingReadTimeout  = 100 * time.Millisecond
	RoutingWriteTimeout = 100 * time.Millisecond

	ControlReadTimeout  = 1 * time.Second
	ControlWriteTimeout = 1 * time.Second
)

func main() {
	flag.Parse()

	body, err := ioutil.ReadFile(*config)
	if err != nil {
		log.Fatalf("unable to read file '%s': %s", *config, err.Error())
	}

	router := new(nfork.Router)

	err = json.Unmarshal(body, &router.Routes)
	if err != nil {
		log.Fatalf("unable to configure nforkd: %s\n%s", err.Error(), string(body))
	}

	router.Init()

	routingServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      router,
		ReadTimeout:  RoutingReadTimeout,
		WriteTimeout: RoutingWriteTimeout,
	}
	go log.Fatal(routingServer.ListenAndServe())

	controlServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *controlPort),
		Handler:      router,
		ReadTimeout:  ControlReadTimeout,
		WriteTimeout: ControlWriteTimeout,
	}
	log.Fatal(controlServer.ListenAndServe())
}
