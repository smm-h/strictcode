package checks

import (
	"errors"
	"fmt"

	"github.com/smm-h/strictcode/internal/certificate"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/options"
	"github.com/smm-h/strictcode/internal/vocab"
)

// checkStrictspecCertificate reports every reason the declared strictspec
// diff certificate blocks: a violated claim, an unsupported claim no
// adjudication entry discharges, and a dangling adjudication entry. The
// rule runs only while its option is on (it defaults to off), so a missing
// [strictspec_certificate] declaration is refused, and so is a declared file
// that cannot be read.
func checkStrictspecCertificate(ctx *Context) []findings.Finding {
	decl := ctx.Cfg.Certificate
	if decl == nil {
		ctx.fail(fmt.Errorf("strictspec-certificate: strictcode:strictspec-certificate is on, but %s declares no [strictspec_certificate]: declare certificate = \"<path>\" there, or %s", ctx.CfgPath, options.SwitchOff("strictspec-certificate")))
		return nil
	}
	blockers, err := certificate.Evaluate(ctx.View.WS.Root, decl.Certificate, decl.Adjudication)
	if errors.Is(err, certificate.ErrNoCertificate) {
		ctx.fail(fmt.Errorf("strictspec-certificate: %w: produce it with `strictspec diff`, or %s", err, options.SwitchOff("strictspec-certificate")))
		return nil
	}
	if err != nil {
		ctx.fail(fmt.Errorf("strictspec-certificate: %w", err))
		return nil
	}
	severity := options.Severity(ctx.Opts.Value("strictspec-certificate"))
	target := anyMemberTargetID(ctx)
	var out []findings.Finding
	for _, b := range blockers {
		out = append(out, ctx.findingAtLine("strictspec-certificate", severity,
			target, vocab.NodeKindWorkspaceMember, b.File, 1, b.Reason))
	}
	return out
}

// anyMemberTargetID is the node ID language-independent findings point at:
// the first workspace member's node in the first language it has one in.
func anyMemberTargetID(ctx *Context) string {
	member := ctx.View.WS.Members[0].Name
	for _, lang := range vocab.Langs {
		if id, ok := ctx.View.memberNodeIDs[langMember{lang, member}]; ok {
			return id
		}
	}
	return memberTargetID(ctx, vocab.LangPy, member)
}
