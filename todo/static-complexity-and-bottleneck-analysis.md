# Static complexity and bottleneck analysis — deterministic Big-O over the semantic graph

## Context

A recent session in another project found a quadratic execution path in
a Go engine only after writing a scale benchmark: a full deep copy of a
growing structure sat inside a per-item loop, so cost per item grew
linearly with items already processed. The pattern was statically
visible the whole time — a clone call reachable inside a loop whose
trip count tracks the structure being cloned — but nothing on the
machine could point at it. Discovering it dynamically cost a benchmark
harness, a 20-second run, and an allocation-profile read; a static
analyzer would have named it from the source alone.

strictcode already builds exactly the substrate this needs: a
deterministic semantic graph of callable units and type units with
call/reference/instantiation edges. Complexity analysis is a rule
family over that graph plus per-callable control-flow structure.

## Problem

Build a deterministic, statically-derived complexity report: point the
tool at any codebase and it finds, without executing anything —

- **Asymptotic cost annotations per callable**: worst-case time and
  space bounds (Big-O style) derived from loop structure, recursion
  shape, and the known costs of called units, composed bottom-up over
  the call graph.
- **Recursion**: every cycle in the call graph (direct and mutual),
  reported as its strongly-connected component, with the recursion
  classified where decidable (structural descent visible vs unknown
  termination).
- **Loop analysis**: nesting depth; what each loop iterates over
  (a collection parameter, a growing structure, a constant range);
  loops whose body calls something whose cost depends on the very
  structure the loop grows — the quadratic-accumulation pattern from
  the motivating incident.
- **Expensive-operation-in-loop findings**: allocation in hot loops,
  deep copies, linear scans inside linear loops (O(n) lookup where a
  map exists), repeated recomputation of loop-invariant calls.
- **Bottleneck candidates**: the callables whose derived bound times
  their call-graph fan-in makes them the likely cost centers, each
  with the full evidence chain (which loop, which callee, which edge).

**Honesty requirement (the hard constraint).** Precise worst-case
complexity is undecidable in general. The analyzer must therefore be
sound-and-honest, never guessing: every bound is an upper approximation
with its derivation recorded; where a bound cannot be derived (dynamic
dispatch the graph cannot resolve, unbounded recursion without visible
descent, loops over unknowable trip counts), the verdict is an explicit
"unknown" carrying the reason — never a silently omitted callable and
never a fabricated bound. This is the no-silent-degradation philosophy
applied to analysis results.

## Output: one open, structured format

The analysis result is a single structured JSON document (a declared,
versioned schema — the schema toolchain this project already uses for
its spec files is the natural authority for it). Every finding carries
detailed addresses: file, line span, callable path, and the evidence
chain (loop → callee → bound). Three consumers, by design:

A. **Visual rendering**: the document carries everything needed to
   render call-graph/cost diagrams and properly typeset bounds
   (LaTeX-ready math for the Big-O expressions), so a renderer needs
   no re-analysis.
B. **Prose generation**: a deterministic prose projection readable by
   humans and LLMs alike — what is slow, why, where exactly (the
   detailed addresses), and what kind of change would resolve it or
   where to look next. This is the report an agent reads before
   optimizing.
C. **Anything else**: the format is open and self-describing, so any
   downstream tool (CI thresholds, diff-over-time cost tracking,
   editor annotations) consumes it without coordination.

## Solution sketch

1. Per-callable control-flow extraction (loops, branches, recursion
   edges) on top of the existing graph extraction.
2. A cost algebra: constant / log / linear-in(x) / product / max /
   unknown(reason), with symbolic size variables tied to parameters
   and receiver structures so bounds compose across calls.
3. Bottom-up propagation over the call graph's condensation (SCCs
   collapse to recursion verdicts first).
4. The rule family for named findings (clone-in-loop, scan-in-loop,
   invariant-recomputation, quadratic accumulation), each a graph
   pattern with an evidence chain.
5. The JSON schema + the prose projection + a minimal graph/LaTeX
   emitter as the reference consumer.

Pros: deterministic and repeatable (same input, same report); catches
the whole class the motivating incident belongs to before any
benchmark exists; the structured output composes with the tiered
finding/fix model (these findings are naturally tier 3 — explain,
don't auto-fix). Cons: sound bounds over real code need a real
interprocedural size analysis — the cost algebra and the honesty rule
are the design work; over-approximation will flag some theoretically-
quadratic-but-practically-fine paths (severity/threshold config is the
answer, not silence).

## Affected files

New analysis stage + rule family in the analyzer core; the JSON schema
under the project's spec-file convention; the prose projection; docs.

## Effort

Large — a design round for the cost algebra and the JSON schema first
(the honesty rule and symbolic sizes are the hard part), then the rule
family incrementally; the clone-in-loop / scan-in-loop pattern subset
alone is a shippable first slice with immediate value.
