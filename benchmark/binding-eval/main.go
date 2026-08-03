// Binding evaluation harness: gotreesitter (pure Go) vs the official CGo
// bindings, per the pinned criteria in DESIGN.md section 12.4.
//
// Absolute criteria (gates):
//  1. identical  — byte-identical parse trees vs the C grammar on the corpus.
//  2. queries    — support for every tree-sitter query form strictcode needs,
//                  with equal results from both engines on the corpus.
//
// If both gates pass: gotreesitter wins when its parse throughput is >= 75%
// of the CGo bindings' throughput (mode: throughput), else CGo wins.
//
// This is a standalone Go module so the main strictcode module only ever
// depends on the winning binding. Methodology and verdict: BUILDLOG.md.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	gtsgrammars "github.com/odvcencio/gotreesitter/grammars"
	cgo "github.com/tree-sitter/go-tree-sitter"
	cgopython "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

// queryForms is the closed list of tree-sitter query forms strictcode's
// extractors need, each exercised by a realistic Python query. Forms covered:
// named nodes, fields, captures, anonymous leaves, alternation [..],
// wildcard (_), quantifiers ? * +, anchor ., negated field !field,
// grouping (..), and the text predicates #eq? #not-eq? #match? #any-of?.
var queryForms = []struct {
	name  string
	query string
}{
	{"import-plain-and-aliased", `(import_statement name: [(dotted_name) (aliased_import name: (dotted_name))] @module)`},
	{"import-from", `(import_from_statement module_name: [(dotted_name) (relative_import)] @module)`},
	{"guarded-import-shape", `(try_statement body: (block [(import_statement) (import_from_statement)] @imp) (except_clause [(identifier) (tuple)] @exc)?)`},
	{"type-checking-block", `(if_statement condition: [(identifier) (attribute)] @cond (#match? @cond "TYPE_CHECKING$"))`},
	{"function-def", `(function_definition name: (identifier) @name)`},
	{"class-def-optional-bases", `(class_definition name: (identifier) @name superclasses: (argument_list)? @bases)`},
	{"calls", `(call function: [(identifier) (attribute)] @callee)`},
	{"dunder-all", `(assignment left: (identifier) @lhs (#eq? @lhs "__all__"))`},
	{"not-private-def", `(function_definition name: (identifier) @name (#not-eq? @name "__init__"))`},
	{"decorated", `(decorated_definition (decorator)+ @dec)`},
	{"docstring-anchor", `(block . (expression_statement (string)) @docstring)`},
	{"return-wildcard", `(return_statement (_) @val)`},
	{"stdout-calls", `((identifier) @id (#any-of? @id "print" "exec" "eval"))`},
	{"unannotated-def", `(function_definition !return_type name: (identifier) @name)`},
	{"grouped-siblings", `((import_statement) @first . (import_statement) @second)`},
}

func main() {
	mode := flag.String("mode", "", "identical | queries | throughput (required)")
	rounds := flag.Int("rounds", 3, "throughput rounds (best-of)")
	flag.Parse()
	if *mode == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: binding-eval -mode identical|queries|throughput <corpus-dir>...")
		os.Exit(2)
	}
	files := collectPython(flag.Args())
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no .py files found in corpus")
		os.Exit(2)
	}
	fmt.Printf("corpus: %d Python files\n", len(files))
	switch *mode {
	case "identical":
		runIdentical(files)
	case "queries":
		runQueries(files)
	case "throughput":
		runThroughput(files, *rounds)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", *mode)
		os.Exit(2)
	}
}

func collectPython(dirs []string) []string {
	var files []string
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == ".venv" || name == "node_modules" || name == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".py") {
				files = append(files, path)
			}
			return nil
		})
	}
	sort.Strings(files)
	return files
}

// --- Common serialization -------------------------------------------------
//
// Both walkers must emit the exact same text for the same tree:
//   (kind[!MISSING] start..end field:(child) (child) ...)
// All children are included, anonymous leaves too.

func serializeCGo(sb *strings.Builder, n *cgo.Node) {
	sb.WriteByte('(')
	sb.WriteString(n.Kind())
	if n.IsMissing() {
		sb.WriteString("!MISSING")
	}
	fmt.Fprintf(sb, " %d..%d", n.StartByte(), n.EndByte())
	count := n.ChildCount()
	for i := uint(0); i < count; i++ {
		child := n.Child(i)
		sb.WriteByte(' ')
		if field := n.FieldNameForChild(uint32(i)); field != "" {
			sb.WriteString(field)
			sb.WriteByte(':')
		}
		serializeCGo(sb, child)
	}
	sb.WriteByte(')')
}

func serializeGTS(sb *strings.Builder, n *gts.Node, lang *gts.Language) {
	sb.WriteByte('(')
	sb.WriteString(n.Type(lang))
	if n.IsMissing() {
		sb.WriteString("!MISSING")
	}
	fmt.Fprintf(sb, " %d..%d", n.StartByte(), n.EndByte())
	count := n.ChildCount()
	for i := 0; i < count; i++ {
		child := n.Child(i)
		sb.WriteByte(' ')
		if field := n.FieldNameForChild(i, lang); field != "" {
			sb.WriteString(field)
			sb.WriteByte(':')
		}
		serializeGTS(sb, child, lang)
	}
	sb.WriteByte(')')
}

// --- Mode: identical ------------------------------------------------------

func runIdentical(files []string) {
	cgoParser := cgo.NewParser()
	defer cgoParser.Close()
	cgoLang := cgo.NewLanguage(cgopython.Language())
	if err := cgoParser.SetLanguage(cgoLang); err != nil {
		fatal("cgo SetLanguage: %v", err)
	}
	gtsLang := gtsgrammars.PythonLanguage()
	gtsParser := gts.NewParser(gtsLang)

	mismatched := 0
	checked := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fatal("read %s: %v", path, err)
		}
		ctree := cgoParser.Parse(src, nil)
		var csb strings.Builder
		serializeCGo(&csb, ctree.RootNode())
		ctree.Close()

		gtree, err := gtsParser.Parse(src)
		if err != nil {
			fmt.Printf("MISMATCH %s: gotreesitter parse error: %v\n", path, err)
			mismatched++
			continue
		}
		var gsb strings.Builder
		serializeGTS(&gsb, gtree.RootNode(), gtsLang)

		checked++
		if csb.String() != gsb.String() {
			mismatched++
			fmt.Printf("MISMATCH %s\n", path)
			reportFirstDivergence(path, csb.String(), gsb.String())
		}
	}
	fmt.Printf("identical-trees gate: %d files checked, %d mismatched\n", checked, mismatched)
	if mismatched > 0 {
		fmt.Println("GATE: FAIL")
		os.Exit(1)
	}
	fmt.Println("GATE: PASS")
}

func reportFirstDivergence(path, a, b string) {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			lo := i - 80
			if lo < 0 {
				lo = 0
			}
			hiA, hiB := i+120, i+120
			if hiA > len(a) {
				hiA = len(a)
			}
			if hiB > len(b) {
				hiB = len(b)
			}
			fmt.Printf("  first divergence at offset %d\n  cgo: ...%s...\n  gts: ...%s...\n", i, a[lo:hiA], b[lo:hiB])
			return
		}
	}
	fmt.Printf("  one serialization is a prefix of the other (lens %d vs %d)\n", len(a), len(b))
}

// --- Mode: queries --------------------------------------------------------

func runQueries(files []string) {
	cgoLang := cgo.NewLanguage(cgopython.Language())
	cgoParser := cgo.NewParser()
	defer cgoParser.Close()
	if err := cgoParser.SetLanguage(cgoLang); err != nil {
		fatal("cgo SetLanguage: %v", err)
	}
	gtsLang := gtsgrammars.PythonLanguage()
	gtsParser := gts.NewParser(gtsLang)

	// Gate part 1: every query form must compile on both engines.
	cgoQueries := make([]*cgo.Query, len(queryForms))
	gtsQueries := make([]*gts.Query, len(queryForms))
	compileFailed := false
	for i, qf := range queryForms {
		cq, qerr := cgo.NewQuery(cgoLang, qf.query)
		if qerr != nil {
			fmt.Printf("FORM %s: cgo compile error: %v\n", qf.name, qerr)
			compileFailed = true
		}
		cgoQueries[i] = cq
		gq, err := gts.NewQuery(qf.query, gtsLang)
		if err != nil {
			fmt.Printf("FORM %s: gotreesitter compile error: %v\n", qf.name, err)
			compileFailed = true
		}
		gtsQueries[i] = gq
	}
	if compileFailed {
		fmt.Println("GATE: FAIL (query form not supported)")
		os.Exit(1)
	}
	fmt.Printf("all %d query forms compile on both engines\n", len(queryForms))

	// Gate part 2: equal results on the corpus.
	filesWithDiffs := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fatal("read %s: %v", path, err)
		}
		ctree := cgoParser.Parse(src, nil)
		gtree, gerr := gtsParser.Parse(src)
		if gerr != nil {
			fmt.Printf("DIFF %s: gotreesitter parse error: %v\n", path, gerr)
			filesWithDiffs++
			ctree.Close()
			continue
		}
		var diffs []string
		for i, qf := range queryForms {
			cres := cgoQueryResults(cgoQueries[i], ctree.RootNode(), src)
			gres := gtsQueryResults(gtsQueries[i], gtree.RootNode(), gtsLang, src)
			if !equalStringSets(cres, gres) {
				diffs = append(diffs, fmt.Sprintf("  form %s: cgo %d captures, gts %d captures\n    cgo-only: %v\n    gts-only: %v",
					qf.name, len(cres), len(gres), setMinus(cres, gres, 3), setMinus(gres, cres, 3)))
			}
		}
		ctree.Close()
		if len(diffs) > 0 {
			filesWithDiffs++
			fmt.Printf("DIFF %s\n%s\n", path, strings.Join(diffs, "\n"))
		}
	}
	fmt.Printf("query-results gate: %d files checked, %d with differing results\n", len(files), filesWithDiffs)
	if filesWithDiffs > 0 {
		fmt.Println("GATE: FAIL")
		os.Exit(1)
	}
	fmt.Println("GATE: PASS")
}

// cgoQueryResults returns a sorted list of "pattern|capture|start..end" strings.
func cgoQueryResults(q *cgo.Query, root *cgo.Node, src []byte) []string {
	cursor := cgo.NewQueryCursor()
	defer cursor.Close()
	names := q.CaptureNames()
	var out []string
	matches := cursor.Matches(q, root, src)
	for m := matches.Next(); m != nil; m = matches.Next() {
		for _, cap := range m.Captures {
			out = append(out, fmt.Sprintf("%d|%s|%d..%d", m.PatternIndex, names[cap.Index], cap.Node.StartByte(), cap.Node.EndByte()))
		}
	}
	sort.Strings(out)
	return out
}

func gtsQueryResults(q *gts.Query, root *gts.Node, lang *gts.Language, src []byte) []string {
	cursor := q.Exec(root, lang, src)
	var out []string
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, cap := range m.Captures {
			out = append(out, fmt.Sprintf("%d|%s|%d..%d", m.PatternIndex, cap.Name, cap.Node.StartByte(), cap.Node.EndByte()))
		}
	}
	sort.Strings(out)
	return out
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func setMinus(a, b []string, limit int) []string {
	inB := map[string]int{}
	for _, s := range b {
		inB[s]++
	}
	var out []string
	for _, s := range a {
		if inB[s] > 0 {
			inB[s]--
			continue
		}
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// --- Mode: throughput -----------------------------------------------------

func runThroughput(files []string, rounds int) {
	type doc struct {
		path string
		src  []byte
	}
	var docs []doc
	var totalBytes int64
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fatal("read %s: %v", path, err)
		}
		docs = append(docs, doc{path, src})
		totalBytes += int64(len(src))
	}
	fmt.Printf("corpus size: %.2f MB\n", float64(totalBytes)/1e6)

	// CGo engine.
	cgoLang := cgo.NewLanguage(cgopython.Language())
	cgoParser := cgo.NewParser()
	defer cgoParser.Close()
	if err := cgoParser.SetLanguage(cgoLang); err != nil {
		fatal("cgo SetLanguage: %v", err)
	}
	var cgoBest time.Duration
	var cgoHash [32]byte
	for r := 0; r < rounds; r++ {
		start := time.Now()
		h := sha256.New()
		for _, d := range docs {
			t := cgoParser.Parse(d.src, nil)
			root := t.RootNode()
			var buf [8]byte
			putU32(buf[0:4], uint32(root.EndByte()))
			putU32(buf[4:8], uint32(root.ChildCount()))
			h.Write(buf[:])
			t.Close()
		}
		elapsed := time.Since(start)
		copy(cgoHash[:], h.Sum(nil))
		if cgoBest == 0 || elapsed < cgoBest {
			cgoBest = elapsed
		}
		fmt.Printf("cgo round %d: %v\n", r+1, elapsed)
	}

	// Pure Go engine.
	gtsLang := gtsgrammars.PythonLanguage()
	gtsParser := gts.NewParser(gtsLang)
	var gtsBest time.Duration
	var gtsHash [32]byte
	for r := 0; r < rounds; r++ {
		start := time.Now()
		h := sha256.New()
		for _, d := range docs {
			t, err := gtsParser.Parse(d.src)
			if err != nil {
				fatal("gts parse %s: %v", d.path, err)
			}
			root := t.RootNode()
			var buf [8]byte
			putU32(buf[0:4], root.EndByte())
			putU32(buf[4:8], uint32(root.ChildCount()))
			h.Write(buf[:])
		}
		elapsed := time.Since(start)
		copy(gtsHash[:], h.Sum(nil))
		if gtsBest == 0 || elapsed < gtsBest {
			gtsBest = elapsed
		}
		fmt.Printf("gts round %d: %v\n", r+1, elapsed)
	}

	cgoMBs := float64(totalBytes) / 1e6 / cgoBest.Seconds()
	gtsMBs := float64(totalBytes) / 1e6 / gtsBest.Seconds()
	ratio := gtsMBs / cgoMBs
	fmt.Printf("\ncgo best: %v (%.2f MB/s)\ngts best: %v (%.2f MB/s)\n", cgoBest, cgoMBs, gtsBest, gtsMBs)
	fmt.Printf("throughput ratio gts/cgo: %.3f (threshold 0.75)\n", ratio)
	fmt.Printf("root-shape hashes equal: %v\n", cgoHash == gtsHash)
	if ratio >= 0.75 {
		fmt.Println("THROUGHPUT: gotreesitter meets the 75%% threshold")
	} else {
		fmt.Println("THROUGHPUT: gotreesitter below the 75%% threshold")
	}
}

func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
