// Package actions exposes the editor schema only; it is not a runtime format.
package actions

import _ "embed"

//go:embed schema.json
var Schema []byte
