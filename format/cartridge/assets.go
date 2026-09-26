// Package cartridge defines bounded data embedded in a Karty game cartridge.
package cartridge

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	AssetSectionName = "karty.assets.v1"
	assetMagic       = "KTYA"
	assetVersion     = uint16(1)
	assetHeaderSize  = 16
	assetEntryHeader = 8
	MaxAssetBundle   = 16 * 1024 * 1024
	MaxAssetCount    = 4096
	MaxAssetName     = 1024
	MaxAssetSize     = 8 * 1024 * 1024
)

var (
	ErrAssets         = errors.New("game asset bundle is invalid")
	ErrSection        = errors.New("game cartridge custom section is invalid")
	ErrSectionMissing = errors.New("game cartridge custom section is missing")
)

type Asset struct {
	Name  string
	Bytes []byte
}

func EncodeAssets(source []Asset) ([]byte, error) {
	assets := slices.Clone(source)
	slices.SortFunc(assets, func(left, right Asset) int { return strings.Compare(left.Name, right.Name) })

	if len(assets) > MaxAssetCount {
		return nil, ErrAssets
	}

	total := assetHeaderSize
	previous := ""

	for _, asset := range assets {
		if asset.Name == "" || len(asset.Name) > MaxAssetName || !utf8.ValidString(asset.Name) ||
			len(asset.Bytes) == 0 || len(asset.Bytes) > MaxAssetSize || asset.Name == previous {
			return nil, fmt.Errorf("asset %q: %w", asset.Name, ErrAssets)
		}

		previous = asset.Name
		total += assetEntryHeader + len(asset.Name) + len(asset.Bytes)

		if total > MaxAssetBundle {
			return nil, ErrAssets
		}
	}

	result := make([]byte, total)
	copy(result, assetMagic)
	binary.LittleEndian.PutUint16(result[4:6], assetVersion)
	binary.LittleEndian.PutUint32(result[8:12], uint32(len(assets)))
	binary.LittleEndian.PutUint32(result[12:16], uint32(total))

	offset := assetHeaderSize
	for _, asset := range assets {
		binary.LittleEndian.PutUint16(result[offset:offset+2], uint16(len(asset.Name)))
		binary.LittleEndian.PutUint32(result[offset+4:offset+8], uint32(len(asset.Bytes)))
		offset += assetEntryHeader
		copy(result[offset:], asset.Name)
		offset += len(asset.Name)
		copy(result[offset:], asset.Bytes)
		offset += len(asset.Bytes)
	}

	return result, nil
}

func DecodeAssets(encoded []byte) ([]Asset, error) {
	if len(encoded) < assetHeaderSize || len(encoded) > MaxAssetBundle || string(encoded[:4]) != assetMagic ||
		binary.LittleEndian.Uint16(encoded[4:6]) != assetVersion || binary.LittleEndian.Uint16(encoded[6:8]) != 0 ||
		int(binary.LittleEndian.Uint32(encoded[12:16])) != len(encoded) {
		return nil, ErrAssets
	}

	count := int(binary.LittleEndian.Uint32(encoded[8:12]))
	if count > MaxAssetCount {
		return nil, ErrAssets
	}

	result := make([]Asset, 0, count)
	offset := assetHeaderSize
	previous := ""

	for range count {
		if len(encoded)-offset < assetEntryHeader {
			return nil, ErrAssets
		}

		nameLength := int(binary.LittleEndian.Uint16(encoded[offset : offset+2]))
		reserved := binary.LittleEndian.Uint16(encoded[offset+2 : offset+4])
		dataLength := int(binary.LittleEndian.Uint32(encoded[offset+4 : offset+8]))
		offset += assetEntryHeader

		if reserved != 0 || nameLength < 1 || nameLength > MaxAssetName || dataLength < 1 || dataLength > MaxAssetSize ||
			nameLength > len(encoded)-offset || dataLength > len(encoded)-offset-nameLength {
			return nil, ErrAssets
		}

		name := string(encoded[offset : offset+nameLength])
		offset += nameLength

		if !utf8.ValidString(name) || name <= previous {
			return nil, ErrAssets
		}

		result = append(result, Asset{Name: name, Bytes: encoded[offset : offset+dataLength]})
		offset += dataLength
		previous = name
	}

	if offset != len(encoded) {
		return nil, ErrAssets
	}

	return result, nil
}

func EmbedAssets(wasm, bundle []byte) ([]byte, error) {
	if _, err := DecodeAssets(bundle); err != nil {
		return nil, err
	}

	embedded, err := EmbedSection(wasm, AssetSectionName, bundle)
	if err != nil {
		return nil, ErrAssets
	}

	return embedded, nil
}

func ExtractAssets(wasm []byte) ([]Asset, error) {
	bundle, err := ExtractSection(wasm, AssetSectionName)
	if err != nil {
		return nil, ErrAssets
	}

	return DecodeAssets(bundle)
}

func EmbedSection(wasm []byte, name string, contents []byte) ([]byte, error) {
	if len(wasm) < 8 || !bytes.Equal(wasm[:8], []byte{0, 'a', 's', 'm', 1, 0, 0, 0}) {
		return nil, ErrSection
	}

	if name == "" || len(contents) == 0 {
		return nil, ErrSection
	}

	if _, err := ExtractSection(wasm, name); err == nil {
		return nil, ErrSection
	} else if !errors.Is(err, ErrSectionMissing) {
		return nil, err
	}

	payload := appendULEB(nil, len(name))
	payload = append(payload, name...)
	payload = append(payload, contents...)
	result := append([]byte(nil), wasm...)
	result = append(result, 0)
	result = appendULEB(result, len(payload))

	return append(result, payload...), nil
}

func ExtractSection(wasm []byte, wanted string) ([]byte, error) {
	if len(wasm) < 8 || !bytes.Equal(wasm[:8], []byte{0, 'a', 's', 'm', 1, 0, 0, 0}) {
		return nil, ErrSection
	}

	var bundle []byte

	for offset := 8; offset < len(wasm); {
		identifier := wasm[offset]
		offset++

		length, valid := readULEB(wasm, &offset)
		if !valid || length > len(wasm)-offset {
			return nil, ErrSection
		}

		section := wasm[offset : offset+length]
		offset += length

		if identifier != 0 {
			continue
		}

		nameOffset := 0

		nameLength, valid := readULEB(section, &nameOffset)
		if !valid || nameLength > len(section)-nameOffset {
			return nil, ErrSection
		}

		if string(section[nameOffset:nameOffset+nameLength]) == wanted {
			if bundle != nil {
				return nil, ErrSection
			}

			bundle = section[nameOffset+nameLength:]
		}
	}

	if bundle == nil {
		return nil, ErrSectionMissing
	}

	return bundle, nil
}

func appendULEB(destination []byte, value int) []byte {
	for value >= 0x80 {
		destination = append(destination, byte(value)|0x80)
		value >>= 7
	}

	return append(destination, byte(value))
}

func readULEB(source []byte, offset *int) (int, bool) {
	value := 0

	for shift := 0; shift < 35 && *offset < len(source); shift += 7 {
		current := source[*offset]
		*offset++
		value |= int(current&0x7f) << shift

		if current&0x80 == 0 {
			return value, true
		}
	}

	return 0, false
}
