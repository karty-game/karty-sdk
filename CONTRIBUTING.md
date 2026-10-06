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
[overview](README.md) for tool ownership and exclusions.

This module builds independently of the private engine. Its two external Go
dependencies are the pinned QOI and QOA codecs; retain their license notices.
Public SDK/host release assets are produced by the engine workflow;
you do not need engine access or signing credentials to contribute format fixes.

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
tests. To investigate a target further, run the corresponding `go test -fuzz`
command through `mise exec` with a longer budget.

The pinned Node tool runs Prettier and Go's Wasm tests through the official Go
runner; runtime format/codec packages keep their Go-only dependencies.

Use `mise run bench` for fixed allocation and timing fixtures. Record the Go
version, platform, CPU and exact command when comparing results. `B/op` measures
total allocation per operation, not peak process memory. Encoding, actual WASM
execution and browser graphics are separate checks; report only what ran.

Dependabot proposes weekly Go module and GitHub Actions updates. Minor and patch
updates are grouped; major updates stay separate. Changes to pinned tools in
`mise.toml` and SDK-selected toolchains are reviewed manually.

## License

Contributions are made under this repository's [MIT license](LICENSE.md).
Only submit material you have the right to contribute, and retain existing
third-party copyright and license notices.
