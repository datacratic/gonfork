// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"github.com/datacratic/gometer/meter"
)

// InboundStats aggregates the stats
type InboundStats struct {

	// Outbounds counts the number of currently active outbounds.
	Outbounds meter.Level

	// Requests counts the number of HTTP requests received on this inbound
	// endpoint.
	Requests meter.Counter

	// ReadErrors counts the number of HTTP requests that could not be read off
	// the wire.
	ReadErrors meter.Counter

	// Errors counts the number of errors encountered by an outbound while
	// proxying an HTTP request.
	Errors meter.MultiCounter

	// Timeouts counts the number of HTTP requests that were not answered in
	// time by each outbound endpoint
	Timeouts meter.MultiCounter

	// Latency records how long each outbound endpoint took to answer an HTTP
	// request.
	Latency meter.MultiDistribution

	// Responses counts how often all endpoints answered with a given HTTP
	// status code.
	Responses meter.MultiCounter
}

func (stats *InboundStats) key(inbound, key string) string {
	return meter.Join("inbounds", inbound, key)
}

func (stats *InboundStats) init(inbound string) {
	meter.Add(stats.key(inbound, "outbounds"), &stats.Outbounds)
	meter.Add(stats.key(inbound, "requests"), &stats.Requests)
	meter.Add(stats.key(inbound, "read-errors"), &stats.ReadErrors)
	meter.Add(stats.key(inbound, "errors"), &stats.Errors)
	meter.Add(stats.key(inbound, "timeouts"), &stats.Timeouts)
	meter.Add(stats.key(inbound, "latency"), &stats.Latency)
	meter.Add(stats.key(inbound, "responses"), &stats.Responses)
}

func (stats *InboundStats) close(inbound string) {
	meter.Remove(stats.key(inbound, "outbounds"))
	meter.Remove(stats.key(inbound, "requests"))
	meter.Remove(stats.key(inbound, "read-errors"))
	meter.Remove(stats.key(inbound, "errors"))
	meter.Remove(stats.key(inbound, "timeouts"))
	meter.Remove(stats.key(inbound, "latency"))
	meter.Remove(stats.key(inbound, "responses"))
}
