# Build-time texture atlases

`texture/atlas@1` uses QOI RGBA/sRGB pages with a `KTA1` binary layout. Builds
emit `@atlas/layout` and zero-based `@atlas/page/00000000` entries. The entire
`@atlas/` namespace is reserved. This format is introduced by public module
v0.0.16 and candidate engine SDK 0.0.15.

The standard 16-byte binary header precedes a count and width/height int32 pairs
for pages, then a region count. Each region contains a bounded UTF-8 logical name
and seven int32 fields: page, x, y, width, height, gutter, filter. A mip count
and up to nine mip rectangles (x/y/width/height int32 fields) follow. Filter 0
uses nearest sampling with no mip chain; filters 1/2 store independently
filtered cutout-color/normal chains down to a minimum side of eight pixels.
Mips share their base image's page and use two-pixel extruded gutters. All
rectangle records, including aliases and mips, are bounded to 8192. Tables contain at most
4096 records each, names at most 1024 bytes, and the layout at most 1 MiB. Names
are strictly sorted. Level names use canonical `@texture/xxxxxxxx` IDs; game
names preserve the authored logical name.

Pages obey the existing image limits (8192 per dimension, 16 million pixels,
8 MiB encoded per page, 256 MiB decoded across pages). Every page is referenced.
Regions include their two-pixel extruded sampling gutters in overlap/bounds
checks. Exactly identical rectangles may alias. A zero-gutter region is valid
only for a dedicated full-page image. Hosts validate the complete bundle before
allocating decoded pixels. A page is decoded/uploaded once; region consumers
borrow views, preserving dimensions and straight-alpha pixels.

`ReadEntries` rejects incomplete, duplicate and unknown reserved entries.
`DecodeLayout` preflights the complete stream before allocating tables. Producers
must additionally preserve the enclosing cartridge/level entry and byte limits,
require `texture/qoi@1`, and keep connected particle frames on the same page.
