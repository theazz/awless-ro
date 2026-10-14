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
	"net"
	"sort"
	"strings"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/console"
	"github.com/theazz/awless-ro/graph"
)

type PortScanner struct {
	inbounds   map[string][]*graph.FirewallRule
	applyingOn map[string][]cloud.Resource
}

func (p *PortScanner) Name() string {
	return "port_scanner"
}

func (p *PortScanner) Inspect(g cloud.GraphAPI) error {
	sgroups, err := g.Find(cloud.NewQuery(cloud.SecurityGroup))
	if err != nil {
		return err
	}

	p.inbounds = make(map[string][]*graph.FirewallRule)
	p.applyingOn = make(map[string][]cloud.Resource)
	for _, sg := range sgroups {
		rules := sg.Properties()["InboundRules"]
		switch rules.(type) {
		case []*graph.FirewallRule:
			p.inbounds[sg.Id()] = rules.([]*graph.FirewallRule)
			res, err := g.ResourceRelations(sg, rdf.ApplyOn, false)
			if err != nil {
				return err
			}
			p.applyingOn[sg.Id()] = append(p.applyingOn[sg.Id()], res...)

		}
	}

	return nil
}

var allLocalIPs = net.ParseIP("0.0.0.0")

// OpenPorts is one row of `inspect -i port_scanner --format json`: a security group,
// what it applies on, and its inbound rules as `list securitygroups --format json`
// prints them.
type OpenPorts struct {
	SecurityGroup string                `json:"securityGroup"`
	AppliesOn     []console.ResourceRef `json:"appliesOn"`
	InboundRules  []*graph.FirewallRule `json:"inboundRules"`
}

func (p *PortScanner) Rows(io.Writer) interface{} {
	rows := []OpenPorts{}
	for _, sg := range p.securityGroups() {
		rules := p.inbounds[sg]
		if rules == nil {
			rules = []*graph.FirewallRule{}
		}
		rows = append(rows, OpenPorts{SecurityGroup: sg, AppliesOn: console.RefsTo(p.applyingOn[sg]), InboundRules: rules})
	}
	return rows
}

// securityGroups is the ids in order, so that two runs print the same report.
func (p *PortScanner) securityGroups() []string {
	ids := make([]string, 0, len(p.inbounds))
	for sg := range p.inbounds {
		ids = append(ids, sg)
	}
	sort.Strings(ids)
	return ids
}

func (p *PortScanner) Print(w io.Writer) {
	for _, sg := range p.securityGroups() {
		inbounds := p.inbounds[sg]
		var targets string
		if len(p.applyingOn[sg]) == 0 {
			targets = "nothing"
		} else {
			names := make([]string, 0, len(p.applyingOn[sg]))
			for _, r := range p.applyingOn[sg] {
				names = append(names, r.String())
			}
			targets = strings.Join(names, ", ")
		}
		fmt.Fprintf(w, "Securitygroup %s applying on %s: \n", sg, targets)

		var allPermissive bool

		for _, inbound := range inbounds {
			if portRange, prot := inbound.PortRange, inbound.Protocol; portRange.Any == true && prot == "any" {
				var allIps bool
				for _, n := range inbound.IPRanges {
					if n.IP.Equal(allLocalIPs) {
						allIps = true
					}
				}
				if allIps {
					fmt.Fprintf(w, "\tall ports via any protocol for all IPs\n")
				} else {
					fmt.Fprintf(w, "\tall ports via any protocol for IPs: %s\n", inbound.IPRanges)
				}

				allPermissive = true
			}
		}

		if !allPermissive {
			for _, inbound := range inbounds {
				if portRange, prot := inbound.PortRange, inbound.Protocol; prot != "any" {
					if from, to := portRange.FromPort, portRange.ToPort; from == to {
						fmt.Fprintf(w, "\tport %d via %s\n", from, prot)
					} else {
						fmt.Fprintf(w, "\tports %d-%d via %s\n", from, to, prot)
					}
				}
			}
		}
	}
}
