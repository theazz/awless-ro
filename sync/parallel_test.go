package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
)

// A sync talks to nine AWS services, and the slowest one decides how long the whole
// thing takes only if they run in parallel. These tests cover that, and the part that
// matters more: one service failing must not cost the others their results. A caller
// whose IAM permissions exclude, say, CloudFront should still get their instances.

// controllableService is a cloud.Service whose fetch can be made to block, fail, or
// count how many fetches are in flight at once.
type controllableService struct {
	name, region, profile string
	graph                 cloud.GraphAPI
	err                   error
	disabled              bool

	release  chan struct{}
	inFlight *int32
	peak     *int32
	fetches  int32
}

func (s *controllableService) Name() string            { return s.name }
func (s *controllableService) Region() string          { return s.region }
func (s *controllableService) Profile() string         { return s.profile }
func (s *controllableService) ResourceTypes() []string { return nil }
func (s *controllableService) IsSyncDisabled() bool    { return s.disabled }

func (s *controllableService) Fetch(context.Context) (cloud.GraphAPI, error) {
	atomic.AddInt32(&s.fetches, 1)

	if s.inFlight != nil {
		now := atomic.AddInt32(s.inFlight, 1)
		for {
			peak := atomic.LoadInt32(s.peak)
			if now <= peak || atomic.CompareAndSwapInt32(s.peak, peak, now) {
				break
			}
		}
		defer atomic.AddInt32(s.inFlight, -1)
	}

	if s.release != nil {
		<-s.release
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.graph, nil
}

func (s *controllableService) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func graphWith(t *testing.T, resourceType, id, name string) cloud.GraphAPI {
	t.Helper()
	g := graph.NewGraph()
	res := graph.InitResource(resourceType, id)
	res.Properties()[properties.Name] = name
	if err := g.AddResource(res); err != nil {
		t.Fatal(err)
	}
	return g
}

// Services are fetched at the same time, not one after another.
func TestServicesAreFetchedInParallel(t *testing.T) {
	t.Setenv("__AWLESS_HOME", t.TempDir())

	var inFlight, peak int32
	release := make(chan struct{})

	const count = 5
	services := make([]cloud.Service, 0, count)
	for i := range count {
		services = append(services, &controllableService{
			name:     fmt.Sprintf("service%d", i),
			region:   "eu-west-1",
			profile:  "admin",
			graph:    graph.NewGraph(),
			release:  release,
			inFlight: &inFlight,
			peak:     &peak,
		})
	}

	done := make(chan error, 1)
	go func() {
		_, err := NewSyncer().Sync(services...)
		done <- err
	}()

	// Every fetch has to reach the barrier before any of them is let go. If the
	// syncer were sequential this would time out, because the first fetch would be
	// waiting for a release that only comes once all five have arrived.
	deadline := time.After(5 * time.Second)
	for atomic.LoadInt32(&inFlight) < count {
		select {
		case <-deadline:
			t.Fatalf("only %d of %d fetches started; services are not fetched in parallel", atomic.LoadInt32(&inFlight), count)
		case err := <-done:
			t.Fatalf("sync returned early: %v", err)
		default:
			time.Sleep(time.Millisecond)
		}
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&peak); got != count {
		t.Errorf("peak concurrent fetches was %d, want %d", got, count)
	}
}

// One service failing must not cost the others their results. Otherwise a caller whose
// permissions exclude one service would get nothing at all.
func TestOneFailingServiceDoesNotLoseTheOthers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("__AWLESS_HOME", home)

	good := &controllableService{
		name: "infra", region: "eu-west-1", profile: "admin",
		graph: graphWith(t, "instance", "i-1", "web"),
	}
	broken := &controllableService{
		name: "cdn", region: "global", profile: "admin",
		err: errors.New("AccessDenied"),
	}
	alsoGood := &controllableService{
		name: "storage", region: "eu-west-1", profile: "admin",
		graph: graphWith(t, "bucket", "b-1", "logs"),
	}

	graphs, err := NewSyncer().Sync(good, broken, alsoGood)

	// The failure is reported...
	if err == nil {
		t.Fatal("expected the failure to be reported")
	}
	if !strings.Contains(err.Error(), "cdn") {
		t.Errorf("the error should name the service that failed, got: %s", err)
	}

	// ...and the others still produced graphs and files.
	for _, name := range []string{"infra", "storage"} {
		if _, ok := graphs[name]; !ok {
			t.Errorf("%s produced no graph despite succeeding", name)
		}
	}
	if _, ok := graphs["cdn"]; ok {
		t.Error("the failing service should not have contributed a graph")
	}

	for _, tc := range []struct{ service, region string }{
		{"infra", "eu-west-1"},
		{"storage", "eu-west-1"},
	} {
		path := filepath.Join(home, "aws", "rdf", "admin", tc.region, tc.service+fileExt)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was not written: %s", path, err)
		}
	}
}

// A service that says it is disabled is not fetched at all.
func TestDisabledServiceIsSkipped(t *testing.T) {
	t.Setenv("__AWLESS_HOME", t.TempDir())

	off := &controllableService{name: "cdn", region: "global", profile: "admin", disabled: true}
	on := &controllableService{name: "infra", region: "eu-west-1", profile: "admin", graph: graph.NewGraph()}

	graphs, err := NewSyncer().Sync(off, on)
	if err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt32(&off.fetches); got != 0 {
		t.Errorf("the disabled service was fetched %d times", got)
	}
	if _, ok := graphs["cdn"]; ok {
		t.Error("a disabled service should contribute no graph")
	}
	if _, ok := graphs["infra"]; !ok {
		t.Error("the enabled service should still be fetched")
	}
}

// What was written has to come back: this is the whole point of syncing, and every
// --local command depends on it.
func TestSyncedGraphsReadBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("__AWLESS_HOME", home)

	infra := &controllableService{
		name: "infra", region: "eu-west-1", profile: "admin",
		graph: graphWith(t, "instance", "i-1", "web"),
	}
	// access lives under the global region, which is why LoadLocalGraphForService
	// has a special case for it.
	access := &controllableService{
		name: "access", region: "global", profile: "admin",
		graph: graphWith(t, "user", "u-1", "jane"),
	}

	if _, err := NewSyncer().Sync(infra, access); err != nil {
		t.Fatal(err)
	}

	t.Run("one service at a time", func(t *testing.T) {
		g, err := LoadLocalGraphForService("infra", "admin", "eu-west-1")
		if err != nil {
			t.Fatal(err)
		}
		assertHasResource(t, g, "instance", "i-1")

		// access is asked for with a real region and still found, because the loader
		// knows it is stored globally.
		g, err = LoadLocalGraphForService("access", "admin", "eu-west-1")
		if err != nil {
			t.Fatal(err)
		}
		assertHasResource(t, g, "user", "u-1")
	})

	t.Run("region and global together", func(t *testing.T) {
		g, err := LoadLocalGraphs("admin", "eu-west-1")
		if err != nil {
			t.Fatal(err)
		}
		assertHasResource(t, g, "instance", "i-1")
		assertHasResource(t, g, "user", "u-1")
	})

	t.Run("every region", func(t *testing.T) {
		g, err := LoadAllLocalGraphs("admin")
		if err != nil {
			t.Fatal(err)
		}
		assertHasResource(t, g, "instance", "i-1")
		assertHasResource(t, g, "user", "u-1")
	})

	t.Run("another profile sees nothing", func(t *testing.T) {
		g, err := LoadLocalGraphs("someone-else", "eu-west-1")
		if err != nil {
			t.Fatal(err)
		}
		res, err := g.Find(cloud.NewQuery("instance"))
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 0 {
			t.Errorf("a different profile saw %d resources; graphs are stored per profile", len(res))
		}
	})
}

// Reading a profile that was never synced gives an empty graph rather than an error,
// because `list --local` before the first sync is a normal thing to do.
func TestReadingAnUnsyncedProfileIsEmptyNotAnError(t *testing.T) {
	t.Setenv("__AWLESS_HOME", t.TempDir())

	g, err := LoadLocalGraphForService("infra", "nobody", "eu-west-1")
	if err != nil {
		t.Fatal(err)
	}
	if g == nil {
		t.Fatal("expected an empty graph, got nil")
	}
	res, err := g.Find(cloud.NewQuery("instance"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("got %d resources from an unsynced profile", len(res))
	}
}

func assertHasResource(t *testing.T, g cloud.GraphAPI, resourceType, id string) {
	t.Helper()
	res, err := g.Find(cloud.NewQuery(resourceType))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Id() == id {
			return
		}
	}
	t.Errorf("%s %q not found; got %v", resourceType, id, res)
}
