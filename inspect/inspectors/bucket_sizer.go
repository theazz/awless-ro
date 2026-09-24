/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package inspectors

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
)

type BucketSizer struct {
	total   int
	buckets map[string]*bucket

	// objectsInGraph separates "every bucket is empty" from "no objects were ever
	// fetched", which are the same zero in the total and very different answers.
	objectsInGraph int

	// unattributed counts objects carrying no bucket name. Reported rather than
	// dropped, so the total can be reconciled with the rows.
	unattributed int
}

type bucket struct {
	objects, size int
}

func (*BucketSizer) Name() string {
	return "bucket_sizer"
}

func (i *BucketSizer) Inspect(g cloud.GraphAPI) error {
	i.buckets = make(map[string]*bucket)

	objects, err := g.Find(cloud.NewQuery(cloud.S3Object))
	if err != nil {
		return err
	}
	i.objectsInGraph = len(objects)

	for _, obj := range objects {
		// Read rather than asserted. These were unchecked type assertions, so an
		// object whose Size did not arrive as an int, or that carried no Bucket, took
		// the process down — and an inspector exists to report on whatever the
		// account happens to contain.
		name, ok := obj.Properties()[properties.Bucket].(string)
		if !ok || name == "" {
			i.unattributed++
			continue
		}
		size, _ := obj.Properties()[properties.Size].(int)

		i.total = i.total + size
		b := i.buckets[name]
		if b == nil {
			b = new(bucket)
			i.buckets[name] = b
		}
		b.size = b.size + size
		b.objects = b.objects + 1
	}

	return nil
}

func (i *BucketSizer) Print(w io.Writer) {
	// No objects at all is the state a fresh install is in, because object sync is
	// off by default: one API call per bucket is too much to spend unasked. Printing
	// a table whose only row read "0.000000 Gb" answered a question about the account
	// that had not been looked into.
	if i.objectsInGraph == 0 {
		fmt.Fprintln(w, "No S3 objects in the local graph, so there is nothing to size.")
		fmt.Fprintln(w, "Object sync is off by default because it costs one API call per bucket. Turn it on with:")
		fmt.Fprintln(w, "\tawless-ro config set aws.storage.s3object.sync true")
		return
	}

	tabw := tabwriter.NewWriter(w, 0, 8, 0, '\t', 0)

	fmt.Fprintln(tabw, "Bucket\tObject count\tS3 total storage\t")
	fmt.Fprintln(tabw, "--------\t----------\t-----------------\t")

	// Sorted, because map order made the same data print in a different order every
	// run, which is unreadable for a report meant to be compared against an earlier one.
	names := make([]string, 0, len(i.buckets))
	for name := range i.buckets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		bucket := i.buckets[name]
		fmt.Fprintf(tabw, "%s\t%d\t%0.6f Gb\t\n", name, bucket.objects, float64(bucket.size)/1e9)
	}

	fmt.Fprintf(tabw, "%s\t%s\t%0.6f Gb\t\n", "TOTAL", "", float64(i.total)/1e9)

	tabw.Flush()

	if i.unattributed > 0 {
		fmt.Fprintf(w, "\n%d object(s) named no bucket and are not counted above.\n", i.unattributed)
	}
}
