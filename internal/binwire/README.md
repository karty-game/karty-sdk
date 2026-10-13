# Typed binary primitives

The candidate runtime formats use static generated field codecs, without JSON
or runtime reflection. Runtime format types carry no JSON tags or custom
`MarshalJSON`/`UnmarshalJSON` methods; the SDK policy tests enforce this. `internal/codegen/binary` reads Go declarations at
build time; generated files remain checked into Git. Run
`mise exec -- go generate ./format/world` after changing an owned contract.
The regeneration test checks world, lightmap and material snapshots together.

The `worldsky` sidecar uses a handwritten static typed codec over these bounded
primitives. Its bulk packed-image fields use exact length prefixes and borrowed
`Raw` reads during complete preflight, then copy into owned decoded storage only
after every gzip member and aggregate resource bound passes. It keeps the
existing generated world codecs and frozen environment payload unchanged.

Each payload begins with 16 little-endian bytes: four-byte magic, uint16 binary
version 1, uint16 zero flags, uint32 exact payload byte length and uint32 zero
reserved. Readers reject unsupported versions, nonzero reserved fields,
truncation and trailing bytes before publication. Owning packages retain their
encoded limits and semantic validation.

Fields follow public Go declaration order. `int` uses signed int32; explicit
unsigned integers keep their widths. Floats use finite IEEE float32/float64;
negative zero is encoded as positive zero and rejected by the reader. Booleans
and pointer presence use one byte, exactly 0 or 1. Strings use uint32 byte
lengths followed by valid UTF-8. Slices use uint32 `length+1`, reserving zero
for nil; an explicit empty slice encodes count 1. Fixed arrays omit counts.

Generated scanners traverse the complete payload without allocating strings or
arrays. Count checks bound each field, its minimum encoded footprint and the
aggregate conservative 32 MiB allocation estimate before typed reads. The
estimate includes pointed records, slices and strings. Generated writers stop
on bounds failures. Browser Wasm scans/reads/writes yield after bounded byte
intervals; native code has no scheduling change. Full authored validation still
runs after decoding. A binary header never substitutes for semantic checks.

Magic values are `KWR1` world, `KLM1` lightmap layout, `KPB1` prebake manifest and
`KMA1` material layout. Internal digest identities use the same primitives with
separate domains: `KWG1` geometry, `KDI1` selected direct lights, `KOI1` offline
controls, `KRA1` reflectance assignments, `KLQ1` liquid geometry and `KSI1` shadow
selection. These are explicit replacements for the released JSON identities;
compiled artifacts and bake caches must be rebuilt. Semantic capability versions
and lightmap algorithms retain their own independent gates.
