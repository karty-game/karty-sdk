// Package level defines the bounded, versioned data envelope stored in a Karty
// level cartridge. It is shared contract code, not generated project output.
package level

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	EnvelopeVersion  = uint16(1)
	HeaderSize       = 32
	EntryHeaderSize  = 12
	MaxEnvelopeSize  = 16 * 1024 * 1024
	MaxMetadataSize  = 16 * 1024
	MaxEntryCount    = 4096
	MaxEntryNameSize = 1024
	MaxEntrySize     = 8 * 1024 * 1024
	envelopeMagic    = "KTYL"
)

var (
	ErrEnvelopeSize = errors.New("level envelope size is invalid")
	ErrHeader       = errors.New("level envelope header is invalid")
	ErrVersion      = errors.New("level envelope version is unsupported")
	ErrMetadata     = errors.New("level metadata is invalid")
	ErrEntry        = errors.New("level data entry is invalid")
	ErrEntryOrder   = errors.New("level data entries are not in canonical order")
	ErrEntryRange   = errors.New("level data entry range is invalid")
)

// EntryKind identifies a named payload's schema family.
type EntryKind uint8

const (
	EntryData    EntryKind = 1
	EntryTexture EntryKind = 2
)

const textureEntryPrefix = "@texture/"

// TextureEntryName returns the reserved envelope name for a nonzero,
// level-local texture ID.
func TextureEntryName(assetID uint32) string {
	return textureEntryPrefix + fmt.Sprintf("%08x", assetID)
}

// TextureAssetID recognizes a reserved texture payload name.
func TextureAssetID(name string) (uint32, bool) {
	if len(name) != len(textureEntryPrefix)+8 || !strings.HasPrefix(name, textureEntryPrefix) {
		return 0, false
	}

	value, err := strconv.ParseUint(name[len(textureEntryPrefix):], 16, 32)

	return uint32(value), err == nil && value != 0
}

// SourceEntry is an authoring-time named payload passed to Encode.
// Texture entries must have distinct nonzero numeric IDs, even when logical
// names use different hexadecimal case.
type SourceEntry struct {
	Name string
	Kind EntryKind
	Data []byte
}

// Entry describes a validated named payload in an Envelope.
type Entry struct {
	Name   string
	Kind   EntryKind
	Offset uint32
	Length uint32
}

// Envelope is a validated view over immutable envelope bytes. Metadata and
// payload slices alias Raw and remain valid only while Raw is retained.
type Envelope struct {
	Raw      []byte
	Metadata []byte
	Entries  []Entry
	payload  []byte
}

// Encode creates a deterministic envelope. Entries are ordered by logical
// name; callers do not control table or payload order.
func Encode(metadata []byte, source []SourceEntry) ([]byte, error) {
	if err := validateMetadata(metadata); err != nil {
		return nil, err
	}

	if len(source) > MaxEntryCount {
		return nil, fmt.Errorf("%d entries: %w", len(source), ErrEntry)
	}

	entries := slices.Clone(source)
	slices.SortFunc(entries, func(left, right SourceEntry) int {
		return strings.Compare(left.Name, right.Name)
	})

	tableLength, payloadLength, err := encodedLengths(entries)
	if err != nil {
		return nil, err
	}

	total := HeaderSize + len(metadata) + tableLength + payloadLength
	if total > MaxEnvelopeSize {
		return nil, fmt.Errorf("%d bytes: %w", total, ErrEnvelopeSize)
	}

	encoded := make([]byte, total)
	copy(encoded[:4], envelopeMagic)
	binary.LittleEndian.PutUint16(encoded[4:6], EnvelopeVersion)
	binary.LittleEndian.PutUint32(encoded[8:12], uint32(len(metadata)))
	binary.LittleEndian.PutUint32(encoded[12:16], uint32(len(entries)))
	binary.LittleEndian.PutUint32(encoded[16:20], uint32(tableLength))
	binary.LittleEndian.PutUint32(encoded[20:24], uint32(payloadLength))
	binary.LittleEndian.PutUint32(encoded[24:28], uint32(total))
	copy(encoded[HeaderSize:], metadata)

	tableOffset := HeaderSize + len(metadata)
	payloadOffset := tableOffset + tableLength
	payloadCursor := 0

	for _, entry := range entries {
		encoded[tableOffset] = byte(entry.Kind)
		binary.LittleEndian.PutUint16(encoded[tableOffset+2:tableOffset+4], uint16(len(entry.Name)))
		binary.LittleEndian.PutUint32(encoded[tableOffset+4:tableOffset+8], uint32(payloadCursor))
		binary.LittleEndian.PutUint32(encoded[tableOffset+8:tableOffset+12], uint32(len(entry.Data)))
		copy(encoded[tableOffset+EntryHeaderSize:], entry.Name)
		copy(encoded[payloadOffset+payloadCursor:], entry.Data)
		tableOffset += EntryHeaderSize + len(entry.Name)
		payloadCursor += len(entry.Data)
	}

	return encoded, nil
}

func encodedLengths(entries []SourceEntry) (int, int, error) {
	tableLength := 0
	payloadLength := 0
	previous := ""
	textureIDs := make(map[uint32]bool)

	for index, entry := range entries {
		if !validEntryKind(entry.Kind) || len(entry.Name) == 0 || len(entry.Name) > MaxEntryNameSize ||
			!utf8.ValidString(
				entry.Name,
			) || !validEntryName(entry.Name, entry.Kind) || len(entry.Data) == 0 || len(entry.Data) > MaxEntrySize {
			return 0, 0, fmt.Errorf("entry %q: %w", entry.Name, ErrEntry)
		}

		if index > 0 && entry.Name == previous {
			return 0, 0, fmt.Errorf("duplicate entry %q: %w", entry.Name, ErrEntry)
		}

		if entry.Kind == EntryTexture {
			id, _ := TextureAssetID(entry.Name) // Already checked by validEntryName.
			if textureIDs[id] {
				return 0, 0, fmt.Errorf("duplicate texture ID %d: %w", id, ErrEntry)
			}
			textureIDs[id] = true
		}

		previous = entry.Name
		if tableLength > MaxEnvelopeSize-EntryHeaderSize-len(entry.Name) ||
			payloadLength > MaxEnvelopeSize-len(entry.Data) {
			return 0, 0, ErrEnvelopeSize
		}

		tableLength += EntryHeaderSize + len(entry.Name)
		payloadLength += len(entry.Data)
	}

	return tableLength, payloadLength, nil
}

// Decode validates a complete canonical envelope before exposing any entry.
func Decode(encoded []byte) (Envelope, error) {
	if len(encoded) < HeaderSize || len(encoded) > MaxEnvelopeSize {
		return Envelope{}, ErrEnvelopeSize
	}

	if string(encoded[:4]) != envelopeMagic || binary.LittleEndian.Uint16(encoded[4:6]) != EnvelopeVersion {
		if string(encoded[:4]) == envelopeMagic {
			return Envelope{}, ErrVersion
		}

		return Envelope{}, ErrHeader
	}

	if binary.LittleEndian.Uint16(encoded[6:8]) != 0 || binary.LittleEndian.Uint32(encoded[28:32]) != 0 {
		return Envelope{}, ErrHeader
	}

	// Validate wire lengths before narrowing to int: hosts may be 32-bit.
	metadataSize := uint64(binary.LittleEndian.Uint32(encoded[8:12]))
	entryCountValue := uint64(binary.LittleEndian.Uint32(encoded[12:16]))
	tableSize := uint64(binary.LittleEndian.Uint32(encoded[16:20]))
	payloadSize := uint64(binary.LittleEndian.Uint32(encoded[20:24]))
	totalSize := uint64(binary.LittleEndian.Uint32(encoded[24:28]))
	available := uint64(len(encoded) - HeaderSize)
	if metadataSize > MaxMetadataSize || entryCountValue > MaxEntryCount || totalSize != uint64(len(encoded)) ||
		metadataSize > available || tableSize > available-metadataSize ||
		payloadSize != available-metadataSize-tableSize {
		return Envelope{}, ErrEnvelopeSize
	}
	metadataLength, entryCount := int(metadataSize), int(entryCountValue)
	tableLength, payloadLength := int(tableSize), int(payloadSize)

	metadata := encoded[HeaderSize : HeaderSize+metadataLength]
	if err := validateMetadata(metadata); err != nil {
		return Envelope{}, err
	}

	table := encoded[HeaderSize+metadataLength : HeaderSize+metadataLength+tableLength]
	payload := encoded[len(encoded)-payloadLength:]

	entries, err := decodeEntries(table, payloadLength, entryCount)
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{Raw: encoded, Metadata: metadata, Entries: entries, payload: payload}, nil
}

func decodeEntries(table []byte, payloadLength, count int) ([]Entry, error) {
	entries := make([]Entry, 0, count)
	offset := 0
	expectedPayloadOffset := 0
	previous := ""
	textureIDs := make(map[uint32]bool)

	for index := range count {
		if len(table)-offset < EntryHeaderSize {
			return nil, ErrEntry
		}

		kind := EntryKind(table[offset])
		reserved := table[offset+1]
		nameLength := int(binary.LittleEndian.Uint16(table[offset+2 : offset+4]))
		dataOffset := binary.LittleEndian.Uint32(table[offset+4 : offset+8])
		dataLength := binary.LittleEndian.Uint32(table[offset+8 : offset+12])

		offset += EntryHeaderSize
		if !validEntryKind(kind) || reserved != 0 || nameLength == 0 || nameLength > MaxEntryNameSize ||
			nameLength > len(table)-offset || dataLength == 0 || dataLength > MaxEntrySize {
			return nil, ErrEntry
		}

		nameBytes := table[offset : offset+nameLength]
		if !utf8.Valid(nameBytes) {
			return nil, ErrEntry
		}

		name := string(nameBytes)
		if !validEntryName(name, kind) {
			return nil, ErrEntry
		}

		if index > 0 && strings.Compare(previous, name) >= 0 {
			return nil, ErrEntryOrder
		}

		if kind == EntryTexture {
			id, _ := TextureAssetID(name) // Already checked by validEntryName.
			if textureIDs[id] {
				return nil, ErrEntry
			}
			textureIDs[id] = true
		}

		if dataOffset != uint32(expectedPayloadOffset) || dataLength > uint32(payloadLength)-dataOffset {
			return nil, ErrEntryRange
		}

		entries = append(entries, Entry{Name: name, Kind: kind, Offset: dataOffset, Length: dataLength})
		previous = name
		expectedPayloadOffset += int(dataLength)
		offset += nameLength
	}

	if offset != len(table) || expectedPayloadOffset != payloadLength {
		return nil, ErrEntryRange
	}

	return entries, nil
}

func validEntryKind(kind EntryKind) bool {
	return kind == EntryData || kind == EntryTexture
}

// validEntryName keeps the reserved texture namespace identical on both paths.
func validEntryName(name string, kind EntryKind) bool {
	if kind == EntryTexture {
		_, valid := TextureAssetID(name)

		return valid
	}

	return !strings.HasPrefix(name, textureEntryPrefix)
}

func validateMetadata(metadata []byte) error {
	if len(metadata) == 0 || len(metadata) > MaxMetadataSize || !utf8.Valid(metadata) || !json.Valid(metadata) {
		return ErrMetadata
	}

	trimmed := bytes.TrimSpace(metadata)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return ErrMetadata
	}

	return nil
}

// Read returns a bounded view of a named payload. The returned bytes alias Raw.
func (envelope Envelope) Read(name string, offset, maximum uint32) ([]byte, uint32, bool) {
	index, found := slices.BinarySearchFunc(envelope.Entries, name, func(entry Entry, target string) int {
		return strings.Compare(entry.Name, target)
	})
	if !found {
		return nil, 0, false
	}

	entry := envelope.Entries[index]
	if offset > entry.Length {
		return nil, entry.Length, false
	}

	length := min(maximum, entry.Length-offset)
	start := entry.Offset + offset

	return envelope.payload[start : start+length], entry.Length, true
}
