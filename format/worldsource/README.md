# World authoring semantics

`worldsource` defines typed room, prefab, port, content and material authoring
semantics. It has no YAML implementation dependency: tags describe field names,
and `Validate` checks typed values. Public compilers own text parsing, presence
and unknown-field checks, diagnostics, prefab expansion and convex decomposition.
They must enforce compiled-world bounds again after expansion.

The Karty CLI generates local YAML editor schemas with `karty schema`. Run it
from a game project to include that level's material, texture and prefab choices,
or from this repository root to generate the shared base referenced by the
standalone source fixture. The generated `.karty/schemas` directory is ignored;
format validation and builds do not require the CLI or a YAML parser here.

| Source version | Added semantics |
| --- | --- |
| 1 | Rooms, prefabs and ordinary connections |
| 2 | Typed actors and instance tags |
| 3 | Directed and transformed connections |
| 4 | Root authored lighting |
| 5 | Material UV mapping |
| 6 | Static solids, root/prefab contents and room-free prefabs |

Versions 1–6 remain supported. Optional fields must respect their version gates;
omitting new fields preserves older encodings. See the [compiled world
contract](../world/README.md) for output validation and lighting bounds. Root
lighting remains in global coordinates and is never transformed by a prefab.

## Material mapping, version 1

SDK 0.0.8 provides `world/material-mapping@1`. Authoring source v5
supports optional root `uv`, room `floor_uv`, `ceiling_uv`, `wall_uv`, and edge
`uv`. Source v1–4 reject those fields; omission preserves their previous
encodings and compiler behavior. Source v5 defaults to triplanar mapping with
world anchoring, one world unit per repeat, zero offset and zero rotation.
Controls inherit field by field from root to room to edge; nil numeric pointers
and empty mode/anchor strings inherit. Explicit `rotation_degrees: 0` overrides
an inherited rotation.

`mode` accepts `triplanar`, `planar`, `wrap`; `anchor` accepts `world`, `top`,
`bottom`. `scale: {x: ..., y: ...}` gives positive world units per repeat in
[0.001, 1,000,000]. `offset` is in repeats, with finite components bounded to
±1,000,000. `rotation_degrees` is finite and bounded to ±1,000,000. Wrap and
vertical top/bottom anchors apply to walls: top uses `ceiling(x,y)-z`, bottom
uses `z-floor(x,y)`. Explicit floor/ceiling wrap or top/bottom anchors are
invalid. Inherited root wrap falls back to planar on floors/ceilings; inherited
wall anchors do not move horizontal surfaces. The compiler resolves inheritance,
projection axes, scale, offset, rotation, anchoring and prefab transforms.

See [compiled mapping](../world/README.md#material-mapping-version-1) for the
resolved affine projections and capability requirements.

## Solids and loose contents, version 1

Source v6 adds optional `solids` and `contents` to root documents and prefabs.
Detail-only, sprite-only and nested room-free prefabs expand into existing rooms
without creating sectors. Older source versions reject these fields. Omission
preserves existing encoded bytes. The source solid count is at most 1,024 across
root solids and prefab definitions; compilers enforce it again after expansion.

Source material names resolve through packaged textures. Nested prefab scale,
yaw, translation and material overrides apply to solids and loose sprites. The
compiler assigns loose contents to the first containing compiled sector in
stable order, including shared boundaries and elevation. Source caps inherit
horizontal root mapping defaults; source `side_uv` is restricted to shared
planar/world X/Z mapping. Per-edge wrap and varying side triplanar weights are
not expressible by a single SideUV and are rejected. Mapped solids require the
existing `world/material-mapping@1` marker/capability; hosts also require
`world/static-solids@1`. No new guest command or level envelope version is added.

See [compiled solids](../world/README.md#static-solids-version-1) for exact
geometry, material and collision bounds.
