package awsfetch

import (
	"context"
	"sync"
)

// maxParallelCalls is how many per-item API calls one fetcher has in flight at once:
// a location or ACL per bucket, a description per task definition, access keys per
// user, and so on.
//
// These used to start one goroutine per item with no limit, so an account with a few
// hundred buckets or a few thousand task definition revisions fired that many requests
// in the same instant. Two things break under that. Each S3 call goes to the bucket's
// own hostname, so it is also that many simultaneous DNS lookups, and a resolver with
// a cold cache — a VPN's in particular — answers some of them "no such host", which
// the SDK does not retry. And the other services throttle: the SDK retries throttling,
// but from a client-side retry budget a burst like that exhausts. Eight keeps a large
// account fast without either.
const maxParallelCalls = 8

// forEachParallel calls fn for every item, at most maxParallelCalls at a time, and
// returns the first error any call returned. After an error no further item is
// started and the context the running calls were given is cancelled, so they stop
// early; forEachParallel returns once every call it started has returned. Nothing is
// left blocked behind it, which the hand-written fan-outs it replaces could not say:
// they returned on the first error and left the other goroutines waiting forever to
// send on an unbuffered channel nobody read any more.
//
// fn runs concurrently with itself; collecting results is fn's business, under a lock.
func forEachParallel[T any](ctx context.Context, items []T, fn func(context.Context, T) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
		slots    = make(chan struct{}, maxParallelCalls)
	)

	for _, item := range items {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(it T) {
			defer wg.Done()
			defer func() { <-slots }()
			if err := fn(ctx, it); err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}(item)
	}
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}
	// The caller's own context ending is an error too, not an empty success.
	return ctx.Err()
}
