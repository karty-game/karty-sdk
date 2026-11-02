package worldgpuchunk

import "math"

func runPayloadCheckpoint(checkpoint func() error) error {
	if checkpoint != nil {
		return checkpoint()
	}
	return nil
}

// Preserve existing static renderer admission without normalizing or generating
// any field. Skinning and vertex RNM fields are absent in this static-only ABI.
func validStaticVertexFrame(vertex []byte) bool {
	normal := [3]float32{readF32(vertex, 12), readF32(vertex, 16), readF32(vertex, 20)}
	tangent := [3]float32{readF32(vertex, 24), readF32(vertex, 28), readF32(vertex, 32)}
	dot := func(a, b [3]float32) float32 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
	if math.Abs(float64(dot(normal, normal)-1)) > 1e-4 || math.Abs(float64(dot(tangent, tangent)-1)) > 1e-4 ||
		math.Abs(float64(dot(normal, tangent))) > 1e-4 || (readF32(vertex, 36) != 1 && readF32(vertex, 36) != -1) {
		return false
	}
	return readF32(vertex, 80) == 0 || readF32(vertex, 80) == 2
}
