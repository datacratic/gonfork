// Copyright (c) 2014 Datacratic. All rights reserved.

package main

import (
	"github.com/datacratic/goklog/klog"
	"github.com/datacratic/gometer/meter"
	"github.com/datacratic/gonfork/nfork"
	"github.com/datacratic/gorest/rest"

	"encoding/json"
	"flag"
	"io/ioutil"
	_ "net/http/pprof"
)

var (
	config = flag.String(
		"config", "nfork.json",
		"file containing initial description of routes")

	listen = flag.String(
		"listen", "0.0.0.0:9090",
		"listen interface for the nfork controller interface")

	carbon = flag.String(
		"carbon", "",
		"carbon host where metrics should be directed to")
)

func main() {
	flag.Parse()

	klog.SetPrinter(
		klog.Chain(klog.NewFilterREST("", klog.FilterOut).AddSuffix("debug", "timeout"),
			klog.Chain(klog.NewDedup(),
				klog.Fork(
					klog.NewRingREST("", 1000),
					klog.GetPrinter()))))

	meter.Handle(meter.NewRESTHandler(""))
	if *carbon != "" {
		meter.Handle(&meter.CarbonHandler{URLs: []string{*carbon}})
	}

	body, err := ioutil.ReadFile(*config)
	if err != nil {
		klog.KFatalf("main.error", "unable to read file '%s': %s", *config, err.Error())
	}

	controller := new(nfork.Controller)
	if err := json.Unmarshal(body, &controller.Inbounds); err != nil {
		klog.KFatalf("main.error", "unable to parse config '%s': %s", *config, err.Error())
	}

	klog.KPrintf("init.info", "starting nfork control on %s\n", *listen)
	controller.Start()

	rest.AddService(controller)
	rest.ListenAndServe(*listen, nil)
}
