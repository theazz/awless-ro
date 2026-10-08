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
	"reflect"
	"sort"
	"strings"
	"testing"

	p "github.com/theazz/awless-ro/cloud/properties"
)

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// cidrFixture is one security group and one route table holding every CIDR shape
// that reaches output: IPv4, IPv6, both /0s, a rule with no CIDR at all, and a
// route whose destination is a prefix list.
func cidrFixture(t *testing.T) []*Resource {
	t.Helper()
	sg := InitResource("securitygroup", "sg-1")
	sg.Properties()[p.InboundRules] = []*FirewallRule{
		{PortRange: PortRange{FromPort: 443, ToPort: 443}, Protocol: "tcp", IPRanges: []*net.IPNet{mustCIDR(t, "10.0.0.0/16"), mustCIDR(t, "2001:db8::/32")}},
		{PortRange: PortRange{FromPort: 8080, ToPort: 8080}, Protocol: "tcp", Sources: []string{"sg-src"}},
	}
	sg.Properties()[p.OutboundRules] = []*FirewallRule{
		{PortRange: PortRange{Any: true}, Protocol: "any", IPRanges: []*net.IPNet{mustCIDR(t, "0.0.0.0/0"), mustCIDR(t, "::/0")}},
	}
	rt := InitResource("routetable", "rtb-1")
	rt.Properties()[p.Routes] = []*Route{
		{Destination: mustCIDR(t, "10.0.0.0/16"), Targets: []*RouteTarget{{Type: GatewayTarget, Ref: "local"}}},
		{DestinationIPv6: mustCIDR(t, "2001:db8::/32"), Targets: []*RouteTarget{{Type: NetworkInterfaceTarget, Ref: "eni-1"}}},
		{DestinationPrefixListId: "pl-1", Targets: []*RouteTarget{{Type: InstanceTarget, Ref: "i-1"}}},
	}
	return []*Resource{sg, rt}
}

func sortCIDRLists(res *Resource) {
	for _, key := range []string{p.InboundRules, p.OutboundRules} {
		if rules, ok := res.Properties()[key].([]*FirewallRule); ok {
			FirewallRules(rules).Sort()
		}
	}
	if routes, ok := res.Properties()[p.Routes].([]*Route); ok {
		Routes(routes).Sort()
	}
}

// TestCIDRTextRepresentationsUnchanged pins every non-JSON text form of the CIDR
// fields. Table, csv, tsv, porcelain and --filter all go through String(), and the
// .nt storage goes through fmt.Stringer on *net.IPNet. The JSON fix for #27 must
// leave every one of these byte-identical; the literals below were captured from
// the code before that fix.
func TestCIDRTextRepresentationsUnchanged(t *testing.T) {
	t.Run("FirewallRule.String", func(t *testing.T) {
		cases := []struct {
			ranges []*net.IPNet
			want   string
		}{
			{[]*net.IPNet{mustCIDR(t, "10.0.0.0/16")}, "PortRange:22:22; Protocol:tcp; IPRanges:[10.0.0.0/16]; Sources:[]"},
			{[]*net.IPNet{mustCIDR(t, "0.0.0.0/0")}, "PortRange:22:22; Protocol:tcp; IPRanges:[0.0.0.0/0]; Sources:[]"},
			{[]*net.IPNet{mustCIDR(t, "::/0")}, "PortRange:22:22; Protocol:tcp; IPRanges:[::/0]; Sources:[]"},
			{[]*net.IPNet{mustCIDR(t, "2001:db8::/32")}, "PortRange:22:22; Protocol:tcp; IPRanges:[2001:db8::/32]; Sources:[]"},
			{[]*net.IPNet{{IP: net.IPv4(10, 10, 0, 0), Mask: net.CIDRMask(16, 32)}}, "PortRange:22:22; Protocol:tcp; IPRanges:[10.10.0.0/16]; Sources:[]"},
		}
		for _, tc := range cases {
			r := &FirewallRule{PortRange: PortRange{FromPort: 22, ToPort: 22}, Protocol: "tcp", IPRanges: tc.ranges}
			if got := r.String(); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		}
	})

	t.Run("Route.String", func(t *testing.T) {
		cases := []struct {
			route *Route
			want  string
		}{
			{&Route{Destination: mustCIDR(t, "10.0.0.0/16")}, "Destination:10.0.0.0/16; DestinationIPv6:<nil>; DestinationPrefixListId:; Targets:[]"},
			{&Route{Destination: mustCIDR(t, "0.0.0.0/0"), DestinationIPv6: mustCIDR(t, "::/0")}, "Destination:0.0.0.0/0; DestinationIPv6:::/0; DestinationPrefixListId:; Targets:[]"},
			{&Route{DestinationIPv6: mustCIDR(t, "2001:db8::/32"), Targets: []*RouteTarget{{Type: GatewayTarget, Ref: "igw-1"}}}, "Destination:<nil>; DestinationIPv6:2001:db8::/32; DestinationPrefixListId:; Targets:[1|igw-1|]"},
			{&Route{Destination: &net.IPNet{IP: net.IPv4(10, 10, 0, 0), Mask: net.CIDRMask(16, 32)}}, "Destination:10.10.0.0/16; DestinationIPv6:<nil>; DestinationPrefixListId:; Targets:[]"},
		}
		for _, tc := range cases {
			if got := tc.route.String(); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		}
	})

	t.Run("N-Triples", func(t *testing.T) {
		g := NewGraph()
		if err := g.AddResource(cidrFixture(t)...); err != nil {
			t.Fatal(err)
		}
		serialised := g.MustMarshal()
		got := canonicalNT(t, serialised)
		const want = `<b0> <net:cidr> "10.0.0.0/16" .
<b0> <net:routeDestinationPrefixList> "" .
<b0> <net:routeTargets> "1|local|" .
<b0> <rdf:type> <net-owl:Route> .
<b1> <net:cidrv6> "2001:db8::/32" .
<b1> <net:routeDestinationPrefixList> "" .
<b1> <net:routeTargets> "4|eni-1|" .
<b1> <rdf:type> <net-owl:Route> .
<b2> <net:routeDestinationPrefixList> "pl-1" .
<b2> <net:routeTargets> "2|i-1|" .
<b2> <rdf:type> <net-owl:Route> .
<b3> <cloud:source> "sg-src" .
<b3> <net:portRange> "8080:8080" .
<b3> <net:protocol> "tcp" .
<b3> <rdf:type> <net-owl:FirewallRule> .
<b4> <net:cidr> "10.0.0.0/16" .
<b4> <net:cidr> "2001:db8::/32" .
<b4> <net:portRange> "443:443" .
<b4> <net:protocol> "tcp" .
<b4> <rdf:type> <net-owl:FirewallRule> .
<b5> <net:cidr> "0.0.0.0/0" .
<b5> <net:cidr> "::/0" .
<b5> <net:portRange> ":" .
<b5> <net:protocol> "any" .
<b5> <rdf:type> <net-owl:FirewallRule> .
<rtb-1> <cloud:id> "rtb-1" .
<rtb-1> <net:routes> <b0> .
<rtb-1> <net:routes> <b1> .
<rtb-1> <net:routes> <b2> .
<rtb-1> <rdf:type> <cloud-owl:Routetable> .
<sg-1> <cloud:id> "sg-1" .
<sg-1> <net:inboundRules> <b3> .
<sg-1> <net:inboundRules> <b4> .
<sg-1> <net:outboundRules> <b5> .
<sg-1> <rdf:type> <cloud-owl:Securitygroup> .
`
		if got != want {
			t.Errorf("serialised form changed.\n--- want ---\n%s\n--- got ---\n%s", want, got)
		}
		for _, line := range []string{`<net:cidr> "2001:db8::/32" .`, `<net:cidrv6> "2001:db8::/32" .`} {
			if !strings.Contains(got, line) {
				t.Errorf("missing %s", line)
			}
		}

		reloaded := NewGraph()
		if err := reloaded.UnmarshalFromReaders(strings.NewReader(serialised)); err != nil {
			t.Fatal(err)
		}
		for _, want := range cidrFixture(t) {
			got, err := reloaded.GetResource(want.Type(), want.Id())
			if err != nil {
				t.Fatal(err)
			}
			// Triples carry no order, so neither do the CIDRs inside a rule once
			// reloaded. Sort both sides the way resource_test.go does.
			sortCIDRLists(want)
			sortCIDRLists(got)
			compareProperties(t, want, got)
		}
	})
}

// Method-less mirrors: encoding these is exactly what --format json did before #27.
type legacyRule FirewallRule
type legacyRoute Route

type firewallRuleJSONCase struct {
	name  string
	rule  *FirewallRule
	want  string
	cidrs []string // expected IPRanges, in order; "" stands for a JSON null
}

func firewallRuleJSONCases(t *testing.T) []firewallRuleJSONCase {
	port := func(n int64) PortRange { return PortRange{FromPort: n, ToPort: n} }
	return []firewallRuleJSONCase{
		{
			name:  "ipv4 (upstream report)",
			rule:  &FirewallRule{PortRange: PortRange{FromPort: 9000, ToPort: 9003}, Protocol: "tcp", IPRanges: []*net.IPNet{mustCIDR(t, "10.0.0.0/16")}},
			want:  `{"PortRange":{"FromPort":9000,"ToPort":9003,"Any":false},"Protocol":"tcp","IPRanges":["10.0.0.0/16"],"Sources":null}`,
			cidrs: []string{"10.0.0.0/16"},
		},
		{
			name:  "ipv4 any",
			rule:  &FirewallRule{PortRange: port(443), Protocol: "tcp", IPRanges: []*net.IPNet{mustCIDR(t, "0.0.0.0/0")}},
			want:  `{"PortRange":{"FromPort":443,"ToPort":443,"Any":false},"Protocol":"tcp","IPRanges":["0.0.0.0/0"],"Sources":null}`,
			cidrs: []string{"0.0.0.0/0"},
		},
		{
			name:  "ipv6 any",
			rule:  &FirewallRule{PortRange: port(443), Protocol: "tcp", IPRanges: []*net.IPNet{mustCIDR(t, "::/0")}},
			want:  `{"PortRange":{"FromPort":443,"ToPort":443,"Any":false},"Protocol":"tcp","IPRanges":["::/0"],"Sources":null}`,
			cidrs: []string{"::/0"},
		},
		{
			name:  "ipv6",
			rule:  &FirewallRule{PortRange: port(22), Protocol: "tcp", IPRanges: []*net.IPNet{mustCIDR(t, "2001:db8::/32")}},
			want:  `{"PortRange":{"FromPort":22,"ToPort":22,"Any":false},"Protocol":"tcp","IPRanges":["2001:db8::/32"],"Sources":null}`,
			cidrs: []string{"2001:db8::/32"},
		},
		{
			name: "mixed, order preserved",
			rule: &FirewallRule{PortRange: port(443), Protocol: "tcp", IPRanges: []*net.IPNet{
				mustCIDR(t, "2001:db8::/32"), mustCIDR(t, "10.0.0.0/16"), mustCIDR(t, "0.0.0.0/0"), mustCIDR(t, "::/0"),
			}},
			want:  `{"PortRange":{"FromPort":443,"ToPort":443,"Any":false},"Protocol":"tcp","IPRanges":["2001:db8::/32","10.0.0.0/16","0.0.0.0/0","::/0"],"Sources":null}`,
			cidrs: []string{"2001:db8::/32", "10.0.0.0/16", "0.0.0.0/0", "::/0"},
		},
		{
			name:  "16-byte ipv4 address",
			rule:  &FirewallRule{PortRange: port(22), Protocol: "tcp", IPRanges: []*net.IPNet{{IP: net.IPv4(10, 10, 0, 0), Mask: net.CIDRMask(16, 32)}}},
			want:  `{"PortRange":{"FromPort":22,"ToPort":22,"Any":false},"Protocol":"tcp","IPRanges":["10.10.0.0/16"],"Sources":null}`,
			cidrs: []string{"10.10.0.0/16"},
		},
		{
			name: "source is a security group",
			rule: &FirewallRule{PortRange: port(8080), Protocol: "tcp", Sources: []string{"sg-src"}},
			want: `{"PortRange":{"FromPort":8080,"ToPort":8080,"Any":false},"Protocol":"tcp","IPRanges":null,"Sources":["sg-src"]}`,
		},
		{
			name:  "empty ranges",
			rule:  &FirewallRule{PortRange: port(80), Protocol: "udp", IPRanges: []*net.IPNet{}},
			want:  `{"PortRange":{"FromPort":80,"ToPort":80,"Any":false},"Protocol":"udp","IPRanges":[],"Sources":null}`,
			cidrs: []string{},
		},
		{
			name:  "nil range",
			rule:  &FirewallRule{PortRange: port(80), Protocol: "udp", IPRanges: []*net.IPNet{nil}},
			want:  `{"PortRange":{"FromPort":80,"ToPort":80,"Any":false},"Protocol":"udp","IPRanges":[null],"Sources":null}`,
			cidrs: []string{""},
		},
		{
			name:  "any port, any protocol",
			rule:  &FirewallRule{PortRange: PortRange{Any: true}, Protocol: "any", IPRanges: []*net.IPNet{mustCIDR(t, "0.0.0.0/0"), mustCIDR(t, "::/0")}},
			want:  `{"PortRange":{"FromPort":0,"ToPort":0,"Any":true},"Protocol":"any","IPRanges":["0.0.0.0/0","::/0"],"Sources":null}`,
			cidrs: []string{"0.0.0.0/0", "::/0"},
		},
	}
}

type routeJSONCase struct {
	name        string
	route       *Route
	want        string
	dest, dest6 string // "" stands for a JSON null
}

func routeJSONCases(t *testing.T) []routeJSONCase {
	gw := []*RouteTarget{{Type: GatewayTarget, Ref: "igw-1", Owner: "owner-1"}}
	return []routeJSONCase{
		{
			name:  "ipv4 only",
			route: &Route{Destination: mustCIDR(t, "10.0.0.0/16"), Targets: []*RouteTarget{{Type: GatewayTarget, Ref: "local"}}},
			want:  `{"Destination":"10.0.0.0/16","DestinationIPv6":null,"DestinationPrefixListId":"","Targets":[{"Type":1,"Ref":"local","Owner":""}]}`,
			dest:  "10.0.0.0/16",
		},
		{
			name:  "ipv6 only",
			route: &Route{DestinationIPv6: mustCIDR(t, "2001:db8::/32"), Targets: []*RouteTarget{{Type: NetworkInterfaceTarget, Ref: "eni-1"}}},
			want:  `{"Destination":null,"DestinationIPv6":"2001:db8::/32","DestinationPrefixListId":"","Targets":[{"Type":4,"Ref":"eni-1","Owner":""}]}`,
			dest6: "2001:db8::/32",
		},
		{
			name:  "both default routes",
			route: &Route{Destination: mustCIDR(t, "0.0.0.0/0"), DestinationIPv6: mustCIDR(t, "::/0"), Targets: gw},
			want:  `{"Destination":"0.0.0.0/0","DestinationIPv6":"::/0","DestinationPrefixListId":"","Targets":[{"Type":1,"Ref":"igw-1","Owner":"owner-1"}]}`,
			dest:  "0.0.0.0/0",
			dest6: "::/0",
		},
		{
			name:  "prefix list only, no targets",
			route: &Route{DestinationPrefixListId: "pl-1"},
			want:  `{"Destination":null,"DestinationIPv6":null,"DestinationPrefixListId":"pl-1","Targets":null}`,
		},
		{
			name:  "16-byte ipv4 address",
			route: &Route{Destination: &net.IPNet{IP: net.IPv4(10, 10, 0, 0), Mask: net.CIDRMask(16, 32)}, Targets: gw},
			want:  `{"Destination":"10.10.0.0/16","DestinationIPv6":null,"DestinationPrefixListId":"","Targets":[{"Type":1,"Ref":"igw-1","Owner":"owner-1"}]}`,
			dest:  "10.10.0.0/16",
		},
	}
}

func keysOf(m map[string]any) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// assertCIDR checks a decoded JSON value is the CIDR text want, or null for "".
func assertCIDR(t *testing.T, got any, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Errorf("got %#v, want null", got)
		}
		return
	}
	s, ok := got.(string)
	if !ok {
		t.Errorf("got %#v (%T), want the string %q", got, got, want)
		return
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Errorf("%q is not a CIDR: %s", s, err)
		return
	}
	if n.String() != want {
		t.Errorf("got %q, want %q", n.String(), want)
	}
}

func TestFirewallRuleMarshalJSON(t *testing.T) {
	for _, tc := range firewallRuleJSONCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.rule)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Fatalf("got  %s\nwant %s", b, tc.want)
			}
			if strings.Contains(string(b), "Mask") {
				t.Errorf("netmask leaked: %s", b)
			}

			var decoded map[string]any
			if err := json.Unmarshal(b, &decoded); err != nil {
				t.Fatal(err)
			}
			if got, want := keysOf(decoded), []string{"IPRanges", "PortRange", "Protocol", "Sources"}; !reflect.DeepEqual(got, want) {
				t.Errorf("keys %v, want %v", got, want)
			}
			if tc.cidrs == nil {
				if decoded["IPRanges"] != nil {
					t.Errorf("IPRanges %#v, want null", decoded["IPRanges"])
				}
				return
			}
			ranges, ok := decoded["IPRanges"].([]any)
			if !ok || len(ranges) != len(tc.cidrs) {
				t.Fatalf("IPRanges %#v, want %d elements", decoded["IPRanges"], len(tc.cidrs))
			}
			for i, want := range tc.cidrs {
				assertCIDR(t, ranges[i], want)
			}
		})
	}
}

func TestRouteMarshalJSON(t *testing.T) {
	for _, tc := range routeJSONCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.route)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Fatalf("got  %s\nwant %s", b, tc.want)
			}
			if strings.Contains(string(b), "Mask") {
				t.Errorf("netmask leaked: %s", b)
			}

			var decoded map[string]any
			if err := json.Unmarshal(b, &decoded); err != nil {
				t.Fatal(err)
			}
			if got, want := keysOf(decoded), []string{"Destination", "DestinationIPv6", "DestinationPrefixListId", "Targets"}; !reflect.DeepEqual(got, want) {
				t.Errorf("keys %v, want %v", got, want)
			}
			assertCIDR(t, decoded["Destination"], tc.dest)
			assertCIDR(t, decoded["DestinationIPv6"], tc.dest6)
		})
	}
}

func decodeObject(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestCIDRJSONKeepsOtherKeys proves the fix changes nothing but the CIDR values:
// every other key decodes to exactly what the default struct encoding produced.
func TestCIDRJSONKeepsOtherKeys(t *testing.T) {
	legacy, err := json.Marshal((*legacyRule)(&FirewallRule{PortRange: PortRange{FromPort: 9000, ToPort: 9003}, Protocol: "tcp", IPRanges: []*net.IPNet{mustCIDR(t, "10.0.0.0/16")}}))
	if err != nil {
		t.Fatal(err)
	}
	// The bug as reported upstream (wallix/awless#215), kept here as the "before".
	if want := `{"PortRange":{"FromPort":9000,"ToPort":9003,"Any":false},"Protocol":"tcp","IPRanges":[{"IP":"10.0.0.0","Mask":"//8AAA=="}],"Sources":null}`; string(legacy) != want {
		t.Fatalf("default encoding\ngot  %s\nwant %s", legacy, want)
	}

	for _, tc := range firewallRuleJSONCases(t) {
		t.Run("rule/"+tc.name, func(t *testing.T) {
			now, before := decodeObject(t, tc.rule), decodeObject(t, (*legacyRule)(tc.rule))
			if !reflect.DeepEqual(keysOf(now), keysOf(before)) {
				t.Fatalf("keys %v, before %v", keysOf(now), keysOf(before))
			}
			for k := range before {
				if k != "IPRanges" && !reflect.DeepEqual(now[k], before[k]) {
					t.Errorf("%s: %#v, before %#v", k, now[k], before[k])
				}
			}
			b, _ := json.Marshal((*legacyRule)(tc.rule))
			if hasRange := len(tc.cidrs) > 0 && tc.cidrs[0] != ""; hasRange != strings.Contains(string(b), `"Mask"`) {
				t.Errorf("default encoding %s: expected a Mask exactly when there is a range", b)
			}
		})
	}
	for _, tc := range routeJSONCases(t) {
		t.Run("route/"+tc.name, func(t *testing.T) {
			now, before := decodeObject(t, tc.route), decodeObject(t, (*legacyRoute)(tc.route))
			if !reflect.DeepEqual(keysOf(now), keysOf(before)) {
				t.Fatalf("keys %v, before %v", keysOf(now), keysOf(before))
			}
			for k := range before {
				if k != "Destination" && k != "DestinationIPv6" && !reflect.DeepEqual(now[k], before[k]) {
					t.Errorf("%s: %#v, before %#v", k, now[k], before[k])
				}
			}
			b, _ := json.Marshal((*legacyRoute)(tc.route))
			if hasDest := tc.dest != "" || tc.dest6 != ""; hasDest != strings.Contains(string(b), `"Mask"`) {
				t.Errorf("default encoding %s: expected a Mask exactly when there is a destination", b)
			}
		})
	}
}

func TestCIDRJSONNilElements(t *testing.T) {
	for _, v := range []any{[]*FirewallRule{nil}, []*Route{nil}} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "[null]" {
			t.Errorf("%T: got %s, want [null]", v, b)
		}
	}
}
