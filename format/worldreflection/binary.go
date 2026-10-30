package worldreflection

import (
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

func Encode(bundle Bundle) ([]byte, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	writer := binwire.NewWriter("KRC1", MaxEncodedSize)
	writer.String(bundle.InputSHA256, 64)
	writer.String(bundle.Producer, 64)
	for _, bindings := range [][]int{bundle.Sectors, bundle.Solids} {
		writer.Count(len(bindings), false, world.MaxStaticSolids)
		for _, id := range bindings {
			writer.I32(id)
		}
	}
	writer.Count(len(bundle.Probes), false, MaxProbes)
	for _, probe := range bundle.Probes {
		writer.I32(probe.ID)
		for _, point := range [][3]float32{probe.Origin, probe.Low, probe.High} {
			for _, value := range point {
				writer.F32(value)
			}
		}
		writer.Count(len(probe.Image), false, MaxEncodedSize)
		writer.Raw(probe.Image)
	}
	data, err := writer.Bytes()
	if err != nil {
		return nil, ErrCubemaps
	}
	return data, nil
}

func Decode(data []byte) (Bundle, error) {
	// Scan complete lengths before allocating tables. Image bytes remain borrowed
	// from the admitted entry and are decoded only after aggregate preflight.
	scan := binwire.NewReader(data, "KRC1", MaxEncodedSize)
	readBundle(scan, false)
	if scan.Done() != nil {
		return Bundle{}, ErrCubemaps
	}
	reader := binwire.NewReader(data, "KRC1", MaxEncodedSize)
	bundle := readBundle(reader, true)
	if reader.Done() != nil {
		return Bundle{}, ErrCubemaps
	}
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}
func readBundle(reader *binwire.Reader, allocate bool) Bundle {
	bundle := Bundle{InputSHA256: reader.String(64), Producer: reader.String(64)}
	for kind, maximum := range []int{world.MaxSectors, world.MaxStaticSolids} {
		count, _ := reader.Count(maximum, 4, 8)
		var bindings []int
		if allocate {
			bindings = make([]int, count)
		}
		for index := range count {
			id := reader.I32()
			if allocate {
				bindings[index] = id
			}
		}
		if kind == 0 {
			bundle.Sectors = bindings
		} else {
			bundle.Solids = bindings
		}
	}
	count, _ := reader.Count(MaxProbes, 44, 64)
	if allocate {
		bundle.Probes = make([]Probe, count)
	}
	for index := range count {
		probe := Probe{ID: reader.I32()}
		points := [3][3]float32{}
		for point := range 3 {
			for axis := range 3 {
				points[point][axis] = reader.F32()
			}
		}
		probe.Origin, probe.Low, probe.High = points[0], points[1], points[2]
		size, _ := reader.Count(MaxEncodedSize, 1, 0)
		probe.Image = reader.Raw(size)
		if allocate {
			bundle.Probes[index] = probe
		}
	}
	return bundle
}
