// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"math/rand"
	"sync"
	"time"
)

const DefaultSampleRate = 1 * time.Second

type Stats struct {
	Requests  uint64         `json:"requests"`
	Timeouts  uint64         `json:"timeouts"`
	Latency   Distribution   `json:"latency"`
	Responses map[int]uint64 `json:"responses"`
}

type Event struct {
	Timeout  bool
	Response int
	Latency  time.Duration
}

type StatsRecorder struct {
	Rate time.Duration
	Rand *rand.Rand

	initialize sync.Once

	mutex         sync.Mutex
	current, prev *Stats

	shutdownC chan int
}

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

func (recorder *StatsRecorder) Close() {
	recorder.Init()
	recorder.shutdownC <- 1
}

func (recorder *StatsRecorder) Record(event Event) {
	recorder.Init()
	recorder.mutex.Lock()

	stats := recorder.current

	stats.Requests++
	stats.Latency.Sample(uint64(event.Latency))

	if event.Timeout {
		stats.Timeouts++

	} else {
		if stats.Responses == nil {
			stats.Responses = make(map[int]uint64)
		}
		stats.Responses[event.Response]++
	}

	recorder.mutex.Unlock()
}

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
