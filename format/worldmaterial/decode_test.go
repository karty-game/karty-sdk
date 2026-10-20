package worldmaterial_test

import (
	"bytes"
	"errors"
	"image"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

func TestPairDecodeRetainsValidatedOwnedPixels(t *testing.T) {
	t.Parallel()
	for _, pair := range []worldmaterial.Pair{testPair(t), testMipPair(t), reflectancePair(t, true)} {
		decoded, err := pair.Decode()
		if err != nil {
			t.Fatal(err)
		}
		images := []*image.NRGBA{decoded.Albedo(), decoded.Data(), decoded.MipTail(), decoded.Reflectance(), decoded.ReflectanceMipTail()}
		for index, encoded := range [][]byte{pair.Albedo, pair.Data, pair.MipTail, pair.Reflectance, pair.ReflectanceMipTail} {
			if len(encoded) == 0 {
				if images[index] != nil {
					t.Fatal("missing optional image allocated pixels")
				}
				continue
			}
			_, want, err := qoi.Decode(encoded)
			if err != nil || images[index].Rect != want.Rect || !bytes.Equal(images[index].Pix, want.Pix) {
				t.Fatalf("image %d changed straight pixel bytes: %v", index, err)
			}
		}
		again, err := pair.Decode()
		if err != nil {
			t.Fatal(err)
		}
		decoded.Data().Pix[0] ^= 255
		if decoded.Data().Pix[0] == again.Data().Pix[0] {
			t.Fatal("independent decode calls share mutable pixels")
		}
	}
}

func TestPairDecodeRejectsMalformedLaterImagesWithoutPartialResult(t *testing.T) {
	t.Parallel()
	for index := range 5 {
		pair := reflectancePair(t, true)
		images := []*[]byte{&pair.Albedo, &pair.Data, &pair.MipTail, &pair.Reflectance, &pair.ReflectanceMipTail}
		encoded := images[index]
		*encoded = bytes.Clone(*encoded)
		(*encoded)[len(*encoded)-1] = 0
		decoded, err := pair.Decode()
		if !errors.Is(err, worldmaterial.ErrAtlas) || pair.Validate() == nil {
			t.Fatalf("malformed image %d accepted: %v", index, err)
		}
		if decoded.Albedo() != nil || decoded.Data() != nil || decoded.MipTail() != nil || decoded.Reflectance() != nil ||
			decoded.ReflectanceMipTail() != nil {
			t.Fatal("failed decode exposed a partial result")
		}
	}
}
