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

package graph

import (
	"encoding/json"
	"net"
)

// JSON encoding of the types holding a *net.IPNet (#27, upstream wallix/awless#215).
//
// net.IPMask is a []byte, so the default encoding of a net.IPNet is an object
// whose mask is base64: {"IP":"10.0.0.0","Mask":"//8AAA=="}. These methods write
// the CIDR text instead, "10.0.0.0/16", which is IPNet.String() and therefore the
// same text the table and the .nt storage already use.
//
// Only encoding/json sees these methods. The .nt storage, table/csv/tsv,
// porcelain and --filter all go through String(), and none of that changes. Key
// names and order are those of the default struct encoding; only the value of the
// CIDR keys goes from an object to a string, or null when the pointer is nil.
//
// There is no UnmarshalJSON: nothing decodes these types from JSON.

// cidrText is the CIDR form of n, or nil, which encodes as null, for a nil n.
func cidrText(n *net.IPNet) *string {
	if n == nil {
		return nil
	}
	s := n.String()
	return &s
}

func (r *FirewallRule) MarshalJSON() ([]byte, error) {
	var ranges []*string
	if r.IPRanges != nil {
		ranges = make([]*string, 0, len(r.IPRanges))
		for _, n := range r.IPRanges {
			ranges = append(ranges, cidrText(n))
		}
	}
	return json.Marshal(struct {
		PortRange PortRange
		Protocol  string
		IPRanges  []*string
		Sources   []string
	}{r.PortRange, r.Protocol, ranges, r.Sources})
}

func (r *Route) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Destination             *string
		DestinationIPv6         *string
		DestinationPrefixListId string
		Targets                 []*RouteTarget
	}{cidrText(r.Destination), cidrText(r.DestinationIPv6), r.DestinationPrefixListId, r.Targets})
}
