package worldlightmap

import (
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldterrain"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

func EncodeFixedLighting(lighting FixedLighting) ([]byte, error) {
	if err := validateFixedStructure(lighting); err != nil {
		return nil, err
	}
	magic := "KFL1"
	if lighting.Schema == FixedCompactSchema {
		magic = "KFL2"
	}
	w := binwire.NewWriter(magic, MaxFixedLightingSize)
	for _, value := range []string{lighting.Schema, lighting.GeometrySHA256, lighting.LayoutSHA256, lighting.SurfaceSHA256, lighting.ReflectanceSHA256, lighting.InputSHA256, lighting.ImageSHA256, lighting.Producer, lighting.TerrainSHA256, lighting.TerrainReceiverSHA256} {
		w.String(value, 128)
	}
	for _, value := range []int{lighting.ImageBytes, lighting.Width, lighting.Height, lighting.Samples, lighting.Bounces} {
		w.I32(value)
	}
	w.U64(lighting.Seed)
	w.String(lighting.Denoise, 16)
	w.F64(lighting.RGBMRange)
	w.Count(len(lighting.Lights), false, MaxCompiledLights)
	for _, light := range lighting.Lights {
		w.String(light.ID, world.MaxIdentifierBytes)
		w.String(light.Kind, 8)
		for _, value := range [3]world.Vec3{light.Position, light.Direction, light.Color} {
			writeFixedVector(w, value)
		}
		w.F64(light.Radius)
	}
	w.Count(len(lighting.Probes), false, MaxLightingProbes)
	for _, probe := range lighting.Probes {
		w.I32(probe.Sector)
		writeFixedVector(w, probe.Position)
		for _, color := range probe.Ambient {
			writeFixedVector(w, color)
		}
		for _, direct := range probe.Direct {
			w.I32(direct.Light)
			w.F64(direct.Visibility)
		}
	}
	w.Count(len(lighting.Terrain), false, worldterrain.MaxPatches)
	for _, patch := range lighting.Terrain {
		w.I32(patch.Patch)
		w.Count(len(patch.Coefficients), false, MaxFixedLightingSize/4)
		for _, value := range patch.Coefficients {
			w.F32(value)
		}
	}
	if lighting.Schema == FixedCompactSchema {
		w.Count(len(lighting.ConstantRNM), false, MaxCharts)
		for _, chart := range lighting.ConstantRNM {
			w.I32(chart.Chart)
			for _, value := range chart.Coefficients {
				w.F32(value)
			}
		}
	}
	data, err := w.Bytes()
	if err != nil {
		return nil, ErrLayout
	}
	return data, nil
}

// DecodeFixedLighting preflights the complete bounded stream before allocating
// owned source/probe slices. Artifact identity admission is a separate operation.
func DecodeFixedLighting(data []byte) (FixedLighting, error) {
	if _, err := readFixedLighting(data, false); err != nil {
		return FixedLighting{}, err
	}
	lighting, err := readFixedLighting(data, true)
	if err != nil {
		return FixedLighting{}, err
	}
	if err := validateFixedStructure(lighting); err != nil {
		return FixedLighting{}, err
	}
	return lighting, nil
}

func readFixedLighting(data []byte, allocate bool) (FixedLighting, error) {
	magic := "KFL1"
	compact := len(data) >= 4 && string(data[:4]) == "KFL2"
	if compact {
		magic = "KFL2"
	}
	r := binwire.NewReader(data, magic, MaxFixedLightingSize)
	var result FixedLighting
	strings := []*string{
		&result.Schema,
		&result.GeometrySHA256,
		&result.LayoutSHA256,
		&result.SurfaceSHA256,
		&result.ReflectanceSHA256,
		&result.InputSHA256,
		&result.ImageSHA256,
		&result.Producer,
		&result.TerrainSHA256,
		&result.TerrainReceiverSHA256,
	}
	for _, target := range strings {
		if allocate {
			*target = r.String(128)
		} else {
			r.SkipString(128)
		}
	}
	for _, target := range []*int{&result.ImageBytes, &result.Width, &result.Height, &result.Samples, &result.Bounces} {
		*target = r.I32()
	}
	result.Seed = r.U64()
	result.Denoise = r.String(16)
	result.RGBMRange = r.F64()
	count, _ := r.Count(MaxCompiledLights, 88, 112)
	if allocate {
		result.Lights = make([]CompiledLight, count)
	}
	for index := 0; index < count; index++ {
		var light CompiledLight
		if allocate {
			light.ID = r.String(world.MaxIdentifierBytes)
			light.Kind = r.String(8)
		} else {
			r.SkipString(world.MaxIdentifierBytes)
			r.SkipString(8)
		}
		light.Position = readFixedVector(r)
		light.Direction = readFixedVector(r)
		light.Color = readFixedVector(r)
		light.Radius = r.F64()
		if allocate {
			result.Lights[index] = light
		}
	}
	count, _ = r.Count(MaxLightingProbes, 196, 216)
	if allocate {
		result.Probes = make([]LightingProbe, count)
	}
	for index := 0; index < count; index++ {
		var probe LightingProbe
		probe.Sector = r.I32()
		probe.Position = readFixedVector(r)
		for face := range probe.Ambient {
			probe.Ambient[face] = readFixedVector(r)
		}
		for direct := range probe.Direct {
			probe.Direct[direct] = ProbeDirect{Light: r.I32(), Visibility: r.F64()}
		}
		if allocate {
			result.Probes[index] = probe
		}
	}
	count, _ = r.Count(worldterrain.MaxPatches, 8, 32)
	if allocate {
		result.Terrain = make([]TerrainPatchLighting, count)
	}
	for index := 0; index < count; index++ {
		patch := r.I32()
		values, _ := r.Count(MaxFixedLightingSize/4, 4, 4)
		var coeff []float32
		if allocate {
			coeff = make([]float32, values)
		}
		for value := 0; value < values; value++ {
			v := r.F32()
			if allocate {
				coeff[value] = v
			}
		}
		if allocate {
			result.Terrain[index] = TerrainPatchLighting{Patch: patch, Coefficients: coeff}
		}
	}
	if compact {
		count, _ = r.Count(MaxCharts, 40, 48)
		if allocate {
			result.ConstantRNM = make([]ChartRNM, count)
		}
		for index := 0; index < count; index++ {
			chart := ChartRNM{Chart: r.I32()}
			for value := range chart.Coefficients {
				chart.Coefficients[value] = r.F32()
			}
			if allocate {
				result.ConstantRNM[index] = chart
			}
		}
	}
	if r.Done() != nil || allocate && compact != (result.Schema == FixedCompactSchema) {
		return FixedLighting{}, ErrLayout
	}
	return result, nil
}

func writeFixedVector(w *binwire.Writer, value world.Vec3) {
	w.F64(value.X)
	w.F64(value.Y)
	w.F64(value.Z)
}
func readFixedVector(r *binwire.Reader) world.Vec3 {
	return world.Vec3{X: r.F64(), Y: r.F64(), Z: r.F64()}
}
