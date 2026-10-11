//go:build !js || !wasm

package worldchunk

type cooperativeWork struct{}

func (*cooperativeWork) checkpoint(int) {}
