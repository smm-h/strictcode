package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/vocab"
)

// checkStaleSuppression: config rot is a defect (lesson 32). A suppression
// referencing a path that no longer exists on disk, a member that is not in
// the workspace, or modules absent from the relation is an error finding.
// (Rule-ID validity, including tombstones, is a config-load hard error.)
func checkStaleSuppression(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, sup := range ctx.Cfg.AllSuppressions() {
		switch sup.Shape {
		case rules.SuppressPath:
			full := filepath.Join(ctx.View.WS.Root, filepath.FromSlash(sup.Path))
			if _, err := os.Stat(full); err != nil {
				out = append(out, ctx.staleFinding(sup,
					fmt.Sprintf("suppression for rule %s references nonexistent path '%s'", sup.Rule, sup.Path)))
			}
		case rules.SuppressProjectDep:
			var missing []string
			if ctx.View.WS.MemberByName(sup.Project) == nil {
				missing = append(missing, sup.Project)
			}
			if ctx.View.WS.MemberByName(sup.Dep) == nil {
				missing = append(missing, sup.Dep)
			}
			if len(missing) > 0 {
				out = append(out, ctx.staleFinding(sup,
					fmt.Sprintf("suppression for rule %s references nonexistent workspace member(s): %s", sup.Rule, strings.Join(missing, ", "))))
			}
		case rules.SuppressMember:
			if ctx.View.WS.MemberByName(sup.Member) == nil {
				out = append(out, ctx.staleFinding(sup,
					fmt.Sprintf("suppression for rule %s references nonexistent workspace member '%s'", sup.Rule, sup.Member)))
			}
		case rules.SuppressMemberSet:
			known := map[string]bool{}
			for lm, modules := range ctx.View.Modules {
				_ = lm
				for logical := range modules {
					known[logical] = true
				}
			}
			var missing []string
			for _, mod := range sup.Modules {
				if !known[mod] {
					missing = append(missing, mod)
				}
			}
			if len(missing) > 0 {
				out = append(out, ctx.staleFinding(sup,
					fmt.Sprintf("suppression for rule %s references nonexistent module(s): %s", sup.Rule, strings.Join(missing, ", "))))
			}
		}
	}
	return out
}

// staleFinding targets the config file; the node is the most relevant
// member node available (the named member when resolvable, else the first
// workspace member).
func (ctx *Context) staleFinding(sup config.Suppression, message string) findings.Finding {
	member := ctx.View.WS.Members[0].Name
	switch {
	case sup.Shape == rules.SuppressProjectDep && ctx.View.WS.MemberByName(sup.Project) != nil:
		member = sup.Project
	case sup.Shape == rules.SuppressMember && ctx.View.WS.MemberByName(sup.Member) != nil:
		member = sup.Member
	}
	targetID := relation.NodeID{Lang: "py", Member: member, Module: "_"}.String()
	for _, lang := range vocab.Langs {
		if id, ok := ctx.View.memberNodeIDs[langMember{lang, member}]; ok {
			targetID = id
			break
		}
	}
	return ctx.finding("stale-suppression", targetID, vocab.NodeKindWorkspaceMember,
		ctx.CfgPath, 0, message)
}
