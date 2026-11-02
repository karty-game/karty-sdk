package worldmaterialgpu

import (
	"github.com/karty-game/karty-sdk/codec/ktx2"
	"github.com/karty-game/karty-sdk/format/asset"
	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

// Pair contains one mandatory trio, with caller-owned immutable byte slices.
// Each KTX2 contains final 256x256 material layers and all nine mip levels.
// Albedo is sRGB RGB and coverage alpha. SSBump is linear coefficients RGB and
// emission-mask alpha. Detail is linear normal XY, metallic B, smoothness A.
type Pair struct {
	Descriptor             Descriptor
	Albedo, SSBump, Detail []byte
}

// Inspect preflights container structure and aggregate allocation bounds before
// inflation. Full Zstd decompression and UASTC validation/transcoding are still
// required before consumer publication; no graphics decoder lives in this format.
func (p Pair) Inspect(layout worldmaterial.Layout) ([3]ktx2.Info, error) {
	var infos [3]ktx2.Info
	if p.Descriptor.Validate(layout) != nil {
		return infos, ErrMaterial
	}
	var inflated, rgba, encoded uint64
	for i, data := range [3][]byte{p.Albedo, p.SSBump, p.Detail} {
		info, err := ktx2.Inspect(data)
		if err != nil || info.Width != worldmaterial.TileSize || info.Height != worldmaterial.TileSize ||
			int(info.Layers) != len(p.Descriptor.MaterialIDs) || info.SRGB != (i == 0) ||
			info.InflatedBytes > asset.MaxDecodedTextures-inflated || info.RGBABytes > asset.MaxDecodedTextures-rgba ||
			uint64(len(data)) > asset.MaxSourceAssetBytes-encoded {
			return [3]ktx2.Info{}, ErrMaterial
		}
		inflated += info.InflatedBytes
		rgba += info.RGBABytes
		encoded += uint64(len(data))
		infos[i] = info
	}
	return infos, nil
}

// Validate checks the prepared-material declaration and preflight structure.
// Consumers must additionally validate the compressed streams while decoding.
func (p Pair) Validate(layout worldmaterial.Layout) error {
	_, err := p.Inspect(layout)
	return err
}
