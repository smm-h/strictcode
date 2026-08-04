package checks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/vocab"
)

// checkImportCycles: Tarjan SCC over the module imports projection per
// member; SCCs of size >= 2 only (lesson 21, self-loops ignored). Never
// runs on languages whose matrix cell is n/a (lesson 20: Go).
func checkImportCycles(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, lang := range vocab.Langs {
		if langNA("import-cycles", lang) {
			continue
		}
		for _, m := range ctx.View.WS.Members {
			out = append(out, cyclesFor(ctx, lang, m.Name)...)
		}
	}
	return out
}

func cyclesFor(ctx *Context, lang vocab.Lang, member string) []findings.Finding {
	lm := langMember{lang, member}
	modules := ctx.View.Modules[lm]
	if len(modules) == 0 {
		return nil
	}

	// Distinct-pair adjacency, self-loops dropped.
	adj := map[string][]string{}
	seen := map[[2]string]bool{}
	for src, edges := range ctx.View.ModuleImports[lm] {
		for _, e := range edges {
			if e.Dst == src || seen[[2]string{src, e.Dst}] {
				continue
			}
			seen[[2]string{src, e.Dst}] = true
			adj[src] = append(adj[src], e.Dst)
		}
	}
	for _, dsts := range adj {
		sort.Strings(dsts)
	}

	sccs := tarjan(sortedKeys(modules), adj)

	var out []findings.Finding
	for _, scc := range sccs {
		if len(scc) < 2 {
			continue
		}
		sort.Strings(scc)
		if ctx.suppressedModuleSet("import-cycles", scc) {
			continue
		}
		first := modules[scc[0]]
		out = append(out, ctx.finding("import-cycles",
			first.ID.String(), vocab.NodeKindModule, first.Path, 0,
			fmt.Sprintf("import cycle among modules: %s", strings.Join(scc, ", "))))
	}
	return out
}

// tarjan computes strongly connected components over the given nodes.
func tarjan(nodes []string, adj map[string][]string) [][]string {
	index := map[string]int{}
	lowlink := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	next := 0
	var sccs [][]string

	var strongconnect func(v string)
	strongconnect = func(v string) {
		index[v] = next
		lowlink[v] = next
		next++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := index[w]; !visited {
				strongconnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if index[w] < lowlink[v] {
					lowlink[v] = index[w]
				}
			}
		}

		if lowlink[v] == index[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}

	for _, v := range nodes {
		if _, visited := index[v]; !visited {
			strongconnect(v)
		}
	}
	return sccs
}
