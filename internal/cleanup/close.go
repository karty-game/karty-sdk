// Package cleanup provides best-effort release after a primary operation.
package cleanup

import "io"

// Close releases a reader or provides a deferred fallback after an error.
// Writers must separately check their final Close before reporting success.
func Close(closer io.Closer) {
	_ = closer.Close()
}
