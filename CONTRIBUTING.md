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
mise run test
mise run build
```

This module builds independently of the private engine and has no external Go
dependencies. Public SDK/host release assets are produced by the engine workflow;
you do not need engine access or signing credentials to contribute format fixes.

## Compatibility

`format/cartridge/` and `format/level/` define data consumed by existing games and
runtimes. Preserve decoding bounds, validation, and existing serialized layouts.
Include round-trip and malformed-input coverage for behavior changes. Discuss
incompatible format changes before implementation and make version changes
explicit; do not silently reinterpret existing data.

Go module tags (`vVERSION`) and downloadable engine releases (`sdk-vVERSION`)
serve different purposes. See the [overview](README.md).

## Sending a pull request

Keep each pull request focused on one problem. Explain the user-visible result,
include a reproduction for a bug where possible, and list your validation results.
Add regression coverage when it protects changed behavior. Run `mise run fmt`
for Go changes and review the result before submitting. Preserve unrelated work.

Dependabot proposes weekly Go module and GitHub Actions updates. Minor and patch
updates are grouped; major updates stay separate. Changes to pinned tools in
`mise.toml` and SDK-selected toolchains are reviewed manually.

## License

Contributions are made under this repository's [MIT license](LICENSE.md).
Only submit material you have the right to contribute, and retain existing
third-party copyright and license notices.
