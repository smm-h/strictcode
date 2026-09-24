package main

// Experiment-only mode: mirror gotreesitter's documented Python wrapper
// removal on the C tree (statement children of module/block: an
// expression_statement or _simple_statements whose single child is named is
// replaced by that child), then compare. Remaining mismatches are divergences
// the documented normalization does not explain.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	gts "github.com/odvcencio/gotreesitter"
	gtsgrammars "github.com/odvcencio/gotreesitter/grammars"
	cgo "github.com/tree-sitter/go-tree-sitter"
	cgopython "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

func unwrapCGo(n *cgo.Node) *cgo.Node {
	for {
		k := n.Kind()
		if (k == "expression_statement" || k == "_simple_statements") && n.ChildCount() == 1 {
			c := n.Child(0)
			if c != nil && c.IsNamed() {
				n = c
				continue
			}
		}
		return n
	}
}

func serializeCGoNormalized(sb *strings.Builder, n *cgo.Node) {
	sb.WriteByte('(')
	sb.WriteString(n.Kind())
	if n.IsMissing() {
		sb.WriteString("!MISSING")
	}
	fmt.Fprintf(sb, " %d..%d", n.StartByte(), n.EndByte())
	parentKind := n.Kind()
	count := n.ChildCount()
	for i := uint(0); i < count; i++ {
		child := n.Child(i)
		if parentKind == "module" || parentKind == "block" {
			child = unwrapCGo(child)
		}
		sb.WriteByte(' ')
		if field := n.FieldNameForChild(uint32(i)); field != "" {
			sb.WriteString(field)
			sb.WriteByte(':')
		}
		serializeCGoNormalized(sb, child)
	}
	sb.WriteByte(')')
}

// divergenceKey names the node kinds at the first divergence point on each side.
func divergenceKey(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return fmt.Sprintf("cgo:%s | gts:%s", tokenAt(a, i), tokenAt(b, i))
}

func tokenAt(s string, i int) string {
	lo := strings.LastIndexAny(s[:i], "( ")
	if lo < 0 {
		lo = 0
	}
	hi := i + 40
	if hi > len(s) {
		hi = len(s)
	}
	t := s[lo:hi]
	if j := strings.IndexByte(t[1:], ' '); j >= 0 {
		t = t[:j+1]
	}
	return t
}

func runNormalized(files []string) {
	cgoParser := cgo.NewParser()
	defer cgoParser.Close()
	if err := cgoParser.SetLanguage(cgo.NewLanguage(cgopython.Language())); err != nil {
		fatal("cgo SetLanguage: %v", err)
	}
	gtsLang := gtsgrammars.PythonLanguage()
	gtsParser := gts.NewParser(gtsLang)

	classes := map[string]int{}
	examples := map[string]string{}
	mismatched := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fatal("read %s: %v", path, err)
		}
		ctree := cgoParser.Parse(src, nil)
		var csb strings.Builder
		serializeCGoNormalized(&csb, ctree.RootNode())
		ctree.Close()
		gtree, err := gtsParser.Parse(src)
		if err != nil {
			fatal("gts parse %s: %v", path, err)
		}
		var gsb strings.Builder
		serializeGTS(&gsb, gtree.RootNode(), gtsLang)
		if csb.String() != gsb.String() {
			mismatched++
			k := divergenceKey(csb.String(), gsb.String())
			classes[k]++
			if _, ok := examples[k]; !ok {
				examples[k] = path
				fmt.Printf("CLASS %s  (%s)\n", k, path)
				reportFirstDivergence(path, csb.String(), gsb.String())
			}
		}
	}
	fmt.Printf("normalized comparison: %d files, %d still mismatched\n", len(files), mismatched)
	keys := make([]string, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return classes[keys[i]] > classes[keys[j]] })
	for _, k := range keys {
		fmt.Printf("%6d  %s   e.g. %s\n", classes[k], k, examples[k])
	}
}
