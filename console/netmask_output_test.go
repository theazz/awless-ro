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
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
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

// assertCIDRString fails unless v is a string net.ParseCIDR accepts.
func assertCIDRString(t *testing.T, where string, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Errorf("%s: %#v (%T) is not a string", where, v, v)
		return
	}
	if _, _, err := net.ParseCIDR(s); err != nil {
		t.Errorf("%s: %q is not a CIDR: %s", where, s, err)
	}
}

// walkCIDRs checks every CIDR-bearing value in one decoded resource.
func walkCIDRs(t *testing.T, res map[string]any) {
	t.Helper()
	id := res["ID"]
	for _, key := range []string{p.InboundRules, p.OutboundRules} {
		rules, _ := res[key].([]any)
		for i, r := range rules {
			ranges, _ := r.(map[string]any)["IPRanges"].([]any)
			for j, v := range ranges {
				assertCIDRString(t, fmt.Sprintf("%v %s[%d].IPRanges[%d]", id, key, i, j), v)
			}
		}
	}
	routes, _ := res[p.Routes].([]any)
	for i, r := range routes {
		for _, key := range []string{"Destination", "DestinationIPv6"} {
			if v := r.(map[string]any)[key]; v != nil {
				assertCIDRString(t, fmt.Sprintf("%v Routes[%d].%s", id, i, key), v)
			}
		}
	}
}

func TestNetmaskJSON(t *testing.T) {
	g := netmaskGraph(t)
	cases := []struct {
		rdfType, want string
	}{
		{"securitygroup", `[
		 {"ID":"sg-1","Name":"web",
		  "InboundRules":[{"PortRange":{"FromPort":9000,"ToPort":9003,"Any":false},"Protocol":"tcp","IPRanges":["10.0.0.0/16"],"Sources":null}],
		  "OutboundRules":[{"PortRange":{"FromPort":0,"ToPort":0,"Any":true},"Protocol":"any","IPRanges":["0.0.0.0/0"],"Sources":null}]},
		 {"ID":"sg-2","Name":"v6",
		  "InboundRules":[{"PortRange":{"FromPort":22,"ToPort":22,"Any":false},"Protocol":"tcp","IPRanges":["2001:db8::/32"],"Sources":null}],
		  "OutboundRules":[{"PortRange":{"FromPort":0,"ToPort":0,"Any":true},"Protocol":"tcp","IPRanges":["::/0"],"Sources":null}]},
		 {"ID":"sg-3","Name":"app",
		  "InboundRules":[{"PortRange":{"FromPort":8080,"ToPort":8080,"Any":false},"Protocol":"tcp","IPRanges":null,"Sources":["sg-src"]}]}
		]`},
		{"routetable", `[
		 {"ID":"rtb-1","Name":"local","Routes":[{"Destination":"10.0.0.0/16","DestinationIPv6":null,"DestinationPrefixListId":"","Targets":[{"Type":1,"Ref":"local","Owner":""}]}]},
		 {"ID":"rtb-2","Name":"public","Routes":[{"Destination":"0.0.0.0/0","DestinationIPv6":"::/0","DestinationPrefixListId":"","Targets":[{"Type":1,"Ref":"igw-1","Owner":""}]}]},
		 {"ID":"rtb-3","Name":"v6","Routes":[{"Destination":null,"DestinationIPv6":"2001:db8::/32","DestinationPrefixListId":"","Targets":[{"Type":4,"Ref":"eni-1","Owner":""}]}]},
		 {"ID":"rtb-4","Name":"prefix","Routes":[{"Destination":null,"DestinationIPv6":null,"DestinationPrefixListId":"pl-1","Targets":[{"Type":2,"Ref":"i-1","Owner":""}]}]}
		]`},
	}
	for _, tc := range cases {
		t.Run(tc.rdfType, func(t *testing.T) {
			out := renderNetmask(t, g, tc.rdfType, WithFormat("json"))
			compareJSON(t, out, tc.want)
			if strings.Contains(out, "Mask") {
				t.Errorf("netmask leaked:\n%s", out)
			}
			var decoded []map[string]any
			if err := json.Unmarshal([]byte(out), &decoded); err != nil {
				t.Fatal(err)
			}
			for _, res := range decoded {
				walkCIDRs(t, res)
			}
		})
	}
}

// TestNetmaskMultiResourceJSON covers the other JSON path: a whole service
// listed at once (`list infra --format json`).
func TestNetmaskMultiResourceJSON(t *testing.T) {
	prev := DefaultsColumnDefinitions
	DefaultsColumnDefinitions = map[string][]ColumnDefinition{
		"securitygroup": netmaskColumns["securitygroup"],
		"routetable":    netmaskColumns["routetable"],
	}
	t.Cleanup(func() { DefaultsColumnDefinitions = prev })

	displayer, err := BuildOptions(WithFormat("json")).SetSource(netmaskGraph(t)).Build()
	if err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	if err := displayer.Print(&w); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.String(), "Mask") {
		t.Errorf("netmask leaked:\n%s", w.String())
	}
	var decoded map[string][]map[string]any
	if err := json.Unmarshal(w.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]map[string]any)
	for _, group := range []string{"securitygroups", "routetables"} {
		if len(decoded[group]) == 0 {
			t.Fatalf("no %s in %s", group, w.String())
		}
		for _, res := range decoded[group] {
			walkCIDRs(t, res)
			byID[res["ID"].(string)] = res
		}
	}

	first := func(id, key string) map[string]any {
		return byID[id][key].([]any)[0].(map[string]any)
	}
	if got := first("sg-1", p.InboundRules)["IPRanges"]; !reflect.DeepEqual(got, []any{"10.0.0.0/16"}) {
		t.Errorf("sg-1 inbound IPRanges = %#v", got)
	}
	if got := first("sg-2", p.OutboundRules)["IPRanges"]; !reflect.DeepEqual(got, []any{"::/0"}) {
		t.Errorf("sg-2 outbound IPRanges = %#v", got)
	}
	if r := first("rtb-2", p.Routes); r["Destination"] != "0.0.0.0/0" || r["DestinationIPv6"] != "::/0" {
		t.Errorf("rtb-2 route = %#v", r)
	}
	if r := first("rtb-3", p.Routes); r["Destination"] != nil || r["DestinationIPv6"] != "2001:db8::/32" {
		t.Errorf("rtb-3 route = %#v", r)
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
