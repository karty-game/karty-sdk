package worldlightmapbake

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

// MaxShadowTriangles bounds optional build-only opaque absorbing geometry.
const MaxShadowTriangles = 4096

func validateShadowTriangles(triangles [][3]world.Vec3) error {
	if len(triangles) > MaxShadowTriangles {
		return fmt.Errorf("shadow geometry exceeds %d triangles", MaxShadowTriangles)
	}
	for i, points := range triangles {
		for _, point := range points {
			for _, value := range [...]float64{point.X, point.Y, point.Z} {
				if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > world.MaxCoordinate {
					return fmt.Errorf("shadow triangle %d has invalid coordinates", i)
				}
			}
		}
		if degenerate(triangle{a: points[0], b: points[1], c: points[2]}) {
			return fmt.Errorf("shadow triangle %d is degenerate", i)
		}
	}
	return nil
}

// Empty input writes no bytes, preserving existing producer cache identities.
func writeShadowTriangles(w *binwire.Writer, triangles [][3]world.Vec3) {
	if len(triangles) == 0 {
		return
	}
	w.String("opaque-shadow-triangles@1", 128)
	w.Count(len(triangles), false, MaxShadowTriangles)
	for _, points := range triangles {
		for _, point := range points {
			writeInputVector(w, point)
		}
	}
}

// Hash caller-owned geometry rather than retaining its mutable slice in Baker.
func shadowTrianglesDigest(triangles [][3]world.Vec3) [32]byte {
	if len(triangles) == 0 {
		return [32]byte{}
	}
	h := sha256.New()
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], uint64(len(triangles)))
	h.Write(encoded[:])
	for _, points := range triangles {
		for _, point := range points {
			for _, value := range [...]float64{point.X, point.Y, point.Z} {
				binary.LittleEndian.PutUint64(encoded[:], math.Float64bits(value))
				h.Write(encoded[:])
			}
		}
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}
