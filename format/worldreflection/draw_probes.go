package worldreflection

import (
	"encoding/binary"

	"github.com/karty-game/karty-sdk/format/worldgpuchunk"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

func validDrawProbes(cells [][]byte, probes []Probe) bool {
	if len(cells) > worldgpuchunk.MaxCells {
		return false
	}

	for _, slots := range cells {
		if len(slots) == 0 || len(slots)%4 != 0 || len(slots)/4 > worldgpuchunk.MaxDraws {
			return false
		}

		for offset := 0; offset < len(slots); offset += 4 {
			if uint64(binary.LittleEndian.Uint32(slots[offset:])) > uint64(len(probes)) {
				return false
			}
		}
	}

	return true
}

func readDrawProbes(reader *binwire.Reader, allocate bool) [][]byte {
	count, _ := reader.Count(worldgpuchunk.MaxCells, 4, 24)

	var cells [][]byte
	if allocate {
		cells = make([][]byte, count)
	}

	for index := range count {
		draws, _ := reader.Count(worldgpuchunk.MaxDraws, 4, 0)

		slots := reader.Raw(draws * 4)
		if allocate {
			cells[index] = slots
		}
	}

	return cells
}
