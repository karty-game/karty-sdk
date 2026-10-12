package cartridge_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/cartridge"
)

func TestParticleCapabilityAndFeatureDependencies(t *testing.T) {
	t.Parallel()
	if err := (asset.Capabilities{Runtime: []asset.Capability{asset.CapabilityWorldParticlesV1}}).Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := cartridge.Manifest{
		ProjectName: "particles",
		Compiler:    "go",
		Width:       640,
		Height:      360,
		Features:    []string{cartridge.FeatureWorldParticlesV1, cartridge.FeatureWorldSectorsV1},
	}
	data, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(data); err != nil {
		t.Fatal(err)
	}
	for _, features := range [][]string{{cartridge.FeatureWorldParticlesV1}, {cartridge.FeatureWorldSectorsV1, cartridge.FeatureWorldParticlesV1}, {"world/particles@3", cartridge.FeatureWorldSectorsV1}} {
		manifest.Features = features
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatalf("invalid feature set accepted: %v", features)
		}
	}
}

func TestAlphaParticleCapabilityAndFeatureDependencies(t *testing.T) {
	t.Parallel()
	for _, runtime := range [][]asset.Capability{
		{asset.CapabilityWorldParticlesV2},
		{asset.CapabilityWorldParticlesV1, asset.CapabilityWorldParticlesV2},
	} {
		if err := (asset.Capabilities{Runtime: runtime}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	manifest := cartridge.Manifest{ProjectName: "alpha-particles", Compiler: "go", Width: 640, Height: 360,
		Features: []string{cartridge.FeatureWorldParticlesV2, cartridge.FeatureWorldSectorsV1}}
	encoded, err := cartridge.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cartridge.DecodeManifest(encoded); err != nil {
		t.Fatal(err)
	}
	for _, features := range [][]string{
		{cartridge.FeatureWorldParticlesV2},
		{cartridge.FeatureWorldParticlesV2, cartridge.FeatureWorldParticlesV2, cartridge.FeatureWorldSectorsV1},
		{"world/particles@3", cartridge.FeatureWorldSectorsV1},
	} {
		manifest.Features = features
		if _, err := cartridge.EncodeManifest(manifest); err == nil {
			t.Fatalf("invalid alpha capability set accepted: %v", features)
		}
	}
}
