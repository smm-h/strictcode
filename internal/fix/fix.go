// Package fix is the tier-1 fix engine (DESIGN.md section 7, SPEC.md
// section 7): a whitelist of hand-proven transforms plus mechanical
// re-verification. A transform declares its relation delta; after applying
// the edits, the workspace is re-extracted and the actual post-fix relation
// must equal the expected one (canonical-form comparison, span attributes
// masked for rows at or after the edit point in edited files). A mismatch
// rolls the file back and reports a tool bug — it is never silently
// accepted.
//
// The whitelist has one transform: unreachable-statement removal (the
// flagship, offered by the unreachable-code rule).
package fix

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// Planned is one planned tier-1 fix: a byte-range removal in one file.
type Planned struct {
	Rule        string
	Description string
	// File is the workspace-root-relative path.
	File string
	// Start/End is the removal range over the file's LF-normalized bytes,
	// snapped to whole lines at apply time.
	Start uint32
	End   uint32
}

// PlanUnreachableRemovals plans the whitelisted removal transform for
// every unreachable region the configuration leaves active. Regions
// contained within a larger dead region of the same file collapse into the
// outer removal.
func PlanUnreachableRemovals(res *extract.Result, cfg *config.Effective) []Planned {
	setting := cfg.Setting("unreachable-code")
	if !setting.Enabled {
		return nil
	}
	suppressed := map[string]bool{}
	for _, s := range setting.Suppressions {
		if s.Path != "" {
			suppressed[s.Path] = true
		}
	}

	regions := make([]extract.UnreachableRegion, 0, len(res.Unreachable))
	for _, u := range res.Unreachable {
		if !suppressed[u.File] {
			regions = append(regions, u)
		}
	}
	// Drop regions nested inside another region of the same file, and
	// refuse regions whose removal would renumber same-name (or same-hint)
	// siblings outside the region — ordinal drift the pruning delta cannot
	// express (SPEC 2.4); those stay detection-only.
	var out []Planned
	for i, u := range regions {
		nested := false
		for j, outer := range regions {
			if i == j || u.File != outer.File {
				continue
			}
			if u.Span.Start >= outer.Span.Start && u.Span.End <= outer.Span.End &&
				(outer.Span.End-outer.Span.Start) > (u.Span.End-u.Span.Start) {
				nested = true
				break
			}
		}
		if nested || removalShiftsSiblings(res.Relation, u) {
			continue
		}
		out = append(out, Planned{
			Rule:        "unreachable-code",
			Description: "remove unreachable statements",
			File:        u.File,
			Start:       u.Span.Start,
			End:         u.Span.End,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Start < out[j].Start
	})
	return out
}

// removalShiftsSiblings reports whether a region contains a definition
// (function, type, or closure) that has a same-name or same-hint sibling in
// the same container OUTSIDE the region: removing it would renumber the
// sibling's overload index or ordinal, changing IDs beyond the transform's
// declared delta.
func removalShiftsSiblings(rel *relation.Relation, u extract.UnreachableRegion) bool {
	within := func(r relation.Row) bool {
		return r.File == u.File && r.Span.Start >= u.Span.Start && r.Span.End <= u.Span.End
	}
	for _, r := range rel.Rows {
		if r.Kind != vocab.RowKindContains || !within(r) {
			continue
		}
		chain := r.Dst.Chain
		if len(chain) == 0 {
			continue
		}
		last := chain[len(chain)-1]
		for _, other := range rel.Rows {
			if other.Kind != vocab.RowKindContains || within(other) {
				continue
			}
			oc := other.Dst.Chain
			if len(oc) != len(chain) {
				continue
			}
			olast := oc[len(oc)-1]
			if olast.Name != last.Name || olast.Anonymous != last.Anonymous {
				continue
			}
			// Same container prefix?
			same := true
			for k := 0; k < len(chain)-1; k++ {
				if chain[k] != oc[k] {
					same = false
					break
				}
			}
			if same && other.Dst.Member == r.Dst.Member && other.Dst.Module == r.Dst.Module {
				return true
			}
		}
	}
	return false
}

// Report is the outcome of Apply.
type Report struct {
	Applied []Planned
	// FilesEdited is the number of distinct files rewritten.
	FilesEdited int
}

// Apply performs the planned fixes and runs the SPEC section 7
// verification. On a verification mismatch every edited file is restored
// and an error describing the tool bug is returned.
func Apply(ws *workspace.Workspace, pre *extract.Result, plans []Planned) (*Report, error) {
	if len(plans) == 0 {
		return &Report{}, nil
	}

	// Group by file; load and line-snap.
	byFile := map[string][]Planned{}
	for _, p := range plans {
		byFile[p.File] = append(byFile[p.File], p)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	originals := map[string][]byte{}
	edited := map[string][]byte{}
	// editStart: per file, the lowest snapped edit offset (the mask point).
	editStart := map[string]uint32{}

	for _, f := range files {
		full := filepath.Join(ws.Root, filepath.FromSlash(f))
		raw, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("fix: %w", err)
		}
		// Tier-1 edits are computed over LF-normalized bytes; rewriting a
		// CRLF file would silently renormalize it wholesale. Refuse.
		if bytes.ContainsRune(raw, '\r') {
			return nil, fmt.Errorf("fix: %s has CR line endings; tier-1 auto-fix requires LF files (normalize the file first)", f)
		}
		originals[f] = raw

		// Snap each range to whole lines and sort descending for splicing.
		ranges := make([][2]uint32, 0, len(byFile[f]))
		for _, p := range byFile[f] {
			start, end := snapToLines(raw, p.Start, p.End)
			ranges = append(ranges, [2]uint32{start, end})
		}
		sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] > ranges[j][0] })
		content := append([]byte{}, raw...)
		min := uint32(len(raw))
		for _, r := range ranges {
			if r[0] < min {
				min = r[0]
			}
			content = append(content[:r[0]], content[r[1]:]...)
		}
		edited[f] = content
		editStart[f] = min
	}

	// Expected post-fix relation: prune rows within removed ranges and the
	// nodes (transitively) defined inside them.
	expNodes, expRows := pruneExpected(pre.Relation, byFile)

	// Write, re-extract, verify.
	for _, f := range files {
		full := filepath.Join(ws.Root, filepath.FromSlash(f))
		if err := os.WriteFile(full, edited[f], 0o644); err != nil {
			restore(ws, originals)
			return nil, fmt.Errorf("fix: %w", err)
		}
	}
	post, err := extract.Extract(ws)
	if err != nil {
		restore(ws, originals)
		return nil, fmt.Errorf("fix: post-fix re-extraction failed (rolled back): %w", err)
	}

	expected := relation.CanonicalFormOf(expNodes, maskRows(expRows, editStart))
	actual := relation.CanonicalFormOf(post.Relation.Nodes, maskRows(post.Relation.Rows, editStart))
	if !bytes.Equal(expected, actual) {
		restore(ws, originals)
		return nil, fmt.Errorf("fix: post-fix relation does not match the transform's declared delta — TOOL BUG, all edits rolled back (report this):\n%s", firstDiffLine(expected, actual))
	}

	return &Report{Applied: plans, FilesEdited: len(files)}, nil
}

// snapToLines widens [start, end) to whole lines: back to the start of
// start's line, forward past end's line terminator.
func snapToLines(content []byte, start, end uint32) (uint32, uint32) {
	s := int(start)
	for s > 0 && content[s-1] != '\n' {
		s--
	}
	e := int(end)
	for e < len(content) && content[e] != '\n' {
		e++
	}
	if e < len(content) {
		e++ // include the newline
	}
	return uint32(s), uint32(e)
}

// pruneExpected computes the declared delta of the removal transform: rows
// sited inside a removed range disappear; nodes introduced by removed
// contains rows disappear transitively, along with every row touching
// them.
func pruneExpected(rel *relation.Relation, byFile map[string][]Planned) ([]relation.Node, []relation.Row) {
	inRemoved := func(r relation.Row) bool {
		for _, p := range byFile[r.File] {
			if r.Span.Start >= p.Start && r.Span.End <= p.End {
				return true
			}
		}
		return false
	}

	removedNodes := map[string]bool{}
	// Seed: dst nodes of removed contains rows; closure over containment.
	changed := true
	for changed {
		changed = false
		for _, r := range rel.Rows {
			if r.Kind != vocab.RowKindContains {
				continue
			}
			if inRemoved(r) || removedNodes[r.Src.String()] {
				id := r.Dst.String()
				if !removedNodes[id] {
					removedNodes[id] = true
					changed = true
				}
			}
		}
	}

	var rows []relation.Row
	for _, r := range rel.Rows {
		if inRemoved(r) || removedNodes[r.Src.String()] || removedNodes[r.Dst.String()] {
			continue
		}
		rows = append(rows, r)
	}
	var nodes []relation.Node
	for _, n := range rel.Nodes {
		if removedNodes[n.ID.String()] {
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, rows
}

// maskRows zeroes the span of every row in an edited file. SPEC section 7
// says spans below the fix point are ignored; the sound symmetric predicate
// masks the whole edited file, because a shrunk enclosing span can end just
// before the edit point post-fix while its pre-fix span reached beyond it —
// a point-relative predicate cannot correlate the two sides (BUILDLOG,
// round 3). Rows in non-edited files keep full span verification, and
// structural identity (kinds, IDs, attributes) is verified everywhere.
func maskRows(rows []relation.Row, editStart map[string]uint32) []relation.Row {
	out := make([]relation.Row, len(rows))
	for i, r := range rows {
		if _, edited := editStart[r.File]; edited {
			r.Span = relation.Span{}
		}
		out[i] = r
	}
	return out
}

func restore(ws *workspace.Workspace, originals map[string][]byte) {
	for f, content := range originals {
		full := filepath.Join(ws.Root, filepath.FromSlash(f))
		_ = os.WriteFile(full, content, 0o644)
	}
}

// firstDiffLine renders the first differing canonical line for the
// tool-bug report.
func firstDiffLine(a, b []byte) string {
	al := bytes.Split(a, []byte{'\n'})
	bl := bytes.Split(b, []byte{'\n'})
	n := len(al)
	if len(bl) < n {
		n = len(bl)
	}
	for i := 0; i < n; i++ {
		if !bytes.Equal(al[i], bl[i]) {
			return fmt.Sprintf("  expected: %s\n  actual:   %s", al[i], bl[i])
		}
	}
	return fmt.Sprintf("  line counts differ: expected %d, actual %d", len(al), len(bl))
}
