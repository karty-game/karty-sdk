# Enclosure planning policy v1

Accepted 2026-10-24 for SDK module v0.0.19.

`Options.EnclosureAware` is an additive build-policy opt-in. Its zero value
preserves existing planning; the CLI enables it by default and changes its
private chunk-cache revision. It changes partition ownership and baked artifact
identities without changing world, mesh, catalog or level envelope formats.
Existing compiled artifacts retain their original meaning.

Infer canonical sector groups from ordinary physical adjacency and authored
convex-decomposition provenance, with conservative coplanar broad-opening
fallback. Explicit `SectorGroups` remain hard boundaries. Identical floor and
ceiling planes and sky classification are required; distinct indoor spaces can
still be ambiguous. Width classification is a heuristic relative to cell area,
not an authored doorway declaration. Authors retain explicit area overrides.

Try a whole connected unassigned enclosure first. When exact budgets require
splitting, prefer the same enclosure and then wider shared edges. Do not take
part of a different enclosure merely to fill spare capacity. Growth remains
connected and deterministic. Every proposal keeps existing sector-count,
decoded/encoded mesh and lightmap capacity limits and requested density.
`Options.MaxSpan` adds an optional finite nonnegative horizontal render bound;
zero preserves SDK compatibility. The CLI defaults to 48 metres per X/Y axis
and accepts a positive `[chunks] max_span` override, including 32 metres.
Bounds include owned solids. Every trial, including mesh-only plans and final
whole-chunk compaction, enforces it. A single oversized sector fails explicitly
rather than bypassing the cap or reducing density. Geometry subdivision or the
separate spatial-surface policy is required for oversized source faces. The CLI
also checks spatial tile size and cropped terrain; outdoor levels can raise the
limit explicitly. Multi-sector terrain overflow requests smaller primary groups
before baking, while an oversized indivisible owner fails explicitly.

Complete neighboring enclosures can share a chunk when their combined bounds
and all resource budgets fit. Whole-chunk lightmap compaction may still combine
complete groups without splitting them. No minimum chunk-count or loading-time improvement is promised.

`PlanResult.Enclosures` maps sectors to the lowest global sector index in each
inferred enclosure, independent of resource partitioning. It is not serialized;
it makes the distinction available to build consumers. It does not implement
saved reflection cubemaps, probe capture positions, projection boxes, blending
or runtime enclosure ownership. Those require a separate format/runtime decision.
A nonrectangular group cannot be treated as a valid projection box automatically.

Coverage checks deterministic grouping, interleaved authored order, physical and
explicit boundaries, unchanged legacy behavior, budget-enforced splits, exact
mesh bytes, unchanged density and identity through lightmap compaction.
Span tests cover exact-boundary merges, splitting along either horizontal axis,
invalid/nonfinite limits and oversized single-sector rejection in both lighting
modes, including final compaction.
