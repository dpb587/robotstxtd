package server

import (
	"fmt"
	"sync"
	"time"
)

// generationSource is a process-local generator. Values are zero-padded
// decimal Unix nanoseconds, so lexicographic order matches numeric order.
// When the clock does not move, the next value is one greater than the last.
type generationSource struct {
	mu   sync.Mutex
	last int64
	now  func() int64
}

func newGenerationSource(now func() int64) *generationSource {
	return &generationSource{now: now}
}

func (g *generationSource) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	n := g.now()
	if n <= g.last {
		n = g.last + 1
	}
	g.last = n

	return fmt.Sprintf("%019d", n)
}

var processGenerations = newGenerationSource(func() int64 {
	return time.Now().UnixNano()
})

func nextGeneration() string {
	return processGenerations.Next()
}
