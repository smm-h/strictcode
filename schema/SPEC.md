# strictcode graph schema — normative specification

**Version: vocabulary format_version 1 (see `vocabulary.toml`).**
Status: first draft, designed for iteration. Every structure here is a strictspec-gated
document; being wrong is a migration, not a scar. Speculative parts are labeled
`maturity = "speculative"` in the vocabulary — their existence is settled, their shapes are
provisional by declaration.

This document is the prose half of the schema. The machine truth is:

- `schema/vocabulary.toml` — node kinds, row kinds, attributes, enums, capabilities, layers
- `schema/profiles/{python,go,ts-js}.toml` — per-language profiles
- `schema/strictspec/*.schema.toml` — the strictspec schemas gating all of the above plus the
  findings output format

Where prose and TOML disagree, the TOML wins and the prose has a bug.

## 1. The model: an interaction relation, not a graph

The primary artifact of extraction is a flat typed **interaction relation**: an ordered set of
rows, each

```
(row_kind, src_node, dst_node, file, span, attrs...)
```

The semantic graph used by algorithms (Tarjan SCC, reachability), the findings feed, and the
JSON output are all **pure deterministic projections** of this relation. None of them is
primary; no consumer touches rows it wasn't handed through a projection.

- The **algorithm graph** projects distinct `(src, dst)` pairs per row kind.
- The **site feed** projects rows with their spans and attributes for findings and fixes.
- **Canonical form** is defined once, on the relation: rows sorted by the total key
  `(row_kind, src_id, dst_id, file, span_start, span_end, attr-tuple)`, serialized
  canonically, hashed (SHA-256). Every projection inherits determinism from this.

Nodes live in a companion **node table**: `(node_kind, id, attrs...)`, sorted by
`(node_kind, id)`, included in the canonical hash.

## 2. Node identity

Public node identity is a **hierarchical qualified name**. Location is an attribute, never
identity. Identity is reproducible from source alone (statelessness) and does not change when
a node's body is edited.

### 2.1 ID structure

Internally an ID is a structured sequence of segments; the serialized form is:

```
<lang>:<member>:<module>:<container-chain>
```

- `<lang>` — `py` | `go` | `ts` (one profile, one prefix; TS and JS share `ts`).
- `<member>` — workspace member name; `_` for single-project (non-workspace) scans.
- `<module>` — the module's **logical name** (§2.2).
- `<container-chain>` — `.`-joined names from module scope inward
  (`UserService.save`, `Outer.Inner.method`). Empty for module-kind nodes.

Serialization escaping: `%`, `:`, `.`, `#`, and whitespace inside a segment are
percent-encoded. Tools must treat IDs as opaque strings or parse the structured form from
JSON output (which carries segments as an array); string-splitting serialized IDs is
unsupported.

### 2.2 Module segment (sub-decision: logical identity, never file path)

- Python: dotted module path from the package root (`pkg.sub.mod`). The package root is
  discovered per workspace member (same discovery as the namespace map in DESIGN.md §6.3).
- Go: the package import path relative to the member's module path (`internal/parser`).
- TS/JS: no logical module identity exists; the workspace-member-relative file path with the
  extension removed and `index` collapsed to its directory (`src/util/strings`,
  `src/util` for `src/util/index.ts`). This is path-derived but rename-stable in the only
  sense TS allows.

A file move that preserves logical identity (Python module moved with its package, Go file
within its package) does not change IDs. Case is preserved; two IDs differing only by case
are a **hard error** at graph-build time (case-insensitive filesystem safety).

### 2.3 Anonymous units (sub-decision: name hint + ordinal + fingerprint)

Closures, lambdas, and anonymous functions get a synthesized final segment:

```
<name-hint|anon>~<ordinal>~<fp8>
```

- `name-hint`: the assigned variable, property, or keyword-argument name when one is
  syntactically derivable; otherwise the literal `anon`.
- `ordinal`: 0-based source-order index among same-hint anonymous siblings in the same
  parent.
- `fp8`: first 8 hex chars of SHA-256 over the unit's normalized signature text (parameter
  list, LF-normalized, whitespace-collapsed). The fingerprint exists so that ordinal drift is
  **detectable**: a suppression or reference whose ordinal now points at a unit with a
  different fingerprint is reported as stale (hard error), never silently misattributed.

### 2.4 Overloads and redefinitions (sub-decision: source-order index)

Same-name siblings in the same container (TS overload signatures, Python conditional `def`,
getter/setter pairs) are disambiguated with `#<n>` (0-based source order, omitted for `#0`).
Parameter types are deliberately **not** in the ID — editing a signature must not change
identity. Reordering same-name siblings changes IDs; accepted as the rarer event.

### 2.5 Go receivers (sub-decision: normalize pointer-ness)

Methods use the receiver type name as their container (`Parser.Parse`). Pointer-ness is
normalized away — Go forbids the same method name on both `T` and `*T`, so this is
collision-safe, and changing a receiver between value and pointer does not change identity.

### 2.6 Collisions

Any two distinct nodes producing the same serialized ID is a **hard error** at graph-build
time. No silent suffixing.

## 3. Spans and positions

Canonical position is `(start_byte, end_byte)` over the file's LF-normalized UTF-8 bytes.
Line/column (1-based) are derived at output time for humans and editors; they are never
stored as truth and never participate in canonical hashing beyond the byte span itself.

## 4. Vocabulary structure

### 4.1 Node kinds

Defined in `vocabulary.toml [[node_kinds]]`, each with `id`, `maturity`
(`stable` | `speculative`), `description`, and an attribute list. Kind-specific attributes
are typed by the vocabulary; extraction populates all attributes a profile declares.

Drafting decision recorded: **tests and decorators are not node kinds.** A test is a callable
with `is_test = true` plus a `validates` row; a decorator is a function plus a `decorates`
row. Relationships beat kind proliferation when the underlying thing already has a kind.

### 4.2 Row kinds

Defined in `vocabulary.toml [[row_kinds]]`, each with `id`, `maturity`, `src_kinds`,
`dst_kinds`, `description`, and typed attributes. Two attribute disciplines carried over from
the design rounds:

- `calls.resolution` ∈ `syntactic | type_informed | unresolved` — mandatory, no default.
- `conforms_to` carries three mandatory orthogonal attributes:
  `provenance` (`declared | derived | declared_external`),
  `discipline` (`nominal | structural`),
  `mechanism` (`inheritance | implements | embedding | satisfaction | register | protocol`).
  Declared rows are stored by extraction; **derived** rows (Go interface satisfaction, TS
  structural compatibility, Python Protocol conformance) are materialized lazily, only when
  an enabled check declares the requiring capability and the profile grants it. The
  declared−actual **gap** is a first-class projection, not a stored row.

### 4.3 Import-edge attributes

`imports` rows carry `test_context`, `guarded`, `type_checking` (booleans, mandatory).
Profile applicability: `guarded` is Python-only semantics (try-body of
`try/except ImportError`); `type_checking` covers Python `if TYPE_CHECKING:` and TS
`import type`. A profile that marks an attribute not-applicable emits it as `false` and the
matrix records the capability as n/a — the attribute never silently means something weaker.

## 5. Capabilities, layers, profiles

- **Fine-grained capabilities** are the unit of truth: things like
  `resolve-imports-internal`, `call-resolution-syntactic`, `conformance-derived`. Checks
  declare required capabilities; profiles declare per-capability status.
- **Layers** are named bundles of capabilities for human presentation and matrix grouping:
  `import-graph`, `full-semantic-graph`, `boundary-graph` (speculative). A layer is exactly
  its capability set; it carries no semantics of its own.
- **Profiles** (`schema/profiles/*.toml`) declare, per capability:
  `status = "supported" | "planned" | "not-applicable"`, with a mandatory `reason` for
  `not-applicable`. `planned` means designed but unimplemented (everything, today);
  `supported` is flipped only when the extractor lands with tests. The matrix is generated
  from profiles ∩ check requirements: a check whose capabilities are all `supported` is
  supported; any `planned` → planned; any `not-applicable` → n/a with the reason surfaced.
  A check requiring a capability a profile lacks entirely is a generation-time hard error
  (vocabulary and profiles are closed sets — no unknown capability names).

## 6. Versioning

All schema artifacts are strictspec documents with integer `format_version`, exact-match
gated, migrated declaratively. Vocabulary **content** versioning rides strictspec decision 32
(enum sourcing): the findings output schema bakes its `node_kind` / `row_kind` /
`capability` enum arms from `vocabulary.toml` via pinned selectors. Adding or removing a
vocabulary entry changes the baked arms, which is a format change of the findings schema,
which triggers strictspec's bump rule and flip-scan. There is no separate
`vocabulary_version` field.

## 7. Tier-1 fix verification (structural correspondence)

Post-fix verification never compares raw IDs or locations. Correspondence between the
pre-fix and post-fix relations is computed structurally: nodes match by
`(node_kind, container chain by matched parents, name, sibling index)`; the expected
post-fix relation is the pre-fix relation with the transform's declared row/node delta
applied. Verification asserts canonical-form equality between expected and actual post-fix
relations, ignoring span attributes for rows whose file was edited below the fix point.
A mismatch rolls back the fix and reports a tool bug.

## 8. What is deliberately not here

- Rule IDs and check definitions — rule catalog round (they will reference capabilities
  defined here).
- The config surface — config round (strictspec documents; suppression targets use §2 IDs).
- Extractor behavior (how tree-sitter queries produce rows) — implementation, governed by
  the lessons register in DESIGN.md §6.7.
