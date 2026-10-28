package server

import (
	"fmt"
	"sync"
	"testing"
)

func TestGenerationMonotonicAndLexicographic(t *testing.T) {
	ticks := []int64{999, 999, 1000, 1000, 500}
	i := 0
	g := newGenerationSource(func() int64 {
		v := ticks[i]
		i++
		return v
	})

	got := make([]string, len(ticks))
	for n := range ticks {
		got[n] = g.Next()
		if len(got[n]) != 19 {
			t.Fatalf("generation %q is not 19 digits", got[n])
		}
	}

	want := []int64{999, 1000, 1001, 1002, 1003}
	for n, value := range want {
		formatted := fmt.Sprintf("%019d", value)
		if got[n] != formatted {
			t.Fatalf("generation %d = %s, want %s", n, got[n], formatted)
		}
		if n > 0 && got[n-1] >= got[n] {
			t.Fatalf("lexicographic order: %s >= %s", got[n-1], got[n])
		}
	}

	if fmt.Sprintf("%019d", int64(999)) >= fmt.Sprintf("%019d", int64(1000)) {
		t.Fatal("padded generations are not lexicographically ordered")
	}
	if fmt.Sprintf("%d", 999) < fmt.Sprintf("%d", 1000) {
		t.Fatal("unpadded decimals sorted numerically; padding would not be what this test locks")
	}
}

func TestGenerationConcurrent(t *testing.T) {
	g := newGenerationSource(func() int64 { return 5000 })

	const n = 64
	got := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			got[i] = g.Next()
		}()
	}
	wg.Wait()

	seen := map[string]struct{}{}
	for _, value := range got {
		if _, ok := seen[value]; ok {
			t.Fatalf("duplicate generation %s", value)
		}
		seen[value] = struct{}{}
		if len(value) != 19 {
			t.Fatalf("generation %q is not 19 digits", value)
		}
	}

	prev := fmt.Sprintf("%019d", 4999)
	for value := int64(5000); value < 5000+n; value++ {
		formatted := fmt.Sprintf("%019d", value)
		if _, ok := seen[formatted]; !ok {
			t.Fatalf("missing %s", formatted)
		}
		if prev >= formatted {
			t.Fatalf("lexicographic order: %s >= %s", prev, formatted)
		}
		prev = formatted
	}
}

func TestProcessGenerationStrictlyIncreases(t *testing.T) {
	prev := nextGeneration()
	if len(prev) != 19 {
		t.Fatalf("generation %q is not 19 digits", prev)
	}

	for range 20 {
		next := nextGeneration()
		if len(next) != 19 || next <= prev {
			t.Fatalf("%s then %s", prev, next)
		}
		prev = next
	}
}
