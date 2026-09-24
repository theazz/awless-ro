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

package triplestore

import (
	"sync"
	"sync/atomic"
)

// Source is a mutable set of triples. Adding the same triple twice is a no-op:
// identity is the subject, predicate and object together.
//
// It is safe for concurrent use, which matters because sync fetches every AWS
// service in parallel into one graph.
type Source interface {
	Add(...Triple)
	Snapshot() RDFGraph
	CopyTriples() []Triple
}

// RDFGraph is an immutable, queryable snapshot of a Source.
//
// Queries are index lookups rather than scans, which is the whole reason snapshots
// exist: resolving a resource reads its properties one predicate at a time, and
// unmarshalling a graph of a few thousand resources does that tens of thousands of
// times.
type RDFGraph interface {
	Contains(Triple) bool
	Triples() []Triple
	Count() int
	WithSubject(s string) []Triple
	WithSubjPred(s, p string) []Triple
	WithPredObj(p string, o Object) []Triple
}

type source struct {
	mu      sync.RWMutex
	triples map[string]Triple

	// The snapshot is cached and only rebuilt after a write, because callers ask
	// for one far more often than they add anything: a single Fetch takes a
	// snapshot per resource type.
	latest  atomic.Value // RDFGraph
	changed atomic.Bool
}

func NewSource() Source {
	s := &source{triples: make(map[string]Triple)}
	s.latest.Store(newGraph(0))
	return s
}

func (s *source) Add(ts ...Triple) {
	if len(ts) == 0 {
		return
	}
	s.mu.Lock()
	for _, t := range ts {
		s.triples[t.key()] = t
	}
	s.mu.Unlock()
	s.changed.Store(true)
}

func (s *source) CopyTriples() []Triple {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Triple, 0, len(s.triples))
	for _, t := range s.triples {
		out = append(out, t.(*triple).clone())
	}
	return out
}

func (s *source) Snapshot() RDFGraph {
	if !s.changed.Load() {
		return s.latest.Load().(RDFGraph)
	}

	s.mu.RLock()
	g := newGraph(len(s.triples))
	for key, t := range s.triples {
		objKey := t.Object().key()
		sub, pred := t.Subject(), t.Predicate()

		g.bySubject[sub] = append(g.bySubject[sub], t)
		g.bySubjPred[sub+pred] = append(g.bySubjPred[sub+pred], t)
		g.byPredObj[pred+objKey] = append(g.byPredObj[pred+objKey], t)
		g.all[key] = t
	}
	s.mu.RUnlock()

	// Clearing the flag before storing would lose a concurrent write; clearing it
	// after means a write racing with this rebuild costs one redundant rebuild
	// next time, which is the harmless direction.
	s.latest.Store(g)
	s.changed.Store(false)

	return g
}

type graph struct {
	bySubject  map[string][]Triple
	bySubjPred map[string][]Triple
	byPredObj  map[string][]Triple
	all        map[string]Triple

	once   sync.Once
	unique []Triple
}

func newGraph(size int) *graph {
	return &graph{
		bySubject:  make(map[string][]Triple, size),
		bySubjPred: make(map[string][]Triple, size),
		byPredObj:  make(map[string][]Triple, size),
		all:        make(map[string]Triple, size),
	}
}

func (g *graph) Contains(t Triple) bool {
	if t == nil {
		return false
	}
	_, ok := g.all[t.key()]
	return ok
}

func (g *graph) Count() int { return len(g.all) }

func (g *graph) Triples() []Triple {
	g.once.Do(func() {
		g.unique = make([]Triple, 0, len(g.all))
		for _, t := range g.all {
			g.unique = append(g.unique, t)
		}
	})
	return g.unique
}

func (g *graph) WithSubject(s string) []Triple { return g.bySubject[s] }

func (g *graph) WithSubjPred(s, p string) []Triple { return g.bySubjPred[s+p] }

func (g *graph) WithPredObj(p string, o Object) []Triple {
	if o == nil {
		return nil
	}
	return g.byPredObj[p+o.key()]
}
