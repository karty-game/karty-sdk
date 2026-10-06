# Authored actions format v1

`actions@1` is an optional level data entry named `karty/actions@1`. It contains
an actions document with version 1 and named sequences. The canonical
`schema.json` defines the serialized shape and supplies editor metadata; the
CLI derives action-specific argument and actor-choice schemas from game
declarations. This package validates format structure and bounds independently
of game declarations or private engine source.

Each step specifies an action with typed argument literals, a condition with
then/else branches, or a positive `waitFrames`. Actions/conditions may provide
onFailure steps. Actor literals are `{ "actor": "compiled/content/id" }`, scoped
to the current level mount. Go function names and runtime ECS IDs are absent.
Named repetition is ignore (default), restart or parallel. Runtime dispatch,
cancellation and activation belong to the engine; catalog extraction and
static target/type checking belong to the CLI.

Documents are bounded at 65536 bytes, 64 sequences, 1024 total steps and 16 branch
levels. Identifiers have at most 128 ASCII bytes, start with a letter and continue
with letters, digits, dots, underscores or hyphens. Arguments have at most 16
named values; strings and actor IDs have at most 1024 UTF-8 bytes. Waits range
from 1 to 1000000000 simulation frames. Duplicate names/JSON keys, unsupported
fields/types, invalid UTF-8, non-finite numbers and deeper/larger documents fail.
Go declaration-specific integer/float ranges are checked by the CLI.

The SDK 0.0.9 candidate adds this optional feature; older level/world layouts
remain unchanged. Clients must select a supporting SDK bundle and matching host.

`go run ./cmd/karty-actions-schema --out PATH` exports an exact schema snapshot.
Engine bundles and CLI test fixtures track generated copies. Optional sibling
parity tests detect drift without making public builds depend on those siblings.
