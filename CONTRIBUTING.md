# Contributing

orbit is built spec-first: a spec in `specs/`, named-case tests before code,
then the code, then the package doc. A change that needs no spec is a change
that has not been thought about. Read `AGENTS.md` and the package's
`PACKAGE.md` before you touch anything.

## the process

1. **Spec first.** A feature lands with `specs/SPEC_<FEATURE>.md` in the
   format of the existing specs (`specs/SPEC_CLIENT.md`: definition,
   boundaries, decisions, testing, scope), written before the code. A spec
   changes only when its deliverable changes, and that change is named in
   the PR.
2. **Tests before code.** Named-case tests first; boundary cases by name
   (empty, first, last, max, off-by-one). A test that never failed proves
   nothing. A test's name carries its invariant.
3. **One surface, one PR.** The design test: a new tool, read, or write is
   a typed row plus a builder and one registration line at the composition
   root. The client is the one seam to the chain.
4. **Keep the docs current.** Every package has a `PACKAGE.md` that is its
   spec file; change it when behavior changes. `README.md`, `specs/`, and
   `CHANGELOG.md` move with the code.

## house rules

- **No comments in Go**, implementation or tests: one rule everywhere,
  because a small model reads the repository as one corpus and cannot hold
  "allowed here, forbidden there". A test's name carries its invariant; the
  `PACKAGE.md` carries the English. The only `//` lines are compiler
  directives. Exempt: generated code and the metadata packages.
- **The devnet gate holds.** Every write is devnet-only, structurally
  (`AllowWrite`), and the operator's authority key never enters the
  process. A change that widens the gate is refused.
- **Errors teach.** Every refusal names what was wrong and what would be
  right. Loud truncation, fail closed, no silent retries.
- **Security conscious.** This is a harness for untrusted model output over
  the chain's write path: deny by default, canonicalize untrusted input
  before acting on it, bound the work a caller can induce, fail closed on
  uncertain state.
- **Terse.** No filler, no ceremony. If a change needs a comment to be
  understood, rename. Lean and terse; a new pattern only if it is
  genuinely better.

## building and testing

```sh
make build       # go build -o bin/orbit ./cmd/orbit
make test        # go vet ./... && go test -race ./...
make fmt-check   # CI runs this; gofmt -l . must be empty
shellcheck install.sh
```

CI runs `go vet ./...`, `go test -race -p 2 ./...`, `make fmt-check`, and
`shellcheck install.sh`.

## versions and releases

- `cmd/orbit/main.go`'s `Version` const is the one source of truth.
  `TestVersionIsTheFreeze` asserts it, and the release workflow refuses a
  tag that does not equal `v` + `Version`.
- `CHANGELOG.md` gets a section for every release; the release body is that
  section, and a missing section refuses an empty release.
- Commit in the house style: the failing tests, the fixes, then "the
  version and the changelog".

## the daily driver

This harness is what the operator uses. A change should remove friction,
not add it: a feature is worth it if the daily driver will reach for it.
