// Package extract builds the interaction relation from a loaded workspace:
// the import-graph extractors for the language trio (DESIGN.md sections
// 6.2-6.4, schema/SPEC.md). One extraction pass populates one relation
// shared by every check (lesson 30).
package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/treesitter"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// Result is the extraction output: the frozen relation plus the line index
// used to derive 1-based lines from byte spans at output time.
type Result struct {
	Relation *relation.Relation
	// LineIndex maps a workspace-root-relative file path to the sorted byte
	// offsets at which its lines start (offset 0 is always present), over
	// the LF-normalized content.
	LineIndex map[string][]uint32
	// ExternalImports carries import sites whose specifier resolves to no
	// workspace member: the relation's node set covers only workspace
	// units, and library-forbidden-imports needs external specifiers
	// (flask, net/http, express). Side data beside the relation, sorted by
	// (lang, member, file, span, specifier).
	ExternalImports []ExternalImport
}

// ExternalImport is one import site of an external (non-workspace) target.
type ExternalImport struct {
	Lang vocab.Lang
	// Member is the importing member's name.
	Member string
	// SrcModule is the importing module's logical name.
	SrcModule string
	// Specifier: Python — the full dotted import (matching uses the
	// top-level segment); Go — the full import path; TS/JS — the raw bare
	// specifier.
	Specifier string
	// File is the workspace-root-relative importing file.
	File        string
	Span        relation.Span
	TestContext bool
}

// Line converts a byte offset in file to a 1-based line number. Files
// without an index (never read during extraction) report line 1.
func (r *Result) Line(file string, offset uint32) int {
	starts, ok := r.LineIndex[file]
	if !ok {
		return 1
	}
	lo, hi := 0, len(starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1
}

// extraction carries the in-progress state.
type extraction struct {
	ws      *workspace.Workspace
	builder *relation.Builder
	lines   map[string][]uint32

	// memberNodes tracks which (lang, member) member nodes exist.
	memberNodes map[string]relation.NodeID
	// entryPoints tracks emitted entry-point nodes (dedup for multi-file
	// Go main packages).
	entryPoints map[string]bool
	// external accumulates external-import sites (see ExternalImport).
	external []ExternalImport

	// pyIndex is the cross-member Python resolution index (built once).
	pyIndex *pyResolutionIndex
}

// Extract runs all extractors over the workspace and freezes the relation.
func Extract(ws *workspace.Workspace) (*Result, error) {
	ex := &extraction{
		ws:          ws,
		builder:     relation.NewBuilder(),
		lines:       map[string][]uint32{},
		memberNodes: map[string]relation.NodeID{},
		entryPoints: map[string]bool{},
	}

	// Python resolution index needs every member's package roots before any
	// member's imports are resolved (namespace map, DESIGN 6.3 step 4).
	idx, err := buildPyResolutionIndex(ws)
	if err != nil {
		return nil, err
	}
	ex.pyIndex = idx

	for _, m := range ws.Members {
		if err := ex.extractPython(m); err != nil {
			return nil, err
		}
		if err := ex.extractGo(m); err != nil {
			return nil, err
		}
		if err := ex.extractTS(m); err != nil {
			return nil, err
		}
	}

	rel, err := ex.builder.Build()
	if err != nil {
		return nil, err
	}
	sort.Slice(ex.external, func(i, j int) bool {
		a, b := ex.external[i], ex.external[j]
		if a.Lang != b.Lang {
			return a.Lang < b.Lang
		}
		if a.Member != b.Member {
			return a.Member < b.Member
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Span.Start != b.Span.Start {
			return a.Span.Start < b.Span.Start
		}
		return a.Specifier < b.Specifier
	})
	return &Result{Relation: rel, LineIndex: ex.lines, ExternalImports: ex.external}, nil
}

// --- member nodes and IDs -------------------------------------------------

// memberNodeID returns (creating on first use) the member node for a
// (lang, member) pair.
func (ex *extraction) memberNodeID(lang vocab.Lang, m *workspace.Member) (relation.NodeID, error) {
	key := string(lang) + "\x00" + m.Name
	if id, ok := ex.memberNodes[key]; ok {
		return id, nil
	}
	id := relation.NodeID{Lang: string(lang), Member: m.Name, Module: "_"}
	node := relation.Node{
		Kind: vocab.NodeKindWorkspaceMember,
		ID:   id,
		Attrs: map[string]relation.Value{
			"member_name":   relation.StringValue(m.Name),
			"registry_name": relation.StringValue(m.RegistryName(lang)),
			"is_library":    relation.BoolValue(m.Library),
			"is_dev_only":   relation.BoolValue(m.DevOnly),
			"root_path":     relation.StringValue(m.Path),
		},
	}
	if err := ex.builder.AddNode(node); err != nil {
		return relation.NodeID{}, err
	}
	ex.memberNodes[key] = id
	return id, nil
}

func moduleNodeID(lang vocab.Lang, member, logical string) relation.NodeID {
	return relation.NodeID{Lang: string(lang), Member: member, Module: logical}
}

func entryPointNodeID(lang vocab.Lang, member, form, name string) relation.NodeID {
	return relation.NodeID{
		Lang: string(lang), Member: member, Module: "_",
		Chain: []relation.Segment{{Name: form + "/" + name}},
	}
}

// --- file reading ---------------------------------------------------------

// readNormalized reads a file, LF-normalizes it, and records its line index
// under the workspace-root-relative path.
func (ex *extraction) readNormalized(relPath string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(ex.ws.Root, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	src := treesitter.NormalizeLF(raw)
	ex.recordLines(relPath, src)
	return src, nil
}

func (ex *extraction) recordLines(relPath string, src []byte) {
	starts := []uint32{0}
	for i, b := range src {
		if b == '\n' && i+1 < len(src) {
			starts = append(starts, uint32(i+1))
		}
	}
	ex.lines[relPath] = starts
}

// locate returns the span of the first occurrence of needle in the
// LF-normalized content of relPath (reading and indexing it on demand), or
// a zero span when absent. Used to give manifest-declared rows real sites.
func (ex *extraction) locate(relPath, needle string) relation.Span {
	src, err := ex.readNormalized(relPath)
	if err != nil {
		return relation.Span{}
	}
	if i := strings.Index(string(src), needle); i >= 0 {
		return relation.Span{Start: uint32(i), End: uint32(i + len(needle))}
	}
	return relation.Span{}
}

// --- declared dependencies ------------------------------------------------

// depResolver matches one declared dependency name against a candidate
// member, per-ecosystem.
type depResolver func(depName string, candidate *workspace.Member) bool

// emitDeclaredDeps adds declares_dependency rows for every dep of the
// given manifest that resolves to a sibling workspace member. mf may be a
// member's root manifest or a nested one (Go nested modules).
func (ex *extraction) emitDeclaredDeps(lang vocab.Lang, m *workspace.Member, mf *workspace.Manifest, matches depResolver) error {
	if mf == nil {
		return nil
	}
	srcID, err := ex.memberNodeID(lang, m)
	if err != nil {
		return err
	}
	for _, dep := range mf.Deps {
		for _, other := range ex.ws.Members {
			if other == m || !matches(dep.Name, other) {
				continue
			}
			dstID, err := ex.memberNodeID(lang, other)
			if err != nil {
				return err
			}
			row := relation.Row{
				Kind: vocab.RowKindDeclaresDependency,
				Src:  srcID,
				Dst:  dstID,
				File: mf.Path,
				Span: ex.locate(mf.Path, dep.Name),
				Attrs: map[string]relation.Value{
					"scope": relation.StringValue(string(dep.Scope)),
				},
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
			break
		}
	}
	return nil
}
