// Copyright (c) 2014 Datacratic. All rights reserved.

package nfork

import (
	"math/rand"
	"sort"
)

// DefaultDistributionSize will be used as the default size for the
// Distribution.Items if not otherwise set.
const DefaultDistributionSize = 1000

// Distribution collects a set of outcomes to calculate various percentiles
// using reservoir sampling to avoid unbounded memory usage.
type Distribution struct {

	// Items holds the value whose sie determines the size of the reservoir.
	Items []uint64

	// Count is the number of elements sampled.
	Count uint64

	// Rand is the RNG used for sampling.
	Rand *rand.Rand

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

// Sample adds a new value to the distribution.
func (dist *Distribution) Sample(value uint64) {
	dist.init()

	if value > dist.max {
		dist.max = value
	}

	dist.Count++

	if dist.Count < uint64(len(dist.Items)) {
		dist.Items[dist.Count] = value

	} else if i := rand.Int63n(int64(dist.Count)); int(i) < len(dist.Items) {
		dist.Items[i] = value
	}
}

// Percentiles returns the approximated 99th, 90th and 50th percentile as well
// as the maximum value seen.
func (dist *Distribution) Percentiles() (p50, p90, p99, max uint64) {
	if len(dist.Items) == 0 {
		return
	}

	n := int(dist.Count)
	if n > len(dist.Items) {
		n = len(dist.Items)
	}

	items := make([]uint64, n)
	for i := 0; i < n; i++ {
		items[i] = dist.Items[i]
	}
	sort.Sort(sampleArray(items))

	percentile := func(p int) uint64 {
		return items[int(float32(n)/100*float32(p))]
	}

	p50 = percentile(50)
	p90 = percentile(90)
	p99 = percentile(99)
	max = dist.max

	return
}

type sampleArray []uint64

func (array sampleArray) Len() int           { return len(array) }
func (array sampleArray) Swap(i, j int)      { array[i], array[j] = array[j], array[i] }
func (array sampleArray) Less(i, j int) bool { return array[i] < array[j] }
