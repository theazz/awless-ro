package awsfetch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestForEachParallelVisitsEveryItem(t *testing.T) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}
	var mu sync.Mutex
	seen := map[int]bool{}

	err := forEachParallel(context.Background(), items, func(_ context.Context, i int) error {
		mu.Lock()
		seen[i] = true
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(items) {
		t.Errorf("visited %d items, want %d", len(seen), len(items))
	}
}

// The point of the helper: a few hundred items must not mean a few hundred requests
// in flight.
func TestForEachParallelBoundsWhatIsInFlight(t *testing.T) {
	var inFlight, peak atomic.Int32
	items := make([]int, 10*maxParallelCalls)

	forEachParallel(context.Background(), items, func(context.Context, int) error {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		inFlight.Add(-1)
		return nil
	})

	if got := peak.Load(); got > maxParallelCalls {
		t.Errorf("%d calls in flight at once, want at most %d", got, maxParallelCalls)
	}
	if got := peak.Load(); got < 2 {
		t.Errorf("calls never overlapped (peak %d); the work is not parallel", got)
	}
}

func TestForEachParallelStopsOnTheFirstError(t *testing.T) {
	boom := errors.New("boom")
	var started atomic.Int32
	items := make([]int, 1000)

	err := forEachParallel(context.Background(), items, func(ctx context.Context, _ int) error {
		if started.Add(1) == 1 {
			return boom
		}
		<-ctx.Done() // the others wait to be told to stop
		return ctx.Err()
	})

	if !errors.Is(err, boom) {
		t.Errorf("got %v, want the first error", err)
	}
	if n := started.Load(); n > 2*maxParallelCalls {
		t.Errorf("%d of %d items were started after the first one failed", n, len(items))
	}
}

func TestForEachParallelReportsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := forEachParallel(ctx, []int{1, 2, 3}, func(context.Context, int) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled: a listing cut short is not an empty listing", err)
	}
}
