# Binary action documents

Candidate SDK 0.0.11 stores `karty/actions@1` as binary `KTYA` version 2.
`Encode`, `Decode` and `Validate` use typed `Document`, `Sequence` and `Step`
values; arguments use `level.Metadata`. JSON payloads are rejected without a
compatibility fallback. YAML authoring belongs to the CLI; the editor JSON
Schema lives separately in `authoring/actions` and is not linked by this package.

A step specifies one action with typed literals, one condition with then/else
branches, or a positive wait. Action/condition failures can run onFailure steps.
Actor arguments are typed objects with one string `actor` naming the authored
content in the current level mount. Go function names and runtime ECS IDs are
absent. Repetition is ignore (default), restart or parallel.

`ActorRef` describes this level-scoped name for callers. It is not a runtime ECS
handle and has no separate binary encoding. To include it in `Step.Args`, use
`level.ObjectValue(level.Metadata{"actor": level.StringValue(ref.Actor)})`.

The little-endian header is `KTYA`, u16 binary version 2, u16 reserved zero.
It carries u32 document version 1, a u32-length UTF-8 schema string and u32
sequence count. Each sequence carries length-prefixed name/repetition strings
and a step list. Lists encode u32(length+1), with zero representing nil; empty
and absent condition branches stay distinct. Each step carries action and
condition strings, u64 wait frames, a length-prefixed binary metadata argument
object (zero length for absent), then/else/onFailure lists in that order.

Limits are 64 KiB encoded, 64 sequences, 1024 total steps, 16 branch levels,
16 arguments, and 1024 UTF-8 bytes per literal string or actor ID. Argument
objects also obey level metadata bounds. Names have at most 128 ASCII bytes;
waits range from 1 to 1000000000 frames. Decoding scans all nested counts and
argument objects before allocating branch arrays and validates the complete
batch before returning it. Invalid UTF-8, nonfinite numbers, malformed counts,
truncations, trailing bytes and invalid final steps are rejected.

`go run ./cmd/karty-actions-schema --out PATH` exports the standard editor
schema snapshot. Consumer snapshots can be checked without engine credentials.
