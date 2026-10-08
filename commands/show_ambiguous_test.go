package commands

import (
	"math/rand"
	"testing"

	"github.com/theazz/awless-ro/cloud"
	p "github.com/theazz/awless-ro/cloud/properties"
)

// A keypair and a classic load balancer both called 'prod' share one graph subject,
// because both use a bare AWS name as their id. `show prod` used to die on that;
// listing the matches and suggesting `show <id>` for each is no use either, since the
// ids are equal. chooseShowTarget is the decision, extracted so it can be tested:
// findResourceInLocalGraphs around it reads global config and calls os.Exit.

func TestChooseShowTarget(t *testing.T) {
	keypair := resource("keypair", "prod", p.Name, "prod")
	elb := resource("classicloadbalancer", "prod", p.Name, "prod")

	t.Run("resources sharing an id pick one deterministically", func(t *testing.T) {
		// Shuffled, because the order out of the graph is Go map order and the
		// answer must not depend on it.
		rnd := rand.New(rand.NewSource(1))
		for i := 0; i < 20; i++ {
			resources := []cloud.Resource{keypair, elb}
			rnd.Shuffle(len(resources), func(a, b int) {
				resources[a], resources[b] = resources[b], resources[a]
			})

			target, ok := chooseShowTarget(resources)
			if !ok {
				t.Fatalf("iteration %d: expected a target", i)
			}
			// Sorted by (Id, Type), so the classic ELB comes first.
			if target.Type() != "classicloadbalancer" {
				t.Fatalf("iteration %d: got the %s, want the classicloadbalancer", i, target.Type())
			}
		}
	})

	t.Run("distinct ids keep the existing listing", func(t *testing.T) {
		resources := []cloud.Resource{
			resource("volume", "vol-1", p.Name, "shared"),
			resource("volume", "vol-2", p.Name, "shared"),
		}
		if target, ok := chooseShowTarget(resources); ok {
			t.Errorf("got %s, want no target so the matches are listed", target.Id())
		}
	})

	t.Run("one resource is not ambiguous", func(t *testing.T) {
		if target, ok := chooseShowTarget([]cloud.Resource{keypair}); ok {
			t.Errorf("got %s, want no target", target.Id())
		}
	})

	t.Run("no resource is not ambiguous", func(t *testing.T) {
		if target, ok := chooseShowTarget(nil); ok {
			t.Errorf("got %s, want no target", target.Id())
		}
	})
}
