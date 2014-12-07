// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"encoding/json"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

// Stats contains the stats of an outbound at a given point in time.
type Stats struct {

	// Requests counts the number of requests made.
	Requests uint64

	// Errors counts the number of errors encountered.
	Errors uint64

	// Timeouts counts the number of timeouts encountered.
	Timeouts uint64

	// Latency is the latency distribution of all requests.
	Latency Distribution

	// Responses counts the number of responses received for an HTTP status
	// code.
	Responses map[int]uint64
}

// MarshalJSON defines a custom JSON format for encoding/json.
func (stats *Stats) MarshalJSON() ([]byte, error) {
	var statsJSON struct {
		Requests  uint64            `json:"requests"`
		Errors    uint64            `json:"errors"`
		Timeouts  uint64            `json:"timeouts"`
		Latency   map[string]string `json:"latency"`
		Responses map[string]uint64 `json:"responses"`
	}

	statsJSON.Requests = stats.Requests
	statsJSON.Errors = stats.Errors
	statsJSON.Timeouts = stats.Timeouts
	statsJSON.Latency = make(map[string]string)
	statsJSON.Responses = make(map[string]uint64)

	p50, p90, p99, max := stats.Latency.Percentiles()
	statsJSON.Latency["p50"] = time.Duration(p50).String()
	statsJSON.Latency["p90"] = time.Duration(p90).String()
	statsJSON.Latency["p99"] = time.Duration(p99).String()
	statsJSON.Latency["pmx"] = time.Duration(max).String()

	for code, count := range stats.Responses {
		statsJSON.Responses[strconv.Itoa(code)] = count
	}

	return json.Marshal(&statsJSON)
}

// Event contains the outcome of an HTTP request.
type Event struct {

	// Error indicates that an error occured.
	Error bool

	// Timeout indicates that the request timed out.
	Timeout bool

	// Response is the HTTP response code received.
	Response int

	// Latency mesures the latency of the request.
	Latency time.Duration
}

// DefaultSampleRate is used if Rate is not set set in StatsRecorder.
const DefaultSampleRate = 1 * time.Second

// StatsRecorder records stats for a given outbound and updates them at a
// given rate.
type StatsRecorder struct {

	// Rate at which stats are updated.
	Rate time.Duration

	// Rand is the RNG used for stats sampling.
	Rand *rand.Rand

	initialize sync.Once

	mutex         sync.Mutex
	current, prev *Stats

	shutdownC chan int
}

// Init initializes the object.
func (recorder *StatsRecorder) Init() {
	recorder.initialize.Do(recorder.init)
}

func (recorder *StatsRecorder) init() {
	if recorder.Rate == 0 {
		recorder.Rate = DefaultSampleRate
	}

	if recorder.Rand == nil {
		recorder.Rand = rand.New(rand.NewSource(0))
	}

	recorder.prev = new(Stats)
	recorder.current = new(Stats)

	recorder.shutdownC = make(chan int)
	go recorder.run()
}

// Close terminates the stats recorder.
func (recorder *StatsRecorder) Close() {
	recorder.Init()
	recorder.shutdownC <- 1
}

// Record records the given outcome.
func (recorder *StatsRecorder) Record(event Event) {
	recorder.Init()
	recorder.mutex.Lock()

	stats := recorder.current

	stats.Requests++
	stats.Latency.Sample(uint64(event.Latency))

	if event.Error {
		stats.Errors++

	} else if event.Timeout {
		stats.Timeouts++

	} else {
		if stats.Responses == nil {
			stats.Responses = make(map[int]uint64)
		}
		stats.Responses[event.Response]++
	}

	recorder.mutex.Unlock()
}

// Read returns the last updated stats.
func (recorder *StatsRecorder) Read() (stats *Stats) {
	recorder.Init()
	recorder.mutex.Lock()

	stats = recorder.prev

	recorder.mutex.Unlock()
	return
}

func (recorder *StatsRecorder) run() {
	tick := time.NewTicker(recorder.Rate)
	for {
		select {
		case <-tick.C:
			recorder.mutex.Lock()

			recorder.prev = recorder.current
			recorder.current = new(Stats)

			recorder.mutex.Unlock()

		case <-recorder.shutdownC:
			tick.Stop()
			return
		}
	}
}
