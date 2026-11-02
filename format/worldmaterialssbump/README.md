# World material SSBump companion, version 1

Versioned decision: SDK candidate **0.1.0** introduces the optional
`world/material-ssbump@1` runtime capability and cartridge requirement. Existing
SDK 0.0.20 artifacts are immutable. An older host must reject the unknown
requirement rather than display coefficients as a normal map. The requirement
depends on `world/sectors@1` and `world/material-atlas@1` or `@2`.

Each level carrying a companion declares metadata key `kartyWorldMaterialSSBump`
with the exact string value `world/material-ssbump@1`. Its cartridge declares the
global feature as well. A global declaration allows normal-only levels in the
same cartridge; it does not imply that every level has a companion. The marker
and complete required entry set must appear together. A missing, unknown or
mistyped marker, orphan companion entry, partial pair or undeclared companion
is invalid. Producers reserve all three logical entry names from authored data.

This is an independent companion. Material data remains RG tangent normal XY,
B height, A ambient occlusion. Reflectance remains R metallic, G smoothness,
B zero, A 255. Existing layouts, source documents and material IDs do not change.

The fixed logical level entries are:

| Entry                           | Content                                                                  |
| ------------------------------- | ------------------------------------------------------------------------ |
| `@world/material-ssbump-layout` | `KSB1` descriptor                                                        |
| `@world/material-ssbump`        | Linear RGBA QOI atlas                                                    |
| `@world/material-ssbump-mips`   | Linear RGBA QOI tail, required exactly when the existing layout has mips |

The descriptor contains the four magic bytes, SHA-256 of the complete canonical
`worldmaterial.EncodeLayout` result, a big-endian uint16 material count, then
big-endian uint32 material IDs. Counts are 1..196. IDs form a unique subset in
the owning layout's order. There are no trailing bytes. Decoders bound the
complete descriptor before allocation and validate its binding to the layout.

RGB are linear, unnormalized self-shadowed RNM coefficients. Basis order is
`(sqrt(2/3),0,sqrt(1/3))`, `(-sqrt(1/6),sqrt(1/2),sqrt(1/3))`,
`(-sqrt(1/6),-sqrt(1/2),sqrt(1/3))` in tangent coordinates. Image columns are
tangent +X and rows tangent +Y. Alpha is reserved 255, including padding. Flat,
unoccluded coefficients are `(85,85,85)`. Authored coefficient energy is not
restricted by an inferred byte-sum rule. Occlusion must not be normalized away.
Consumers evaluate `sum(coefficient[i] * incomingRNM[i])` and do not multiply
the same height-derived AO again. Normal data remains available for consumers
that do not select this detail mode, including initial model and terrain paths.

Atlas dimensions and material placements exactly match the existing layout.
Mip-tail dimensions also match; coefficients occupy the existing **data** mip
cells (`MipRect(level, slot, true)`). Unselected materials, outer padding and
all other tail cells contain flat coefficients. Selected cells include their
extruded gutters. Mips average coefficients linearly without renormalization.
All image streams must validate completely, have RGBA channels and a linear
QOI tag, and fit the shared decoded-texture budget together with the base pair.
No partially validated pair may be published or consumed.

The CLI producer generates each original material's periodic height field
independently before chart packing; unrelated packed charts never cast rays
into one another. Its versioned algorithm, generation controls and exact height
bytes identify cached output. Reprojected chart coefficients use the RNM basis
transfer and preserve occlusion energy before alpha compositing. Generation is
an artistic finite-radius approximation, not recovered physical surface shape.

Public dependency qualification uses a frozen, isolated 0.1.0 module/file-proxy
candidate and matching checksums before engine and CLI pins advance. It does not
publish a source tag or replace released 0.0.20 archives. Format tests, actual
runtime admission and rendered lighting qualification remain separate gates.
