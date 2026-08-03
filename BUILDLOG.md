# strictcode foundation build log

Durable record of every material decision and every deviation from the design
corpus (`DESIGN.md`, `CATALOG.md`, `schema/SPEC.md`) made during the autonomous
foundation build (started 2026-08-03). Per DESIGN.md §12 ("Foundation build
execution"), the builder session has full decision-making authority inside this
repository and must record its decisions here.

Format: dated entries, newest last. Each entry states the decision, the
rationale, and whether it deviates from a design document.

---

## 2026-08-03 — Scaffolding

**Go module and toolchain.** Module `github.com/smm-h/strictcode`, `go 1.26.3`
(the installed toolchain). No deviation.

**LICENSE.** Apache 2.0, verbatim from apache.org. Explicitly approved by the
owner in the build charter; DESIGN.md §11 lists it as a settled fact. No
deviation.

**SARIF todo.** Filed directly in `todo/.defer/` per DESIGN.md §10 (SARIF output
deferred; "todo filed in `todo/.defer/` at repo scaffolding time").

**rlsbl scaffold.** Ran `rlsbl scaffold --target go --publish-mode ci
--no-auto-tag --yes`. Decisions:

- `--no-auto-tag`: the auto-tag flag writes a GitHub topic to the remote repo.
  The build charter forbids touching the remote in any way (nothing is pushed
  before the first release), so the topic tag is left for the release session.
- The pre-existing `.github/workflows/publish.yml` was the hand-written PyPI
  placeholder workflow (manual dispatch, used for the name claim). Scaffold
  refuses to merge over a file it has no base for; the placeholder workflow was
  renamed to `.github/workflows/pypi-placeholder-publish.yml` (it remains
  useful if the placeholder ever needs republishing) and scaffold now owns
  `publish.yml` for the Go target.
- Scaffold detected the go target as `artifact: library` (no binaries). The
  DESIGN §12.5 goreleaser-binaries distribution is a release-round concern;
  the scaffold config can be extended then.
