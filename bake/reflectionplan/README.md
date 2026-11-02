# Offline reflection planning

`Plan(document, digest)` creates deterministic `worldreflection.Bundle` metadata
for a validated compiled world. It groups connected indoor geometry, chooses
clear capture origins and projection bounds, and binds sectors and static solids
to sparse probe IDs. Connected coplanar liquid sheets retain one environment.

The caller supplies the exact reflection input digest. Probe images remain empty
until the offline capture fills them; capture encoding and GPU image preparation
remain separate build steps. This package performs no image work and is never a
runtime loading or rendering dependency.

The algorithm and producer identity are unchanged from the existing capture
planner. Moving ownership into this public SDK package lets the CLI plan build
batches before capture without importing engine source or adding a digest cycle.
Engine capture delegates to the same implementation. Existing KRC1/KRG1 wire
contracts, IDs, owner bindings and placement remain unchanged.

The implementation is covered by the SDK module's MIT license. Tests use public
SDK types and standalone geometry; no engine or sample checkout is required.
