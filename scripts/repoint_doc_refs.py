#!/usr/bin/env python3
"""One-off rewrite of references to the retired root design documents.

DESIGN.md, schema/SPEC.md, CATALOG.md, BUILDLOG.md, and docs/MATRIX.md were
replaced by pages under stricttools/docs/. Each entry below names a file, the
exact old text, and the new text; every entry must match once, so a typo stops
the run instead of silently changing nothing.

Usage: scripts/repoint_doc_refs.py --dry-run | --apply
"""
import sys
from pathlib import Path

D = "stricttools/docs/"
R = [
("benchmark/binding-eval/main.go", "per the pinned criteria in DESIGN.md section 12.4.", f"per the pinned criteria recorded in {D}decisions.md."),
("benchmark/binding-eval/main.go", "Methodology and verdict: BUILDLOG.md.", f"Methodology and verdicts: {D}decisions.md (experiments)."),
("internal/config/config.go", "express (DESIGN.md section 12.3):", f"express ({D}config.md):"),
("internal/config/config.go", "the stale-suppression rule (CATALOG.md), evaluated", f"the stale-suppression rule ({D}rules/stale-suppression.md), evaluated"),
("internal/checks/callgraph.go", "(DESIGN.md 6.5:", f"({D}check-semantics.md, library-boundary rules:"),
("internal/checks/lessons_test.go", "The lessons register (DESIGN.md section 6.7)", f"The lessons register ({D}lessons.md)"),
("internal/engine/engine.go", "the stateless batch pipeline (DESIGN.md section 9):", f"the stateless batch pipeline ({D}implementation.md, runtime model):"),
("internal/extract/pystdlib.go", "sys.stdlib_module_names (see BUILDLOG).", "sys.stdlib_module_names."),
("internal/extract/pystdlib.go", "Used by the DESIGN.md section-6.3 resolution order, step 1:", f"Used by step 1 of the import resolution order ({D}check-semantics.md):"),
("internal/extract/pystdlib.go", "with the command recorded in BUILDLOG.md if the interpreter baseline moves.", "with the generating command from this file's git history if the interpreter baseline moves."),
("internal/extract/tsjs.go", "(DESIGN.md 6.2, lesson 18).", f"({D}check-semantics.md, lesson 18)."),
("internal/extract/tsjs.go", "directory (SPEC.md 2.2).", f"directory ({D}node-identity.md, module segment)."),
("internal/extract/tsjs.go", "case-insensitively (DESIGN.md 6.3 TS/JS).", f"case-insensitively ({D}check-semantics.md, TypeScript/JavaScript import resolution)."),
("internal/extract/python.go", "// --- resolution index (DESIGN.md 6.3, built across all members) -----------", "// --- resolution index (built across all members; see check-semantics.md) ---"),
("internal/extract/python.go", "resolveMember applies the DESIGN.md 6.3 resolution order", f"resolveMember applies the import resolution order in {D}check-semantics.md"),
("internal/extract/python.go", "(SPEC.md 2.2: dotted path from the discovered", f"({D}node-identity.md, module segment: dotted path from the discovered"),
("internal/extract/python.go", "resolution (DESIGN.md 6.3 step 1 drops them).", "resolution (step 1 of the import resolution order drops them)."),
("internal/extract/extract.go", "(DESIGN.md sections\n// 6.2-6.4, schema/SPEC.md).", f"({D}check-semantics.md\n// and {D}graph-model.md)."),
("internal/extract/extract.go", "rows demand both endpoints exist as nodes; see BUILDLOG.", f"rows demand both endpoints exist as nodes; see {D}graph-model.md, side tables."),
("internal/extract/extract.go", "(namespace map, DESIGN 6.3 step 4).", "(namespace map, step 4 of the import resolution order)."),
("internal/relation/id.go", "(schema/SPEC.md\n// section 2).", f"({D}node-identity.md)."),
("internal/relation/id.go", "(SPEC.md section 2.2).", f"({D}node-identity.md, module segment)."),
("internal/relation/id.go", "(SPEC.md section 2.4).", f"({D}node-identity.md, overloads and redefinitions)."),
("internal/relation/id.go", "(SPEC.md section 2.3):", f"({D}node-identity.md, anonymous units):"),
("internal/relation/id.go", "percent-encoded (SPEC.md section 2.1 lists %, :, ., #, whitespace; the", f"percent-encoded ({D}node-identity.md, escaping: %, :, ., #, ~, and whitespace; the"),
("internal/relation/id.go", "<hint>~<ordinal>~<fp8> is unambiguous — see BUILDLOG.md).", "<hint>~<ordinal>~<fp8> is unambiguous)."),
("internal/extract/golang.go", "or is prefixed by it (DESIGN.md 6.3, Go).", f"or is prefixed by it ({D}check-semantics.md, Go import resolution)."),
("internal/checks/dead.go", "pinned in DESIGN.md 6.2:", f"described in {D}check-semantics.md, dead modules:"),
("internal/extract/python_sem.go", "(DESIGN.md section 8 layer 1).", f"({D}call-resolution.md, the syntactic layer)."),
("internal/extract/python_sem.go", "with SPEC 2.3-2.5 identity (anonymous", f"with the identity rules in {D}node-identity.md (anonymous"),
("internal/extract/python_sem.go", "(DESIGN.md 6.5): return/raise/break/continue;", f"({D}rules/unreachable-code.md): return/raise/break/continue;"),
("internal/extract/python_sem.go", "// (SPEC 2.3).", f"// ({D}node-identity.md, anonymous units)."),
("internal/extract/python_sem.go", "(whitespace collapsed) — SPEC 2.3.", f"(whitespace collapsed); see {D}node-identity.md, anonymous units."),
("internal/spec/roundtrip_test.go", "// SPEC.md section 5: vocabulary and profiles are closed sets;", f"// {D}capabilities-and-profiles.md: vocabulary and profiles are closed sets;"),
("internal/extract/walk.go", "from DESIGN.md section 6.6", f"from {D}check-semantics.md (source-walk exclusions)"),
("internal/fix/fix.go", "(DESIGN.md section 7, SPEC.md\n// section 7):", f"({D}fixes.md):"),
("internal/fix/fix.go", "express (SPEC 2.4); those stay detection-only.", f"express ({D}node-identity.md, overloads and redefinitions); those stay detection-only."),
("internal/fix/fix.go", "// Apply performs the planned fixes and runs the SPEC section 7\n// verification.", f"// Apply performs the planned fixes and runs the post-fix verification\n// described in {D}fixes.md."),
("internal/fix/fix.go", "// maskRows zeroes the span of every row in an edited file. SPEC section 7\n// says spans below the fix point are ignored; the sound symmetric predicate\n// masks the whole edited file, because a shrunk enclosing span can end just\n// before the edit point post-fix while its pre-fix span reached beyond it —\n// a point-relative predicate cannot correlate the two sides (BUILDLOG,\n// round 3).", f"// maskRows zeroes the span of every row in an edited file. Spans are\n// ignored across the whole edited file, not only after the edit point,\n// because a shrunk enclosing span can end just before the edit point post-fix\n// while its pre-fix span reached beyond it, so a point-relative predicate\n// cannot correlate the two sides ({D}fixes.md)."),
("internal/rules/matrix.go", "// CATALOG.md requires/uses model:", f"// requires/uses model ({D}capabilities-and-profiles.md):"),
("internal/findings/findings.go", "(schema/SPEC.md section 2).", f"({D}node-identity.md)."),
("internal/rules/rules_test.go", 't.Fatalf("registry has %d rules, CATALOG.md mints 14", len(Rules))', 't.Fatalf("registry has %d rules, the rule reference documents 14", len(Rules))'),
("internal/rules/rules.go", "// Package rules is the rule registry: the Go declarations behind CATALOG.md.", f"// Package rules is the rule registry: the Go declarations documented in {D}rules.md."),
("internal/rules/rules.go", "// (DESIGN.md section 12.3). Config suppressions", f"// ({D}config.md). Config suppressions"),
("internal/rules/rules.go", "(DESIGN.md section 7).", f"({D}fixes.md)."),
("internal/rules/rules.go", "re-minted at their best form pre-ship (CATALOG.md).", f"re-minted at their best form pre-ship ({D}decisions.md)."),
("internal/rules/rules.go", '"Source units unreachable or unreferenced, per the per-language algorithms pinned in DESIGN.md section 6.2."', '"Source units unreachable or unreferenced, per the per-language algorithms on the check semantics page."'),
("internal/rules/rules.go", "// BUILDLOG 2026-08-04): the export-exemption facet", f"// {D}decisions.md, 2026-08-04): the export-exemption facet"),
("internal/relation/relation.go", "artifact of extraction (schema/SPEC.md).", f"artifact of extraction ({D}graph-model.md)."),
("internal/relation/relation.go", "(schema/SPEC.md section 3).", f"({D}graph-model.md, spans and positions)."),
("internal/relation/relation.go", "(SPEC.md section 1):", f"({D}graph-model.md):"),
("internal/relation/relation_test.go", "// SPEC.md section 4.2: calls.resolution is mandatory, no default.", f"// {D}vocabulary.md: calls.resolution is mandatory, no default."),
("internal/relation/relation_test.go", "// SPEC.md section 1: the relation is an ordered SET of rows.", f"// {D}graph-model.md: the relation is an ordered SET of rows."),
("internal/relation/relation_test.go", "// SPEC 2.2 scopes the case-only-clash hard error to module identity", f"// {D}node-identity.md scopes the case-only-clash hard error to module identity"),
("internal/testctx/testctx_test.go", "(DESIGN.md section 6.4).", f"({D}check-semantics.md, test context)."),
("internal/testctx/testctx.go", "(DESIGN.md\n// section 6.4):", f"({D}check-semantics.md,\n// test context):"),
("internal/vocab/gen/gen.go", "(SPEC.md section 5 — unknown capability names are hard", f"({D}capabilities-and-profiles.md: unknown capability names are hard"),
("internal/workspace/workspace.go", "disk (DESIGN.md section 6.6):", f"disk ({D}check-semantics.md, workspace and manifest inputs):"),
("internal/checks/library.go", "(DESIGN.md 6.5), replaceable via config.", f"({D}check-semantics.md, library-boundary rules), replaceable via config."),
("internal/treesitter/treesitter.go", "winner; see BUILDLOG.md).", f"winner; see {D}decisions.md)."),
("internal/treesitter/treesitter.go", "span over LF-normalized UTF-8 (schema/SPEC.md section 3);", f"span over LF-normalized UTF-8 ({D}graph-model.md);"),
("internal/treesitter/treesitter.go", "Extension mapping follows DESIGN.md\n// section 6.2 (TS/JS resolution", f"Extension mapping follows {D}check-semantics.md\n// (TS/JS resolution"),
("internal/treesitter/treesitter.go", "(schema/SPEC.md section 3).", f"({D}graph-model.md, spans and positions)."),
("schema/vocabulary.toml", "# Identity (the qualified ID, SPEC.md section 2) and span are universal and", "# Identity (the qualified ID, stricttools/docs/node-identity.md) and span are universal and"),
("schema/vocabulary.toml", 'description = "An anonymous callable capturing scope; identity per SPEC.md section 2.3."', 'description = "An anonymous callable capturing scope, identified by name hint, ordinal, and signature fingerprint."'),
("schema/profiles/go.toml", "pointer-ness normalized (SPEC.md 2.5).", "pointer-ness normalized, so changing a receiver between value and pointer keeps the ID."),
("schema/profiles/go.toml", 'notes = "Identity per SPEC.md 2.3."', 'notes = "Identified by name hint, ordinal, and signature fingerprint."'),
("schema/profiles/python.toml", 'notes = "Identity per SPEC.md 2.3 (name hint, ordinal, fingerprint)."', 'notes = "Identified by name hint, ordinal, and signature fingerprint."'),
("schema/profiles/python.toml", "resolution to workspace members per the DESIGN.md 6.3 order.", "resolution to workspace members per the import resolution order."),
("schema/profiles/ts-js.toml", "index collapsed (SPEC.md 2.2).", "index collapsed."),
("schema/profiles/ts-js.toml", "disambiguated by source-order index (SPEC.md 2.4).", "disambiguated by source-order index."),
("schema/profiles/ts-js.toml", "Specifier reduction per DESIGN.md 6.3 (TS/JS);", "Specifiers reduce to bare package names;"),
("schema/strictspec/findings.schema.toml", 'description = "Serialized qualified node ID (SPEC.md section 2)."', 'description = "Serialized qualified node ID (see stricttools/docs/node-identity.md)."'),
("schema/strictspec/config.schema.toml", 'description = "Gates strictcode.toml: the single configuration file (DESIGN.md section 12.3) — rule toggles/severities/thresholds, analysis modes, group toggles, and per-rule-shaped suppressions with mandatory reasons. Rule-ID validity (including tombstone rendering) and suppression-shape-vs-rule matching are consumer-native checks against the registry; path staleness is the stale-suppression rule."', 'description = "Validates strictcode.toml, the single configuration file (see stricttools/docs/config.md): rule toggles, severities, thresholds, and allow lists; analysis modes; group toggles; and suppressions in each rule\'s declared shape, each with a mandatory reason. Rule-ID validity (a retired rule\'s error shows its retirement record) and matching suppressions to their rule\'s shape are checked by the loader against the registry; suppressions naming things that no longer exist are the stale-suppression rule."'),
("schema/strictspec/vocabulary.schema.toml", 'description = "Gates schema/vocabulary.toml:', 'description = "Validates schema/vocabulary.toml:'),
("schema/strictspec/profile.schema.toml", 'description = "Gates schema/profiles/*.toml:', 'description = "Validates schema/profiles/*.toml:'),
("schema/vocabulary.toml", "# A strictspec-gated document (schema/strictspec/vocabulary.schema.toml).", "# A strictspec document, validated by schema/strictspec/vocabulary.schema.toml."),
("schema/profiles/python.toml", "# A strictspec-gated document (schema/strictspec/profile.schema.toml).", "# A strictspec document, validated by schema/strictspec/profile.schema.toml."),
("schema/profiles/go.toml", "# A strictspec-gated document (schema/strictspec/profile.schema.toml).", "# A strictspec document, validated by schema/strictspec/profile.schema.toml."),
("schema/profiles/ts-js.toml", "# A strictspec-gated document (schema/strictspec/profile.schema.toml).", "# A strictspec document, validated by schema/strictspec/profile.schema.toml."),
("schema/strictspec/config.schema.toml", "Python call-resolution layer (DESIGN.md section 8).", "Python call-resolution layer (see stricttools/docs/call-resolution.md)."),
("schema/strictspec/config.schema.toml", 'description = "A file path (Python, TS/JS) or package directory (Go), relative to the project root."', 'description = "A file path (Python, TS/JS) or package directory (Go), relative to the workspace root."'),
]

def main():
    mode = sys.argv[1] if len(sys.argv) == 2 else ""
    if mode not in ("--dry-run", "--apply"):
        sys.exit("usage: repoint_doc_refs.py --dry-run | --apply")
    texts = {}
    for path, old, new in R:
        text = texts.setdefault(path, Path(path).read_text())
        count = text.count(old)
        if count != 1:
            sys.exit(f"{path}: expected 1 match, found {count}: {old!r}")
        texts[path] = text.replace(old, new)
        if mode == "--dry-run":
            print(f"{path}\n  - {old}\n  + {new}")
    if mode == "--apply":
        for path, text in texts.items():
            Path(path).write_text(text)
    print(f"{len(R)} replacements across {len(texts)} files ({mode})")

main()
