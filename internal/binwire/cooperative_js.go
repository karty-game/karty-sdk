//go:build js && wasm

package binwire

import "time"

// Bound browser event-loop work during a large typed scan/read/write. Small
// payloads retain their ordinary synchronous path.
func checkpoint(position, previous int) int {
	if position-previous < 256*1024 {
		return previous
	}
	time.Sleep(time.Millisecond)
	return position
}
