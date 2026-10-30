# Binary performance profiles

`Config` and `Profile` are the shared cartridge/runtime performance contract.
`Default` supplies the high, medium, low and potato ladder. `Normalize` overlays
authored settings; `Encode` writes a resolved ladder and `Decode` validates it.

High, medium and low enable reflections by default; potato disables them.
Low retains its 0.75 render scale. An authored `floor_reflections` value overrides
the default for that profile.

Candidate SDK 0.0.11 keeps custom section `karty.performance.v1` and selects
binary version 2. The header is `KTPF`, u16 version 2, u16 reserved zero, followed
by target FPS f64, a u32-length UTF-8 default profile, and u32 profile count 4.
Each profile carries a length-prefixed name, render scale f64, minimum/maximum
window widths int32, and one optional reflection flag (0 absent, 1 false, 2 true).
All numeric fields are little-endian. JSON, unknown flags, bad bounds, nonfinite
values, trailing data and incomplete ladders are rejected. Decode allocates at
most four profiles from validated counts. Existing scale and FPS rules remain
unchanged; the CLI owns TOML authoring.

Width bounds are inclusive, and zero means unbounded. The resolved ladder must
cover every positive window width: adjacent intervals are accepted, but even a
single uncovered pixel width is rejected by normalization, encoding and decoding.
An individual default profile may be ineligible at a width; the host selects an
eligible alternative. This corrects candidate SDK 0.0.11 validation without
changing binary version 2 or its serialized layout.
