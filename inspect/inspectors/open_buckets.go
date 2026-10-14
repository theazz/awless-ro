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
	"strings"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/graph"
)

type OpenBuckets struct {
	openToAny     []string
	openToAnyAuth []string
}

func (*OpenBuckets) Name() string {
	return "open_buckets"
}

func (a *OpenBuckets) Inspect(g cloud.GraphAPI) error {
	buckets, err := g.Find(cloud.NewQuery(cloud.Bucket))
	if err != nil {
		return err
	}

	openToAuthUsers := make(map[string]bool)
	openToUsers := make(map[string]bool)

	for _, buck := range buckets {
		grants, ok := buck.Properties()["Grants"].([]*graph.Grant)
		if ok {
			for _, g := range grants {
				if strings.Contains(g.Grantee.GranteeID, "AllUsers") {
					openToUsers[fmt.Sprint(buck.Properties()["ID"])] = true
				}
				if strings.Contains(g.Grantee.GranteeID, "AuthenticatedUsers") {
					openToAuthUsers[fmt.Sprint(buck.Properties()["ID"])] = true
				}
			}
		}
	}

	for k := range openToUsers {
		a.openToAny = append(a.openToAny, k)
	}

	for k := range openToAuthUsers {
		a.openToAnyAuth = append(a.openToAnyAuth, k)
	}

	// Sorted, so that the same buckets print the same way from one run to the next.
	sort.Strings(a.openToAny)
	sort.Strings(a.openToAnyAuth)

	return nil
}

// OpenBucket is one row of `inspect -i open_buckets --format json`: a bucket and the
// group its ACL grants access to, AllUsers (anybody) or AuthenticatedUsers (anyone
// with an AWS account). A bucket open to both has a row for each.
type OpenBucket struct {
	Bucket  string `json:"bucket"`
	Grantee string `json:"grantee"`
}

func (a *OpenBuckets) Rows(io.Writer) interface{} {
	rows := []OpenBucket{}
	for _, b := range a.openToAny {
		rows = append(rows, OpenBucket{Bucket: b, Grantee: "AllUsers"})
	}
	for _, b := range a.openToAnyAuth {
		rows = append(rows, OpenBucket{Bucket: b, Grantee: "AuthenticatedUsers"})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Bucket < rows[j].Bucket })
	return rows
}

func (a *OpenBuckets) Print(w io.Writer) {
	if len(a.openToAny) > 0 {
		fmt.Fprintf(w, "Buckets open to anybody: %s\n", strings.Join(a.openToAny, ", "))
	}
	if len(a.openToAnyAuth) > 0 {
		fmt.Fprintf(w, "Buckets open to anyone with an AWS account: %s\n", strings.Join(a.openToAnyAuth, ", "))
	}

	if len(a.openToAnyAuth) == 0 && len(a.openToAny) == 0 {
		fmt.Fprintln(w, "none found")
	}
}
