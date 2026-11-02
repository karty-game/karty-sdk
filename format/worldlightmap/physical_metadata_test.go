package worldlightmap

import (
	"math"
	"testing"
)

func TestPhysicalLightingMetadataRetainsLayoutAndScalarAdmission(t *testing.T) {
	_, layout, lighting, _ := fixedFormatFixture(t)
	if err := ValidateFixedLightingMetadata(lighting, layout); err != nil {
		t.Fatal(err)
	}
	changed := lighting
	changed.RGBMRange = math.NaN()
	if ValidateFixedLightingMetadata(changed, layout) == nil {
		t.Fatal("invalid scalar admitted without source image")
	}
	layout.Pages[0].Width++
	if ValidateFixedLightingMetadata(lighting, layout) == nil {
		t.Fatal("foreign layout admitted without source image")
	}
}
