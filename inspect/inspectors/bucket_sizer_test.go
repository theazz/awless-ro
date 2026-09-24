package inspectors

import (
	"bytes"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// An empty graph is what a fresh install has, because object sync is off by default:
// one API call per bucket is too much to spend unasked. The inspector used to print a
// table whose only row read "0.000000 Gb", which answers a question about the account
// that was never looked into.
func TestBucketSizerSaysNothingWasFetchedRatherThanZero(t *testing.T) {
	var out bytes.Buffer
	sizer := &BucketSizer{}

	if err := sizer.Inspect(graph.NewGraph()); err != nil {
		t.Fatal(err)
	}
	sizer.Print(&out)

	got := out.String()
	if strings.Contains(got, "0.000000 Gb") {
		t.Errorf("a total of zero was reported for an account nothing was read from:\n%s", got)
	}
	// And it has to say what to do about it.
	if !strings.Contains(got, "aws.storage.s3object.sync") {
		t.Errorf("the setting that turns object sync on is not mentioned:\n%s", got)
	}
}

func TestBucketSizerTotalsBySizeAndBucket(t *testing.T) {
	g := graph.NewGraph()
	g.AddResource(
		object("obj_1", "logs", 1_500_000_000),
		object("obj_2", "logs", 500_000_000),
		object("obj_3", "backups", 3_000_000_000),
	)

	var out bytes.Buffer
	sizer := &BucketSizer{}
	if err := sizer.Inspect(g); err != nil {
		t.Fatal(err)
	}
	sizer.Print(&out)
	got := out.String()

	for _, want := range []string{
		"backups", "3.000000 Gb",
		"logs", "2.000000 Gb",
		"TOTAL", "5.000000 Gb",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}

	if got, want := sizer.buckets["logs"].objects, 2; got != want {
		t.Errorf("logs holds %d objects, want %d", got, want)
	}
}

// Rows are ordered, because map order printed the same data differently on every run,
// which cannot be compared against an earlier report.
func TestBucketSizerOutputIsOrdered(t *testing.T) {
	g := graph.NewGraph()
	g.AddResource(
		object("a", "zebra", 1),
		object("b", "alpha", 1),
		object("c", "middle", 1),
	)

	var first string
	for range 5 {
		var out bytes.Buffer
		sizer := &BucketSizer{}
		if err := sizer.Inspect(g); err != nil {
			t.Fatal(err)
		}
		sizer.Print(&out)

		if first == "" {
			first = out.String()
			continue
		}
		if out.String() != first {
			t.Fatalf("two runs over the same graph printed different output:\n%s\nand\n%s", first, out.String())
		}
	}

	alpha, zebra := strings.Index(first, "alpha"), strings.Index(first, "zebra")
	if alpha > zebra {
		t.Errorf("rows are not in name order:\n%s", first)
	}
}

// The property reads used to be unchecked type assertions, so an object with no
// Bucket, or a Size that did not arrive as an int, took the process down. An inspector
// reports on whatever the account happens to contain.
func TestBucketSizerSurvivesOddProperties(t *testing.T) {
	g := graph.NewGraph()
	g.AddResource(
		object("good", "logs", 1_000_000_000),
		// No bucket: counted apart rather than attributed or crashed on.
		resourcetest.S3Object("orphan").Prop(properties.Size, 42).Build(),
		// A size that is not an int.
		resourcetest.S3Object("odd").Prop(properties.Bucket, "logs").
			Prop(properties.Size, "not a number").Build(),
		// No size at all.
		resourcetest.S3Object("sizeless").Prop(properties.Bucket, "logs").Build(),
	)

	var out bytes.Buffer
	sizer := &BucketSizer{}
	if err := sizer.Inspect(g); err != nil {
		t.Fatal(err)
	}
	sizer.Print(&out)
	got := out.String()

	// The one good object still counts, and the sizeless ones count as objects
	// without adding to the size.
	if want := "1.000000 Gb"; !strings.Contains(got, want) {
		t.Errorf("output is missing %q:\n%s", want, got)
	}
	if n := sizer.buckets["logs"].objects; n != 3 {
		t.Errorf("logs holds %d objects, want 3", n)
	}
	if sizer.unattributed != 1 {
		t.Errorf("%d objects counted as naming no bucket, want 1", sizer.unattributed)
	}
	if !strings.Contains(got, "named no bucket") {
		t.Errorf("the object naming no bucket is not reported:\n%s", got)
	}
}

func object(id, bucket string, size int) *graph.Resource {
	return resourcetest.S3Object(id).
		Prop(properties.Bucket, bucket).
		Prop(properties.Size, size).
		Build()
}
