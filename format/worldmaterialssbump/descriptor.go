// Package worldmaterialssbump defines optional linear self-shadowed RNM weights.
// Existing worldmaterial data and reflectance channels retain their meanings.
package worldmaterialssbump

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/karty-game/karty-sdk/format/worldmaterial"
)

const (
	Feature            = "world/material-ssbump@1"
	MetadataKey        = "kartyWorldMaterialSSBump"
	DescriptorEntry    = "@world/material-ssbump-layout"
	AtlasEntry         = "@world/material-ssbump"
	MipTailEntry       = "@world/material-ssbump-mips"
	BinaryMagic        = "KSB1"
	MaxDescriptorBytes = 1024
	descriptorHeader   = 38 // magic, SHA-256, big-endian uint16 count
)

var ErrCompanion = errors.New("world material SSBump companion is invalid")

// Descriptor binds a canonical layout, including strengths, coverage and mips.
// IDs are a nonempty unique subset in layout order; slices remain caller-owned.
type Descriptor struct {
	LayoutSHA256 [32]byte
	MaterialIDs  []uint32
}

func NewDescriptor(layout worldmaterial.Layout, ids []uint32) (Descriptor, error) {
	encoded, err := worldmaterial.EncodeLayout(layout)
	if err != nil {
		return Descriptor{}, ErrCompanion
	}
	d := Descriptor{LayoutSHA256: sha256.Sum256(encoded), MaterialIDs: append([]uint32(nil), ids...)}
	if d.Validate(layout) != nil {
		return Descriptor{}, ErrCompanion
	}
	return d, nil
}

func (d Descriptor) Validate(layout worldmaterial.Layout) error {
	if !validIDs(d.MaterialIDs) {
		return ErrCompanion
	}
	encoded, err := worldmaterial.EncodeLayout(layout)
	if err != nil || sha256.Sum256(encoded) != d.LayoutSHA256 {
		return ErrCompanion
	}
	next := 0
	for _, r := range layout.Materials {
		if next < len(d.MaterialIDs) && r.MaterialID == d.MaterialIDs[next] {
			next++
		}
	}
	if next != len(d.MaterialIDs) {
		return ErrCompanion
	}
	return nil
}

func validIDs(ids []uint32) bool {
	if len(ids) == 0 || len(ids) > worldmaterial.MaxMaterials {
		return false
	}
	for i, id := range ids {
		for _, previous := range ids[:i] {
			if previous == id {
				return false
			}
		}
	}
	return true
}

// EncodeDescriptor emits KSB1, a 32-byte layout digest, uint16 count and uint32
// IDs. All integers are big-endian; no optional fields or trailing bytes exist.
// Validate against the owning layout before publication or consumption.
func EncodeDescriptor(d Descriptor) ([]byte, error) {
	if !validIDs(d.MaterialIDs) {
		return nil, ErrCompanion
	}
	encoded := make([]byte, descriptorHeader+4*len(d.MaterialIDs))
	copy(encoded, BinaryMagic)
	copy(encoded[4:36], d.LayoutSHA256[:])
	binary.BigEndian.PutUint16(encoded[36:38], uint16(len(d.MaterialIDs)))
	for i, id := range d.MaterialIDs {
		binary.BigEndian.PutUint32(encoded[descriptorHeader+4*i:], id)
	}
	return encoded, nil
}

// DecodeDescriptor bounds the complete payload before allocating its ID slice.
func DecodeDescriptor(encoded []byte) (Descriptor, error) {
	if len(encoded) < descriptorHeader || len(encoded) > MaxDescriptorBytes || string(encoded[:4]) != BinaryMagic {
		return Descriptor{}, ErrCompanion
	}
	count := int(binary.BigEndian.Uint16(encoded[36:38]))
	if count < 1 || count > worldmaterial.MaxMaterials || len(encoded) != descriptorHeader+4*count {
		return Descriptor{}, ErrCompanion
	}
	d := Descriptor{MaterialIDs: make([]uint32, count)}
	copy(d.LayoutSHA256[:], encoded[4:36])
	for i := range d.MaterialIDs {
		d.MaterialIDs[i] = binary.BigEndian.Uint32(encoded[descriptorHeader+4*i:])
	}
	if !validIDs(d.MaterialIDs) {
		return Descriptor{}, ErrCompanion
	}
	return d, nil
}
