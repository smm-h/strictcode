package checks

import (
	"strings"

	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// langMember keys per-language member data.
type langMember struct {
	Lang   vocab.Lang
	Member string
}

// memberEdge keys member-to-member relations within one language.
type memberEdge struct {
	Lang vocab.Lang
	Src  string
	Dst  string
}

// ModuleInfo is one module node's queryable projection.
type ModuleInfo struct {
	ID      relation.NodeID
	Logical string
	// Path is workspace-root-relative; MemberRel is member-root-relative.
	Path      string
	MemberRel string
	Test      bool
}

// ModuleEdge is one module->module imports row.
type ModuleEdge struct {
	Dst string
	Row relation.Row
}

// ExportEdge is one exports row (src module exports dst module).
type ExportEdge struct {
	Src string
	Dst string
}

// EntryPointInfo is one entry_point node with its site.
type EntryPointInfo struct {
	ID   relation.NodeID
	Form string
	Name string
	// File is the site for findings: the resolves_to row's file when
	// present, else the member manifest.
	File string
	Span relation.Span
}

// View indexes the relation for the checks. Built once per run from the one
// shared relation (lesson 30).
type View struct {
	WS  *workspace.Workspace
	Res *extract.Result

	// Modules: (lang, member) -> logical -> info.
	Modules map[langMember]map[string]*ModuleInfo
	// MemberImports: imports rows whose dst is a member node.
	MemberImports map[memberEdge][]relation.Row
	// DeclaredDeps: declares_dependency rows.
	DeclaredDeps map[memberEdge][]relation.Row
	// ModuleImports: (lang, member) -> src logical -> edges.
	ModuleImports map[langMember]map[string][]ModuleEdge
	// Exports: (lang, member) -> export edges.
	Exports map[langMember][]ExportEdge
	// EntryTargets: (lang, member) -> logicals reached by resolves_to rows.
	EntryTargets map[langMember][]string
	// EntryPoints: (lang, member) -> entry point infos.
	EntryPoints map[langMember][]EntryPointInfo
	// memberNodeIDs: (lang, member) -> serialized member node ID.
	memberNodeIDs map[langMember]string
}

func buildView(ws *workspace.Workspace, res *extract.Result) *View {
	v := &View{
		WS:            ws,
		Res:           res,
		Modules:       map[langMember]map[string]*ModuleInfo{},
		MemberImports: map[memberEdge][]relation.Row{},
		DeclaredDeps:  map[memberEdge][]relation.Row{},
		ModuleImports: map[langMember]map[string][]ModuleEdge{},
		Exports:       map[langMember][]ExportEdge{},
		EntryTargets:  map[langMember][]string{},
		EntryPoints:   map[langMember][]EntryPointInfo{},
		memberNodeIDs: map[langMember]string{},
	}

	epByID := map[string]*EntryPointInfo{}

	for i := range res.Relation.Nodes {
		n := &res.Relation.Nodes[i]
		lm := langMember{vocab.Lang(n.ID.Lang), n.ID.Member}
		switch n.Kind {
		case vocab.NodeKindModule:
			logical, _ := n.Attrs["logical_name"].AsString()
			path, _ := n.Attrs["path"].AsString()
			test, _ := n.Attrs["test_context"].AsBool()
			if v.Modules[lm] == nil {
				v.Modules[lm] = map[string]*ModuleInfo{}
			}
			v.Modules[lm][logical] = &ModuleInfo{
				ID:        n.ID,
				Logical:   logical,
				Path:      path,
				MemberRel: memberRelPath(ws, n.ID.Member, path),
				Test:      test,
			}
		case vocab.NodeKindWorkspaceMember:
			v.memberNodeIDs[lm] = n.ID.String()
		case vocab.NodeKindEntryPoint:
			form, _ := n.Attrs["form"].AsString()
			name, _ := n.Attrs["declared_name"].AsString()
			info := EntryPointInfo{ID: n.ID, Form: form, Name: name}
			// Fallback site: the member's manifest for the language.
			if m := ws.MemberByName(n.ID.Member); m != nil {
				if mf := m.Manifests[vocab.Lang(n.ID.Lang)]; mf != nil {
					info.File = mf.Path
				}
			}
			v.EntryPoints[lm] = append(v.EntryPoints[lm], info)
			epByID[n.ID.String()] = &v.EntryPoints[lm][len(v.EntryPoints[lm])-1]
		}
	}

	for _, r := range res.Relation.Rows {
		lang := vocab.Lang(r.Src.Lang)
		switch r.Kind {
		case vocab.RowKindImports:
			if r.Dst.Module == "_" && len(r.Dst.Chain) == 0 {
				// Member import.
				e := memberEdge{lang, r.Src.Member, r.Dst.Member}
				v.MemberImports[e] = append(v.MemberImports[e], r)
			} else {
				lm := langMember{lang, r.Src.Member}
				if v.ModuleImports[lm] == nil {
					v.ModuleImports[lm] = map[string][]ModuleEdge{}
				}
				v.ModuleImports[lm][r.Src.Module] = append(v.ModuleImports[lm][r.Src.Module],
					ModuleEdge{Dst: r.Dst.Module, Row: r})
			}
		case vocab.RowKindDeclaresDependency:
			e := memberEdge{lang, r.Src.Member, r.Dst.Member}
			v.DeclaredDeps[e] = append(v.DeclaredDeps[e], r)
		case vocab.RowKindExports:
			lm := langMember{lang, r.Src.Member}
			v.Exports[lm] = append(v.Exports[lm], ExportEdge{Src: r.Src.Module, Dst: r.Dst.Module})
		case vocab.RowKindResolvesTo:
			lm := langMember{lang, r.Src.Member}
			v.EntryTargets[lm] = append(v.EntryTargets[lm], r.Dst.Module)
			if info, ok := epByID[r.Src.String()]; ok {
				info.File = r.File
				info.Span = r.Span
			}
		}
	}
	return v
}

// memberRelPath strips the member's path prefix from a workspace-relative
// path.
func memberRelPath(ws *workspace.Workspace, memberName, wsRel string) string {
	m := ws.MemberByName(memberName)
	if m == nil || m.Path == "." {
		return wsRel
	}
	return strings.TrimPrefix(wsRel, m.Path+"/")
}

// rowBool reads a boolean row attribute.
func rowBool(r relation.Row, name string) bool {
	b, _ := r.Attrs[name].AsBool()
	return b
}

// rowScope reads a declares_dependency row's scope.
func rowScope(r relation.Row) workspace.DepScope {
	s, _ := r.Attrs["scope"].AsString()
	return workspace.DepScope(s)
}
