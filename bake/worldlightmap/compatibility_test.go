package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

// Pin existing producer pixels and reflectance identity across internal changes.
func TestProducerCompatibility(t *testing.T) {
	cases := []struct {
		barrier             float64
		bounces             int
		pixels, reflectance string
	}{
		{0, 0, "860841426310d3f88b43339628051c0c478d64dfaadfac26b7fb2b1e600ee096", "8938e79f958dead37f23d2e68b3f0e04a8a83e6c86e070ef477542d5e5af5a46"},
		{0, 2, "1e8d9f6dd63953ccfaf3ccc7526623cc04706aa5bbfd89b238d109485aa90e4e", "8938e79f958dead37f23d2e68b3f0e04a8a83e6c86e070ef477542d5e5af5a46"},
		{1.5, 0, "6110397e8817c6beeb0cc0ddd97ddce45a6f9dad26bf2e930bd1bb7b1a55720c", "1c755c5bc2bdea2610cd434052091b501606ea85c150cfb6ff8726ba6a219705"},
		{1.5, 2, "59a031f5756a4739a8dc8ee13decfe4ade8e6904fa235fa65e2a9541e91857d7", "1c755c5bc2bdea2610cd434052091b501606ea85c150cfb6ff8726ba6a219705"},
		{3, 0, "1a8ae922df495b38483bc1d5772a96439d4ec3ab140fa31020fc426b33282a11", "1c755c5bc2bdea2610cd434052091b501606ea85c150cfb6ff8726ba6a219705"},
		{3, 2, "807bb71aa8cbb7954b7e48312372776db326a390757f3c1ef3e48226afc71291", "1c755c5bc2bdea2610cd434052091b501606ea85c150cfb6ff8726ba6a219705"},
	}
	for _, test := range cases {
		t.Run(fmt.Sprintf("barrier-%g-bounces-%d", test.barrier, test.bounces), func(t *testing.T) {
			d, l, materials := room(t, test.barrier)
			result, err := Bake(context.Background(), d, l, materials, Options{Samples: 16, Bounces: test.bounces, Workers: 1, Seed: 22})
			if err != nil {
				t.Fatal(err)
			}
			if pixels := fmt.Sprintf("%x", sha256.Sum256(result.Image.Pix)); pixels != test.pixels {
				t.Fatalf("pixels changed: %s", pixels)
			}
			if result.ReflectanceSHA256 != test.reflectance {
				t.Fatalf("reflectance changed: %s", result.ReflectanceSHA256)
			}
		})
	}
}
