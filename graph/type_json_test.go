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
	"net"
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
