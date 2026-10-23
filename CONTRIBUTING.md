# Contributing to Karty SDK

Contributions to the public cartridge and level formats shared by the CLI and engine are welcome. Small bug fixes,
documentation improvements, and reproducible bug reports are good starting points.
For a substantial feature or a compatibility change, open an issue first so we
can agree on scope before implementation.

## Local setup

Fork and clone this repository, install [mise](https://mise.jdx.dev/), then run
these commands from the repository root:

```sh
mise install
mise run fmt
mise run check-fmt
mise run lint
mise run test
mise run build
```

`mise install` also installs the repository hk pre-commit hook. Formatting and
lint use the same pinned hk configuration locally and in CI; see the
[formatting and tooling](#formatting-and-tooling) below.

This module builds independently of the private engine. Its two external Go
dependencies are the pinned QOI and QOA codecs; retain their license notices.
Public SDK/host release assets are produced by the engine workflow;
you do not need engine access or signing credentials to contribute format fixes.

## Formatting and tooling

The shared checks in `hk.pkl` use golangci-lint for Go (including `govet`),
yamllint for YAML, Taplo for TOML, and Prettier for YAML layout, JSON, Markdown
and web files. Taplo validates syntax without downloading schemas.

Use `mise run install-hooks` to reinstall the hook. It checks staged files and
applies formatting fixes while preserving unstaged work. `fmt` applies all
formatters through hk without staging; `check-fmt` reports differences without
writing files. `lint` runs all format and lint checks; `check` also runs tests
and build. Generated Go and `*.generated.*` snapshots, derived output, local
contributor directories and binary fixtures are excluded from formatting.
Authoring YAML fixtures retain their decoder input format; yamllint checks them.

Golangci-lint's Revive `file-length-limit` rule caps handwritten production Go
files at 500 physical lines, including comments and blank lines. Test files
(`*_test.go`) and files with standard generated-code headers are exempt. Split
code by responsibility within its owning package; do not remove useful comments
or add generated headers to handwritten files to satisfy the limit.
The uncached `mise run check-source-policy` guard also covers other platforms
and inactive build tags; both tests and Go lint run it.

Every `nolint` directive must name its rule and explain why it is necessary.
`nolintlint` checks this policy and reports unused directives. Staticcheck includes
its quick-fix checks. Bundled false-positive and unchecked-cleanup exclusions are
not enabled. Complete successful writes with a checked `Close`; the internal
cleanup helper is for readers and deferred fallback cleanup only. The remaining
codec-fixture `makezero` exceptions intentionally build headers before appending
payloads. Producer `errorlint` exceptions preserve the source error's identity
without exposing a secondary compiled diagnostic as another wrapped error.

After changing typed world/lightmap/material declarations, regenerate their
checked-in codecs with `mise exec -- go generate ./format/world`. Runtime
formats use bounded typed binary; tests reject JSON/reflection imports in format
and codec production code. Binary wire formats require rebuilding levels and
bake caches created with older JSON formats.

After changing `authoring/actions/schema.json`, regenerate consumer snapshots with
their `mise run generate-action-contract` tasks; public SDK checks remain
independent of private source.

The pinned mise environment enables Go 1.27 portable SIMD for the offline
baker. Direct Go builds importing that package require `GOEXPERIMENT=simd`;
format-only imports are unaffected. `mise run test-simd-emulated` exercises Go's
built-in fallback rather than a second production implementation.

## Compatibility

The `format/` packages define data consumed by existing games and runtimes.
Preserve decoding bounds, complete validation, and existing serialized layouts.
Include round-trip and malformed-input coverage for behavior changes. Discuss
incompatible format changes before implementation and make version changes
explicit; do not silently reinterpret existing data.

Keep published package paths stable. Prefer splitting large files within their
owning package to moving public types or mixing format contracts with runtime
code. Keep format helpers independent of private engine packages and credentials.
Producer optimizations must preserve pixel bytes, digest inputs and sampling
order; changes to those semantics require a versioned producer decision.

Go module tags (`vVERSION`) and downloadable engine releases (`sdk-vVERSION`)
serve different purposes. See the [overview](README.md).

## Sending a pull request

Keep each pull request focused on one problem. Explain the user-visible result,
include a reproduction for a bug where possible, and list your validation results.
Add regression coverage when it protects changed behavior. Run `mise run fmt`
for all Go packages and review the result before submitting. Preserve unrelated work.

`mise run test` and `mise run build` are the minimum checks. CI also runs
`check-fmt`, `lint`, `test-race`, native `test-32`, `test-wasm` and `fuzz`. Formatting
and lint run through hk with golangci-lint, yamllint, Taplo and Prettier. The 32-bit task requires an
x86 Linux host capable of executing 386 binaries; cross-compilation alone does
not validate integer-width behavior. `fuzz` gives each target five seconds of
actual exploration with two workers, beyond the seed corpus used in ordinary
tests. The task rejects missing or renamed targets before starting exploration.
To investigate a target further, run the corresponding `go test -fuzz`
command through `mise exec` with a longer budget.

`test-race` and `test-32` run one package at a time to bound CPU contention.
Dense baker producer compatibility fixtures run separately with the same
nine-second timeout. Race tests also isolate the large-world mesh entry fixture
from the other chunk tests so they do not share one package deadline. Every
race invocation retains race detection; native 386 tests also exercise Go's
SIMD fallback.

`test-wasm` executes Go Wasm tests using the official runner and pinned Node,
one package at a time to bound memory use. Its launcher passes a minimal
environment to stay within Go's 8 KiB argument/environment limit, preserving
temporary paths and the `GODEBUG` and `GOMAXPROCS` controls. It does not exercise
browser graphics.

Runtime format/codec packages keep their Go-only dependencies.

Use `mise run bench` for fixed codec, material-atlas, connected-world and bake
allocation and timing fixtures. Record the Go version, platform, CPU and exact command when comparing results. `B/op` measures
total allocation per operation, not peak process memory. Encoding, actual WASM
execution and browser graphics are separate checks; report only what ran.

Dependabot proposes weekly Go module and GitHub Actions updates. Minor and patch
updates are grouped; major updates stay separate. Changes to pinned tools in
`mise.toml` and SDK-selected toolchains are reviewed manually.

## License

Contributions are made under this repository's [MIT license](LICENSE.md).
Only submit material you have the right to contribute, and retain existing
third-party copyright and license notices.
