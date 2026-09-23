package awsservices

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/smithy-go/middleware"

	"github.com/theazz/awless-ro/console"
)

// DefaultNetworkMonitor records the timing of every AWS call made during a
// command, for the hidden --network-monitor flag.
//
// SDK v1 hung this off session.Handlers and keyed it on *request.Request. SDK v2
// has no request object and no handler list, so it is a smithy middleware
// instead: one around the whole operation for the overall span, and one around
// each attempt so that retries stay visible.
var DefaultNetworkMonitor = &NetworkMonitor{}

type NetworkMonitor struct {
	l        sync.Mutex
	requests []*req
}

type req struct {
	name     string
	from     time.Time
	to       time.Time
	attempts []time.Time
}

// APIOptions returns the middleware registration to pass to
// config.WithAPIOptions so that every client reports into this monitor.
func (n *NetworkMonitor) APIOptions() []func(*middleware.Stack) error {
	return []func(*middleware.Stack) error{
		func(stack *middleware.Stack) error {
			if err := stack.Initialize.Add(middleware.InitializeMiddlewareFunc("awlessNetworkMonitorOperation",
				func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (
					middleware.InitializeOutput, middleware.Metadata, error) {
					r := n.begin(awsmiddleware.GetOperationName(ctx))
					out, md, err := next.HandleInitialize(ctx, in)
					n.end(r)
					return out, md, err
				}), middleware.Before); err != nil {
				return err
			}

			return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("awlessNetworkMonitorAttempt",
				func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
					middleware.FinalizeOutput, middleware.Metadata, error) {
					n.attempt(awsmiddleware.GetOperationName(ctx))
					return next.HandleFinalize(ctx, in)
				}), middleware.After)
		},
	}
}

func (n *NetworkMonitor) begin(name string) *req {
	n.l.Lock()
	defer n.l.Unlock()
	r := &req{name: name, from: time.Now().UTC()}
	n.requests = append(n.requests, r)
	return r
}

func (n *NetworkMonitor) end(r *req) {
	n.l.Lock()
	defer n.l.Unlock()
	r.to = time.Now().UTC()
}

// attempt records a retry. The first attempt of an operation is the operation
// span itself, so only the later ones are kept.
func (n *NetworkMonitor) attempt(name string) {
	n.l.Lock()
	defer n.l.Unlock()
	for i := len(n.requests) - 1; i >= 0; i-- {
		if n.requests[i].name == name && n.requests[i].to.IsZero() {
			if !n.requests[i].from.IsZero() && time.Since(n.requests[i].from) > 0 {
				n.requests[i].attempts = append(n.requests[i].attempts, time.Now().UTC())
			}
			return
		}
	}
}

func (n *NetworkMonitor) DisplayStats(w io.Writer) {
	n.l.Lock()
	defer n.l.Unlock()

	fmt.Fprintf(w, "\n%d requests sent:\n", len(n.requests))
	if len(n.requests) == 0 {
		return
	}

	var min, max time.Time
	var maxFunctionNameLength int
	sorted := make([]*req, 0, len(n.requests))
	for _, r := range n.requests {
		if min.IsZero() || r.from.Before(min) {
			min = r.from
		}
		if max.IsZero() || r.to.After(max) {
			max = r.to
		}
		if len(r.name) > maxFunctionNameLength {
			maxFunctionNameLength = len(r.name)
		}
		sorted = append(sorted, r)
	}
	sort.Slice(sorted, func(i int, j int) bool {
		if sorted[i].from.Equal(sorted[j].from) {
			return sorted[i].to.Before(sorted[j].to)
		}
		return sorted[i].from.Before(sorted[j].from)
	})

	width := console.GetTerminalWidth() - maxFunctionNameLength - 11 // 11 = '['+']'+' '+'('+4+'m'+'s'+')'
	if width < 1 {
		width = 1
	}
	maxwidth := uint(width)

	maxDuration := max.Sub(min)
	if maxDuration <= 0 {
		maxDuration = time.Millisecond
	}

	// Retries beyond the first are drawn as separate segments, so that a slow
	// call and a retried call look different.
	for _, r := range sorted {
		if len(r.attempts) > 0 {
			drawRequest(w, r.name, min, r.from, r.attempts[0], maxwidth, maxDuration, "[", "X")
			for i := 0; i < len(r.attempts)-1; i++ {
				drawRequest(w, r.name, min, r.attempts[i], r.attempts[i+1], maxwidth, maxDuration, "o", "X")
			}
			drawRequest(w, r.name, min, r.attempts[len(r.attempts)-1], r.to, maxwidth, maxDuration, "o", "]")
			continue
		}
		drawRequest(w, r.name, min, r.from, r.to, maxwidth, maxDuration, "[", "]")
	}
}

func drawRequest(w io.Writer, name string, min, from, to time.Time, maxwidth uint, maxduration time.Duration, startChar, stopChar string) {
	duration := to.Sub(from)
	if duration < 0 {
		duration = 0
	}
	width := uint(duration) * maxwidth / uint(maxduration)
	before := uint(from.Sub(min)) * maxwidth / uint(maxduration)
	if before+width > maxwidth {
		width = maxwidth - before
	}
	after := maxwidth - width - before
	fmt.Fprintf(w, "%s%s%s%s%s %s(%dms)\n",
		strings.Repeat(" ", int(before)), startChar, strings.Repeat("-", int(width)), stopChar,
		strings.Repeat(" ", int(after)), name, duration/time.Millisecond)
}
