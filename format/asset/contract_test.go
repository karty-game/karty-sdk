package asset_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/asset"
)

func TestCapabilitiesAreCanonical(t *testing.T) {
	t.Parallel()

	valid := asset.Capabilities{
		Processors: []asset.Processor{asset.ProcessorCopyPNGv1, asset.ProcessorQOAv1, asset.ProcessorQOIv1},
		Runtime:    []asset.Capability{asset.CapabilitySoundQOAv1, asset.CapabilityTextureQOIv1},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}

	for name, capabilities := range map[string]asset.Capabilities{
		"unknown processor": {Processors: []asset.Processor{"future@1"}},
		"duplicate":         {Runtime: []asset.Capability{asset.CapabilitySoundQOAv1, asset.CapabilitySoundQOAv1}},
		"unsorted":          {Processors: []asset.Processor{asset.ProcessorQOIv1, asset.ProcessorQOAv1}},
	} {
		if err := capabilities.Validate(); err == nil {
			t.Errorf("Validate() accepted %s", name)
		}
	}
}

func TestRecipesValidateFrozenOptions(t *testing.T) {
	t.Parallel()

	if err := (asset.ImageRecipe{MaxWidth: 2048, Filter: asset.ImageFilterSmooth, BitDepth: 8}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (asset.ImageRecipe{Filter: "bilinear", BitDepth: 8}).Validate(); err == nil {
		t.Fatal("ImageRecipe.Validate() accepted an unversioned filter")
	}
	if err := (asset.AudioRecipe{SampleRate: 48_000, ChannelMode: asset.ChannelPreserve}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (asset.AudioRecipe{SampleRate: 96_000, ChannelMode: asset.ChannelPreserve}).Validate(); err == nil {
		t.Fatal("AudioRecipe.Validate() accepted an unsupported sample rate")
	}
}
