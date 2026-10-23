# Level envelopes and passive modules

`Encode` and `Decode` own the versioned, bounded KTYL level envelope.
`WrapModule` places a validated envelope in a canonical passive Wasm module,
with memory and the three level ABI exports. SDK 0.0.11 uses envelope/ABI
version 2 and rejects previous JSON metadata. Existing candidate KLDs require
rebuilding; released SDK hosts remain immutable.

`ValidatePassiveModule` rejects executable or noncanonical modules before a
consumer instantiates independently loaded child cartridges. Candidate SDK
0.0.11 also adds `DecodePassiveModule`: it performs the same validation and
returns the validated envelope without executing Wasm. Build caches can inspect
child metadata and payload contracts with this helper. Rejected modules never
expose an envelope, and errors retain `ErrPassiveModule` plus any underlying
envelope validation error.

Both passive helpers accept at most 32 MiB of module input containing at most
16 MiB of envelope data. The returned envelope borrows its raw, metadata and
payload bytes from the module. Retain the module and treat it as immutable for
the envelope's lifetime. Canonical reconstruction and the bounded entry table
allocate during validation; no guest memory or instructions are executed.

These helpers validate the transport contract. Consumers must additionally
validate mesh, lightmap and other named payload schemas against their parent
level and catalog before publishing resources.

`EncodeMetadata` and `DecodeMetadata` own the bounded KTYM version 2 binary
object. `Metadata` values use explicit typed constructors and accessors; custom
metadata keys remain supported without reflection. Canonical keys are sorted,
unique, 1–1024 UTF-8 bytes. The entire record is scanned before allocation,
within 16 KiB, 2048 values and root-plus-16 nesting levels. Numbers are finite
and preserve signed/unsigned integer types. JSON and trailing bytes are rejected.

## Binary metadata

`EncodeMetadata` and `DecodeMetadata` use typed `Metadata` objects and `Value`
constructors. Custom objects, arrays, booleans, strings, bytes, signed/unsigned
64-bit integers and finite 64-bit floats retain their types and precision.
There is no reflection, JSON parser or JSON compatibility fallback.

The header is `KTYM`, u16 version 2, u16 reserved zero. A tagged root object
follows: null 0, false 1, true 2, int64 3, uint64 4, float64 5, UTF-8 string 6,
bytes 7, object 8, array 9. Scalars are little-endian; strings/bytes have u32
lengths, objects/arrays have u32 counts. Objects encode sorted unique UTF-8 keys
as length-prefixed strings followed by their tagged values. Trailing bytes,
unknown tags, bad flags/versions and noncanonical keys are rejected.

Metadata has at most 16 KiB, 2048 values including its root, and depth 16 with
root depth zero. Keys contain 1–1024 UTF-8 bytes. Decoding scans the complete
payload and all nested bounds before allocating values. `ValidateMetadata`
performs this scan without allocating nested values. Authored YAML/TOML parsing
belongs to the CLI and is not a runtime dependency.

`PassiveModuleIdentity` returns the SHA-256 and length of canonical `WrapModule`
output from an envelope without constructing another full payload. Passive
decoding compares the small canonical ABI prefix and borrows the envelope bytes.
