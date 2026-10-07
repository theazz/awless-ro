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

package console

import (
	"bytes"
	"net"
	"testing"

	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// The CIDR fields of security groups and route tables (#27). JSON output used to
// render their netmasks as base64; every other format renders them through
// String(), and has to stay byte-identical.

func cidr(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// netmaskGraph holds one CIDR shape per rule and one rule or route per property.
// That is deliberate: a list is stored as one triple per element and comes back
// from the graph in map order, so a property holding two rules, or a rule holding
// two ranges, renders in a different order from run to run. Spreading the shapes
// over several resources keeps every literal below deterministic.
func netmaskGraph(t *testing.T) *graph.Graph {
	t.Helper()
	g := graph.NewGraph()
	err := g.AddResource(
		// The upstream report: tcp 9000-9003 from 10.0.0.0/16.
		resourcetest.SecurityGroup("sg-1").Prop(p.Name, "web").
			Prop(p.InboundRules, []*graph.FirewallRule{{PortRange: graph.PortRange{FromPort: 9000, ToPort: 9003}, Protocol: "tcp", IPRanges: []*net.IPNet{cidr(t, "10.0.0.0/16")}}}).
			Prop(p.OutboundRules, []*graph.FirewallRule{{PortRange: graph.PortRange{Any: true}, Protocol: "any", IPRanges: []*net.IPNet{cidr(t, "0.0.0.0/0")}}}).Build(),
		resourcetest.SecurityGroup("sg-2").Prop(p.Name, "v6").
			Prop(p.InboundRules, []*graph.FirewallRule{{PortRange: graph.PortRange{FromPort: 22, ToPort: 22}, Protocol: "tcp", IPRanges: []*net.IPNet{cidr(t, "2001:db8::/32")}}}).
			Prop(p.OutboundRules, []*graph.FirewallRule{{PortRange: graph.PortRange{Any: true}, Protocol: "tcp", IPRanges: []*net.IPNet{cidr(t, "::/0")}}}).Build(),
		// Source is another security group, not a CIDR.
		resourcetest.SecurityGroup("sg-3").Prop(p.Name, "app").
			Prop(p.InboundRules, []*graph.FirewallRule{{PortRange: graph.PortRange{FromPort: 8080, ToPort: 8080}, Protocol: "tcp", Sources: []string{"sg-src"}}}).Build(),

		resourcetest.RouteTable("rtb-1").Prop(p.Name, "local").
			Prop(p.Routes, []*graph.Route{{Destination: cidr(t, "10.0.0.0/16"), Targets: []*graph.RouteTarget{{Type: graph.GatewayTarget, Ref: "local"}}}}).Build(),
		resourcetest.RouteTable("rtb-2").Prop(p.Name, "public").
			Prop(p.Routes, []*graph.Route{{Destination: cidr(t, "0.0.0.0/0"), DestinationIPv6: cidr(t, "::/0"), Targets: []*graph.RouteTarget{{Type: graph.GatewayTarget, Ref: "igw-1"}}}}).Build(),
		resourcetest.RouteTable("rtb-3").Prop(p.Name, "v6").
			Prop(p.Routes, []*graph.Route{{DestinationIPv6: cidr(t, "2001:db8::/32"), Targets: []*graph.RouteTarget{{Type: graph.NetworkInterfaceTarget, Ref: "eni-1"}}}}).Build(),
		resourcetest.RouteTable("rtb-4").Prop(p.Name, "prefix").
			Prop(p.Routes, []*graph.Route{{DestinationPrefixListId: "pl-1", Targets: []*graph.RouteTarget{{Type: graph.InstanceTarget, Ref: "i-1"}}}}).Build(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// Spelled out rather than read from DefaultsColumnDefinitions, which another test
// in this package overwrites.
var netmaskColumns = map[string][]ColumnDefinition{
	"securitygroup": {
		StringColumnDefinition{Prop: p.ID},
		StringColumnDefinition{Prop: p.Name},
		FirewallRulesColumnDefinition{StringColumnDefinition: StringColumnDefinition{Prop: p.InboundRules, Friendly: "Inbound"}},
		FirewallRulesColumnDefinition{StringColumnDefinition: StringColumnDefinition{Prop: p.OutboundRules, Friendly: "Outbound"}},
	},
	"routetable": {
		StringColumnDefinition{Prop: p.ID},
		StringColumnDefinition{Prop: p.Name},
		RoutesColumnDefinition{StringColumnDefinition: StringColumnDefinition{Prop: p.Routes}},
	},
}

// pinTableWidths fixes the package-level wrapping knobs, which other tests in this
// package change without restoring.
func pinTableWidths(t *testing.T) {
	t.Helper()
	prevWrap, prevCol := autowrapMaxSize, tableColWidth
	autowrapMaxSize, tableColWidth = 35, 30
	t.Cleanup(func() { autowrapMaxSize, tableColWidth = prevWrap, prevCol })
}

func renderNetmask(t *testing.T, g *graph.Graph, rdfType string, opts ...optsFn) string {
	t.Helper()
	all := append([]optsFn{WithRdfType(rdfType), WithColumnDefinitions(netmaskColumns[rdfType])}, opts...)
	displayer, err := BuildOptions(all...).SetSource(g).Build()
	if err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	if err := displayer.Print(&w); err != nil {
		t.Fatal(err)
	}
	return w.String()
}

// TestNetmaskNonJSONFormatsUnchanged is the regression guard for the JSON fix: the
// literals were captured before it, and every non-JSON format must still produce
// them byte for byte.
func TestNetmaskNonJSONFormatsUnchanged(t *testing.T) {
	pinTableWidths(t)
	g := netmaskGraph(t)

	cases := []struct {
		name, rdfType string
		opts          []optsFn
		want          string
	}{
		{"securitygroup table", "securitygroup", []optsFn{WithFormat("table")}, sgTable},
		{"securitygroup csv", "securitygroup", []optsFn{WithFormat("csv")}, sgCSV},
		{"securitygroup tsv", "securitygroup", []optsFn{WithFormat("tsv")}, sgTSV},
		{"securitygroup porcelain", "securitygroup", []optsFn{WithFormat("porcelain")}, sgPorcelain},
		{"securitygroup ids", "securitygroup", []optsFn{WithFormat("table"), WithIDsOnly(true)}, "sg-1\nsg-2\nsg-3"},
		{"routetable table", "routetable", []optsFn{WithFormat("table")}, rtTable},
		{"routetable csv", "routetable", []optsFn{WithFormat("csv")}, rtCSV},
		{"routetable tsv", "routetable", []optsFn{WithFormat("tsv")}, rtTSV},
		{"routetable porcelain", "routetable", []optsFn{WithFormat("porcelain")}, rtPorcelain},
		{"routetable ids", "routetable", []optsFn{WithFormat("table"), WithIDsOnly(true)}, "rtb-1\nrtb-2\nrtb-3\nrtb-4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderNetmask(t, g, tc.rdfType, tc.opts...); got != tc.want {
				t.Errorf("output changed.\n--- want ---\n%s\n--- got ---\n%s\n--- got (quoted) ---\n%q", tc.want, got, got)
			}
		})
	}
}

// TestNetmaskFilterUnchanged pins --filter on the CIDR columns. Matching compares
// fmt.Sprint of the value, that is String(), never JSON.
func TestNetmaskFilterUnchanged(t *testing.T) {
	g := netmaskGraph(t)
	cases := []struct {
		rdfType, filter, want string
	}{
		{"securitygroup", "inbound=10.0.0.0/16", "ID,Name,Inbound,Outbound\nsg-1,web,[10.0.0.0/16](tcp:9000-9003) ,[0.0.0.0/0](any) \n"},
		{"securitygroup", "inboundrules=2001:db8::/32", "ID,Name,Inbound,Outbound\nsg-2,v6,[2001:db8::/32](tcp:22) ,[::/0](tcp:any) \n"},
		{"securitygroup", "outbound=0.0.0.0/0", "ID,Name,Inbound,Outbound\nsg-1,web,[10.0.0.0/16](tcp:9000-9003) ,[0.0.0.0/0](any) \n"},
		{"routetable", "routes=::/0", "ID,Name,Routes\nrtb-2,public,0.0.0.0/0+::/0->gw:igw-1 \n"},
		{"routetable", "routes=2001:db8", "ID,Name,Routes\nrtb-3,v6,2001:db8::/32->ni:eni-1 \n"},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			got := renderNetmask(t, g, tc.rdfType, WithFormat("csv"), WithFilters([]string{tc.filter}))
			if got != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// Captured from the code before the JSON fix. Quoted rather than raw because the
// cells carry trailing spaces and tabs.
const (
	sgTable = "| ID ▲ | NAME |            INBOUND            |     OUTBOUND      |\n" +
		"|------|------|-------------------------------|-------------------|\n" +
		"| sg-1 | web  | [10.0.0.0/16](tcp:9000-9003)  | [0.0.0.0/0](any)  |\n" +
		"| sg-2 | v6   | [2001:db8::/32](tcp:22)       | [::/0](tcp:any)   |\n" +
		"| sg-3 | app  | [sg-src](tcp:8080)            |                   |\n"
	sgCSV = "ID,Name,Inbound,Outbound\n" +
		"sg-1,web,[10.0.0.0/16](tcp:9000-9003) ,[0.0.0.0/0](any) \n" +
		"sg-2,v6,[2001:db8::/32](tcp:22) ,[::/0](tcp:any) \n" +
		"sg-3,app,[sg-src](tcp:8080) ,\n"
	sgTSV = "ID\tName\tInbound\tOutbound\n" +
		"sg-1\tweb\t[10.0.0.0/16](tcp:9000-9003) \t[0.0.0.0/0](any) \n" +
		"sg-2\tv6\t[2001:db8::/32](tcp:22) \t[::/0](tcp:any) \n" +
		"sg-3\tapp\t[sg-src](tcp:8080) \t\n"
	sgPorcelain = "sg-1\nweb\n" +
		"[PortRange:9000:9003; Protocol:tcp; IPRanges:[10.0.0.0/16]; Sources:[]]\n" +
		"[PortRange::; Protocol:any; IPRanges:[0.0.0.0/0]; Sources:[]]\n" +
		"sg-2\nv6\n" +
		"[PortRange:22:22; Protocol:tcp; IPRanges:[2001:db8::/32]; Sources:[]]\n" +
		"[PortRange::; Protocol:tcp; IPRanges:[::/0]; Sources:[]]\n" +
		"sg-3\napp\n" +
		"[PortRange:8080:8080; Protocol:tcp; IPRanges:[]; Sources:[sg-src]]"
	rtTable = "| ID ▲  |  NAME  |          ROUTES           |\n" +
		"|-------|--------|---------------------------|\n" +
		"| rtb-1 | local  | 10.0.0.0/16->gw:local     |\n" +
		"| rtb-2 | public | 0.0.0.0/0+::/0->gw:igw-1  |\n" +
		"| rtb-3 | v6     | 2001:db8::/32->ni:eni-1   |\n" +
		"| rtb-4 | prefix | ->inst:i-1                |\n"
	rtCSV = "ID,Name,Routes\n" +
		"rtb-1,local,10.0.0.0/16->gw:local \n" +
		"rtb-2,public,0.0.0.0/0+::/0->gw:igw-1 \n" +
		"rtb-3,v6,2001:db8::/32->ni:eni-1 \n" +
		"rtb-4,prefix,->inst:i-1 \n"
	rtTSV = "ID\tName\tRoutes\n" +
		"rtb-1\tlocal\t10.0.0.0/16->gw:local \n" +
		"rtb-2\tpublic\t0.0.0.0/0+::/0->gw:igw-1 \n" +
		"rtb-3\tv6\t2001:db8::/32->ni:eni-1 \n" +
		"rtb-4\tprefix\t->inst:i-1 \n"
	rtPorcelain = "rtb-1\nlocal\n" +
		"[Destination:10.0.0.0/16; DestinationIPv6:<nil>; DestinationPrefixListId:; Targets:[1|local|]]\n" +
		"rtb-2\npublic\n" +
		"[Destination:0.0.0.0/0; DestinationIPv6:::/0; DestinationPrefixListId:; Targets:[1|igw-1|]]\n" +
		"rtb-3\nv6\n" +
		"[Destination:<nil>; DestinationIPv6:2001:db8::/32; DestinationPrefixListId:; Targets:[4|eni-1|]]\n" +
		"rtb-4\nprefix\n" +
		"[Destination:<nil>; DestinationIPv6:<nil>; DestinationPrefixListId:pl-1; Targets:[2|i-1|]]"
)
