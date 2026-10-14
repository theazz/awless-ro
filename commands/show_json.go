package commands

import (
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/console"
)

// showOutput is `show --format json`: the resource as `list --format json` prints it,
// under "properties", and its relations as references rather than whole resources.
// Embedding them would make showing a region print the account; a reference is
// enough to ask for any of them with `show <id> --format json`.
type showOutput struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	// From the top of the lineage (usually the region) down to the direct parent,
	// the order the text output draws them in.
	Parents     []console.ResourceRef `json:"parents"`
	Children    []childRef            `json:"children"`
	AppliedOn   []console.ResourceRef `json:"appliedOn"`
	DependingOn []console.ResourceRef `json:"dependingOn"`
	// Only with --siblings, as in the text output, where they are otherwise cut
	// short. A pointer so that "asked for, none" is [] and "not asked for" is absent.
	Siblings *[]console.ResourceRef `json:"siblings,omitempty"`
}

type childRef struct {
	console.ResourceRef
	Children []childRef `json:"children,omitempty"`
}

func buildShowOutput(resource cloud.Resource, gph cloud.GraphAPI, withSiblings bool) (showOutput, error) {
	out := showOutput{Type: resource.Type(), Properties: console.JSONProperties(resource)}

	parents, err := gph.ResourceRelations(resource, rdf.ParentOf, true)
	if err != nil {
		return out, err
	}
	// Nearest first from the graph; reversed to read from the top down.
	out.Parents = make([]console.ResourceRef, 0, len(parents))
	for i := len(parents) - 1; i >= 0; i-- {
		out.Parents = append(out.Parents, console.RefTo(parents[i]))
	}

	if out.Children, err = childTree(resource, gph); err != nil {
		return out, err
	}

	appliedOn, err := gph.ResourceRelations(resource, rdf.ApplyOn, false)
	if err != nil {
		return out, err
	}
	out.AppliedOn = console.RefsTo(appliedOn)

	dependingOn, err := gph.ResourceRelations(resource, rdf.DependingOnRel, false)
	if err != nil {
		return out, err
	}
	out.DependingOn = console.RefsTo(dependingOn)

	if withSiblings {
		siblings, err := gph.ResourceSiblings(resource)
		if err != nil {
			return out, err
		}
		refs := console.RefsTo(siblings)
		out.Siblings = &refs
	}
	return out, nil
}

// childTree nests the descendants of resource the way the text output indents them.
// The traversal is depth first and reports each node's depth, so the node a child
// hangs under is the last one seen one level up.
func childTree(resource cloud.Resource, gph cloud.GraphAPI) ([]childRef, error) {
	root := &childRef{}
	path := []*childRef{root}
	err := gph.VisitRelations(resource, rdf.ChildrenOfRel, false, func(r cloud.Resource, depth int) error {
		if depth < 1 || depth > len(path) {
			return nil
		}
		parent := path[depth-1]
		parent.Children = append(parent.Children, childRef{ResourceRef: console.RefTo(r)})
		path = append(path[:depth], &parent.Children[len(parent.Children)-1])
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortChildren(root.Children)
	if root.Children == nil {
		return []childRef{}, nil
	}
	return root.Children, nil
}

func sortChildren(children []childRef) {
	console.SortRefs(children, func(i int) console.ResourceRef { return children[i].ResourceRef })
	for i := range children {
		sortChildren(children[i].Children)
	}
}
