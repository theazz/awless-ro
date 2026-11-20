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
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/graph"
)

type PortScanner struct {
	inbounds   map[string][]*graph.FirewallRule
	applyingOn map[string][]string
	names      map[string]string
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
	p.applyingOn = make(map[string][]string)
	p.names = make(map[string]string)
	for _, sg := range sgroups {
		if name, _ := sg.Properties()[properties.Name].(string); name != "" {
			p.names[sg.Id()] = name
		}
		inbounds, ok := sg.Properties()[properties.InboundRules].([]*graph.FirewallRule)
		if !ok {
			continue
		}
		p.inbounds[sg.Id()] = inbounds
		res, err := g.ResourceRelations(sg, rdf.ApplyOn, false)
		if err != nil {
			return err
		}
		for _, r := range res {
			p.applyingOn[sg.Id()] = append(p.applyingOn[sg.Id()], r.String())
		}
		sort.Strings(p.applyingOn[sg.Id()])
	}

	return nil
}

func (p *PortScanner) Print(w io.Writer) {
	for _, id := range p.orderedGroups() {
		var targets string
		if len(p.applyingOn[id]) == 0 {
			targets = "nothing"
		} else {
			targets = strings.Join(p.applyingOn[id], ", ")
		}
		if name := p.names[id]; name != "" {
			fmt.Fprintf(w, "Securitygroup %s (%s), applying on %s:\n", name, id, targets)
		} else {
			fmt.Fprintf(w, "Securitygroup %s applying on %s:\n", id, targets)
		}
		for _, rule := range p.orderedRules(id) {
			fmt.Fprintf(w, "\t%s\n", p.ruleLine(rule))
		}
	}
}

// orderedGroups lists the groups open to the internet first, then by name: map
// iteration order printed the same account differently on every run.
func (p *PortScanner) orderedGroups() []string {
	ids := make([]string, 0, len(p.inbounds))
	for id := range p.inbounds {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		iOpen, jOpen := p.openToInternet(ids[i]), p.openToInternet(ids[j])
		if iOpen != jOpen {
			return iOpen
		}
		iName, jName := p.displayName(ids[i]), p.displayName(ids[j])
		if iName != jName {
			return iName < jName
		}
		return ids[i] < ids[j]
	})
	return ids
}

func (p *PortScanner) displayName(id string) string {
	if name := p.names[id]; name != "" {
		return name
	}
	return id
}

func (p *PortScanner) openToInternet(id string) bool {
	for _, rule := range p.inbounds[id] {
		if isOpenToInternet(rule) {
			return true
		}
	}
	return false
}

// orderedRules lists the broadest rule first, then by port and protocol.
func (p *PortScanner) orderedRules(id string) []*graph.FirewallRule {
	rules := append([]*graph.FirewallRule(nil), p.inbounds[id]...)
	sort.Slice(rules, func(i, j int) bool {
		a, b := rules[i], rules[j]
		if a.PortRange.Any != b.PortRange.Any {
			return a.PortRange.Any
		}
		if a.PortRange.FromPort != b.PortRange.FromPort {
			return a.PortRange.FromPort < b.PortRange.FromPort
		}
		if a.PortRange.ToPort != b.PortRange.ToPort {
			return a.PortRange.ToPort < b.PortRange.ToPort
		}
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		return p.ruleLine(a) < p.ruleLine(b)
	})
	return rules
}

func (p *PortScanner) ruleLine(rule *graph.FirewallRule) string {
	line := rulePorts(rule) + " from " + strings.Join(p.sourceLabels(rule), ", ")
	if isOpenToInternet(rule) {
		line += "  (open to the internet)"
	}
	return line
}

func rulePorts(rule *graph.FirewallRule) string {
	switch {
	case rule.PortRange.Any && rule.Protocol == "any":
		return "all ports/any protocol"
	case rule.PortRange.Any:
		return "all ports/" + rule.Protocol
	case rule.PortRange.FromPort == rule.PortRange.ToPort:
		return fmt.Sprintf("port %d/%s", rule.PortRange.FromPort, rule.Protocol)
	default:
		return fmt.Sprintf("ports %d-%d/%s", rule.PortRange.FromPort, rule.PortRange.ToPort, rule.Protocol)
	}
}

// sourceLabels names each source of a rule: CIDR ranges, security groups and
// prefix lists. Sorted, so two runs over the same graph read the same.
func (p *PortScanner) sourceLabels(rule *graph.FirewallRule) []string {
	var labels []string
	for _, cidr := range rule.IPRanges {
		labels = append(labels, cidr.String())
	}
	for _, source := range rule.Sources {
		switch {
		case strings.HasPrefix(source, "sg-"):
			if name := p.names[source]; name != "" {
				labels = append(labels, fmt.Sprintf("sg %s (%s)", name, source))
			} else {
				labels = append(labels, "sg "+source)
			}
		case strings.HasPrefix(source, "pl-"):
			labels = append(labels, "prefix list "+source)
		default:
			labels = append(labels, source)
		}
	}
	sort.Strings(labels)
	if len(labels) == 0 {
		return []string{"no source"}
	}
	return labels
}

// isOpenToInternet reports a rule whose source is a /0 range: 0.0.0.0/0 or ::/0.
// Comparing with 0.0.0.0 alone missed the IPv6 case.
func isOpenToInternet(rule *graph.FirewallRule) bool {
	for _, cidr := range rule.IPRanges {
		if ones, _ := cidr.Mask.Size(); ones == 0 {
			return true
		}
	}
	return false
}
