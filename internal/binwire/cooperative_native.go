//go:build !js || !wasm

package binwire

func checkpoint(_, previous int) int { return previous }
