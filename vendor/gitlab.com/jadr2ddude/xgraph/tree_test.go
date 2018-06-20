package xgraph

import (
	"fmt"
	"testing"
)

func BenchmarkTreeDeps(b *testing.B) {
	for _, depth := range []int{4, 8, 10} {
		g := denseValidGraph(depth, 8)
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			benchmarkTreeDeps(b, g, 8, false)
		})
		b.Run(fmt.Sprintf("%dTarjan", depth), func(b *testing.B) {
			benchmarkTreeDeps(b, g, 8, true)
		})
	}
}

func benchmarkTreeDeps(b *testing.B, g *Graph, w int, tarjan bool) {
	for i := 0; i < b.N; i++ {
		tb := &treeBuilder{
			forest: make(map[string]*jTree),
			g:      g,
		}

		for i := 0; i < w; i++ {
			tb.genTree(jobname(0, i))
		}

		if tarjan {
			tb.findCycles()
		} else {
			tb.findCyclesOld()
		}
	}
}

func denseValidGraph(layerCount, width int) *Graph {
	g := New()

	for layeri := 0; layeri < layerCount; layeri++ {
		var deps []string
		if layeri+1 < layerCount {
			for index := 0; index < width; index++ {
				deps = append(deps, jobname(layeri+1, index))
			}
		}

		for index := 0; index < width; index++ {
			g.AddJob(testJob(layeri, index, deps))
		}
	}

	return g
}

func jobname(layer, index int) string {
	return fmt.Sprintf("%d_%d", layer, index)
}

func testJob(layer, index int, deps []string) *BasicJob {
	return &BasicJob{
		JobName:     jobname(layer, index),
		RunCallback: func() error { return nil },
		Deps:        deps,
	}
}
