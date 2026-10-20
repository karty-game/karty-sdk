package textureatlas_test

import (
	"math"
	"testing"

	atlasfilter "github.com/karty-game/karty-sdk/bake/textureatlas"
)

func TestMipPreservesCutoutColorAndRenormalizesNormals(t *testing.T) {
	t.Parallel()
	pixels := make([]byte, 32*32*4)
	for i := 0; i < len(pixels); i += 4 {
		copy(pixels[i:i+4], []byte{255, 0, 255, 0})
		if i/4%32 < 8+i/128%8 {
			copy(pixels[i:i+4], []byte{32, 160, 24, 255})
		}
	}
	target := atlasfilter.Coverage(pixels, 1)
	for width := 32; width > 8; width /= 2 {
		pixels = atlasfilter.Mip(pixels, width, width, false)
		atlasfilter.PreserveCoverage(pixels, target)
		if math.Abs(atlasfilter.Coverage(pixels, 1)-target) > .05 {
			t.Fatal("cutout coverage drifted")
		}
		for i := 0; i < len(pixels); i += 4 {
			if pixels[i+3] > 0 && (pixels[i] > 33 || pixels[i+1] < 159 || pixels[i+2] > 25) {
				t.Fatal("transparent RGB leaked into foliage")
			}
		}
	}
	normals := make([]byte, 16*16*4)
	for i := 0; i < len(normals); i += 4 {
		copy(normals[i:i+4], []byte{218, 128, 218, 255})
		if i/4%2 == 0 {
			copy(normals[i:i+4], []byte{128, 218, 218, 0})
		}
	}
	mip := atlasfilter.Mip(normals, 16, 16, true)
	for i := 0; i < len(mip); i += 4 {
		length := 0.0
		for c := range 3 {
			value := float64(mip[i+c])/255*2 - 1
			length += value * value
		}
		if math.Abs(math.Sqrt(length)-1) > .008 || mip[i+3] != 255 {
			t.Fatal("normal length/opacity changed")
		}
	}
	if atlasfilter.Mip(nil, 8192, 8192, false) != nil {
		t.Fatal("invalid source accepted")
	}
}
