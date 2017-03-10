// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"hash/fnv"
	"strings"
	"bytes"
	"fmt"
)

type graphviz struct {
	ps []*gvnode
	b  bytes.Buffer
	h  map[string]uint32
}

type gvnode struct {
	project  string
	version  string
	children []string
}

func (g graphviz) New() *graphviz {
	ga := &graphviz{
		ps: []*gvnode{},
		h:  make(map[string]uint32),
	}
	return ga
}

func (g graphviz) output() bytes.Buffer {
	g.b.WriteString("digraph { node [shape=box]; ")

	for _, gvp := range g.ps {
		g.h[gvp.project] = gvp.hash()

		// Create node string
		g.b.WriteString(fmt.Sprintf("%d [label=\"%s\"];", gvp.hash(), gvp.label()))
	}

	// Store relations to avoid duplication
	rels := make(map[string]bool)

	// Create relations
	for _, dp := range g.ps {
		for _, bsc := range dp.children {
			for pr, hsh := range g.h {
				if strings.HasPrefix(bsc, pr) {
					r := fmt.Sprintf("%d -> %d", g.h[dp.project], hsh)

					if _, ex := rels[r]; !ex {
						g.b.WriteString(r + "; ")
						rels[r] = true
					}

				}
			}
		}
	}

	g.b.WriteString("}")

	return g.b
}

func (g *graphviz) createNode(p, v string, c []string) {
	pr := &gvnode{
		project:  p,
		version:  v,
		children: c,
	}

	g.ps = append(g.ps, pr)
}

func (dp gvnode) hash() uint32 {
	h := fnv.New32a()
	h.Write([]byte(dp.project))
	return h.Sum32()
}

func (dp gvnode) label() string {
	label := []string{dp.project}

	if dp.version != "" {
		label = append(label, dp.version)
	}

	return strings.Join(label, "\n")
}
