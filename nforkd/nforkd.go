// Copyright (c) 2014 Datacratic. All rights reserved.

package main

import (
	"github.com/datacratic/goklog/klog"
	"github.com/datacratic/gonfork/nfork"
	"github.com/datacratic/gorest/rest"

	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	_ "net/http/pprof"
)

var (
	config = flag.String(
		"config", "nfork.json",
		"file containing initial description of routes")

	listen = flag.String(
		"listen", "0.0.0.0:9090",
		"listen interface for the nfork controller interface")
)

func main() {
	flag.Parse()

	klog.SetPrinter(
		klog.Chain(klog.NewFilterREST("", klog.FilterOut).AddSuffix("debug", "timeout"),
			klog.Chain(klog.NewDedup(),
				klog.Fork(
					klog.NewRingREST("", 1000),
					klog.GetPrinter()))))

	body, err := ioutil.ReadFile(*config)
	if err != nil {
		log.Fatalf("unable to read file '%s': %s", *config, err.Error())
	}

	controller := new(nfork.Controller)
	if err := json.Unmarshal(body, &controller.Inbounds); err != nil {
		log.Fatalf("unable to parse config '%s': %s", *config, err.Error())
	}

	klog.KPrintf("init.info", "starting nfork control on %s\n", *listen)
	controller.Start()

	rest.AddService(controller)
	rest.ListenAndServe(*listen, nil)
}
