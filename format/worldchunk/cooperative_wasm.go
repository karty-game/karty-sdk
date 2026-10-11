//go:build js && wasm

package worldchunk

import "time"

// Fixed binary records have bounded work. Sleep returns control to JavaScript;
// Gosched alone cannot service browser frames. Native decoding has no sleeps.
type cooperativeWork struct {
	units    int
	deadline time.Time
}

func (work *cooperativeWork) checkpoint(units int) {
	work.units += units
	if work.units < 32*1024 {
		return
	}
	work.units = 0
	if time.Now().Before(work.deadline) {
		return
	}
	time.Sleep(time.Millisecond)
	work.deadline = time.Now().Add(2 * time.Millisecond)
}
