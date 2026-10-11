package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

// Pin unchanged producer pixels and the candidate binary reflectance identity.
func TestProducerCompatibility(t *testing.T) {
	cases := []struct {
		barrier             float64
		bounces             int
		pixels, reflectance string
	}{
		{
			0,
			0,
			"860841426310d3f88b43339628051c0c478d64dfaadfac26b7fb2b1e600ee096",
			"90fcd2582f951eb4372a535d874d250ecccaa940720e0afe6f37f6557cd0f983",
		},
		{
			0,
			2,
			"1e8d9f6dd63953ccfaf3ccc7526623cc04706aa5bbfd89b238d109485aa90e4e",
			"90fcd2582f951eb4372a535d874d250ecccaa940720e0afe6f37f6557cd0f983",
		},
		{
			1.5,
			0,
			"6110397e8817c6beeb0cc0ddd97ddce45a6f9dad26bf2e930bd1bb7b1a55720c",
			"34c5895302153587a59d9e51c615952634083db493fb6a698b4b72a7f885dff2",
		},
		{
			1.5,
			2,
			"59a031f5756a4739a8dc8ee13decfe4ade8e6904fa235fa65e2a9541e91857d7",
			"34c5895302153587a59d9e51c615952634083db493fb6a698b4b72a7f885dff2",
		},
		{
			3,
			0,
			"1a8ae922df495b38483bc1d5772a96439d4ec3ab140fa31020fc426b33282a11",
			"34c5895302153587a59d9e51c615952634083db493fb6a698b4b72a7f885dff2",
		},
		{
			3,
			2,
			"807bb71aa8cbb7954b7e48312372776db326a390757f3c1ef3e48226afc71291",
			"34c5895302153587a59d9e51c615952634083db493fb6a698b4b72a7f885dff2",
		},
	}
	for _, test := range cases {
		t.Run(fmt.Sprintf("barrier-%g-bounces-%d", test.barrier, test.bounces), func(t *testing.T) {
			// Retain the original density and hashes; other behavior tests need
			// fewer receiver texels to exercise the same transport properties.
			d, l, materials := roomWithDensity(t, test.barrier, 4)
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
