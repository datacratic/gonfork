// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"encoding/json"
	"math/rand"
	"sort"
)

const DefaultDistributionSize = 1000

type Distribution struct {
	Items []uint64
	Count uint64
	Rand  *rand.Rand

	max uint64
}

func (dist *Distribution) init() {
	if dist.Items == nil {
		dist.Items = make([]uint64, DefaultDistributionSize)
	}

	if dist.Rand == nil {
		dist.Rand = rand.New(rand.NewSource(0))
	}
}

func (dist *Distribution) Sample(value uint64) {
	dist.init()

	if value > dist.max {
		dist.max = value
	}

	dist.Count++

	if len(dist.Items) < cap(dist.Items) {
		dist.Items = append(dist.Items, value)

	} else if i := rand.Int63n(int64(dist.Count)); int(i) < len(dist.Items) {
		dist.Items[i] = value
	}
}

func (dist *Distribution) Percentiles() (p50, p90, p99, max uint64) {
	if len(dist.Items) == 0 {
		return
	}

	n := len(dist.Items)

	items := make([]uint64, n)
	for i, v := range dist.Items {
		items[i] = v
	}
	sort.Sort(sampleArray(items))

	p50 = items[(n/100)*50]
	p90 = items[(n/100)*90]
	p99 = items[(n/100)*99]
	max = dist.max

	return
}

func (dist *Distribution) MarshalJSON() ([]byte, error) {
	p50, p90, p99, max := dist.Percentiles()

	distJSON := struct{ p50, p90, p99, max uint64 }{p50, p90, p99, max}

	return json.Marshal(&distJSON)

}

type sampleArray []uint64

func (array sampleArray) Len() int           { return len(array) }
func (array sampleArray) Swap(i, j int)      { array[i], array[j] = array[j], array[i] }
func (array sampleArray) Less(i, j int) bool { return array[i] < array[j] }
