// Package worldmaterialgpu defines prepared GPU material texture arrays.
package worldmaterialgpu

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

const (
	Feature            = "world/material-gpu@1"
	MetadataKey        = "kartyWorldMaterialGPU"
	DescriptorEntry    = "@world/material-gpu-layout"
	AlbedoEntry        = "@world/material-albedo.ktx2"
	SSBumpEntry        = "@world/material-ssbump.ktx2"
	DetailEntry        = "@world/material-detail.ktx2"
	BinaryMagic        = "KMG1"
	MaxDescriptorBytes = 1024
	descriptorHeader   = 38
)

var ErrMaterial = errors.New("prepared GPU world material is invalid")

// Descriptor binds the full material order to the canonical source layout,
// including coverage and strength controls. Every material has all three arrays.
type Descriptor struct {
	LayoutSHA256 [32]byte
	MaterialIDs  []uint32
}

func NewDescriptor(layout worldmaterial.Layout) (Descriptor, error) {
	encoded, err := worldmaterial.EncodeLayout(layout)
	if err != nil {
		return Descriptor{}, ErrMaterial
	}
	d := Descriptor{LayoutSHA256: sha256.Sum256(encoded), MaterialIDs: make([]uint32, len(layout.Materials))}
	for i, r := range layout.Materials {
		d.MaterialIDs[i] = r.MaterialID
	}
	if d.Validate(layout) != nil {
		return Descriptor{}, ErrMaterial
	}
	return d, nil
}

func (d Descriptor) Validate(layout worldmaterial.Layout) error {
	if !validIDs(d.MaterialIDs) || len(d.MaterialIDs) != len(layout.Materials) {
		return ErrMaterial
	}
	encoded, err := worldmaterial.EncodeLayout(layout)
	if err != nil || sha256.Sum256(encoded) != d.LayoutSHA256 {
		return ErrMaterial
	}
	for i, r := range layout.Materials {
		if r.MaterialID != d.MaterialIDs[i] {
			return ErrMaterial
		}
	}
	return nil
}

func validIDs(ids []uint32) bool {
	if len(ids) == 0 || len(ids) > worldmaterial.MaxMaterials {
		return false
	}
	for i, id := range ids {
		for _, previous := range ids[:i] {
			if id == previous {
				return false
			}
		}
	}
	return true
}

// EncodeDescriptor emits KMG1, canonical-layout SHA-256, uint16 count and full
// uint32 material IDs. All integers are big-endian; no trailing fields exist.
func EncodeDescriptor(d Descriptor) ([]byte, error) {
	if !validIDs(d.MaterialIDs) {
		return nil, ErrMaterial
	}
	encoded := make([]byte, descriptorHeader+4*len(d.MaterialIDs))
	copy(encoded, BinaryMagic)
	copy(encoded[4:36], d.LayoutSHA256[:])
	binary.BigEndian.PutUint16(encoded[36:38], uint16(len(d.MaterialIDs)))
	for i, id := range d.MaterialIDs {
		binary.BigEndian.PutUint32(encoded[descriptorHeader+i*4:], id)
	}
	return encoded, nil
}

func DecodeDescriptor(encoded []byte) (Descriptor, error) {
	if len(encoded) < descriptorHeader || len(encoded) > MaxDescriptorBytes || string(encoded[:4]) != BinaryMagic {
		return Descriptor{}, ErrMaterial
	}
	count := int(binary.BigEndian.Uint16(encoded[36:38]))
	if count < 1 || count > worldmaterial.MaxMaterials || len(encoded) != descriptorHeader+4*count {
		return Descriptor{}, ErrMaterial
	}
	d := Descriptor{MaterialIDs: make([]uint32, count)}
	copy(d.LayoutSHA256[:], encoded[4:36])
	for i := range d.MaterialIDs {
		d.MaterialIDs[i] = binary.BigEndian.Uint32(encoded[descriptorHeader+i*4:])
	}
	if !validIDs(d.MaterialIDs) {
		return Descriptor{}, ErrMaterial
	}
	return d, nil
}
