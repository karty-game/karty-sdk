package character

import (
	"encoding/binary"
	"math"
)

func encodedSize(document Document) int {
	size := 12
	for _, bank := range document.Banks {
		size += 4 + len(bank.ID)
		for _, clip := range bank.Clips {
			size += 8 + len(clip.Name)
			for _, track := range clip.Tracks {
				size += 8 + len(track.Node) + 4*(len(track.Times)+len(track.Values))
			}
		}
	}
	for _, model := range document.Models {
		size += 26 + len(
			model.ID,
		) + 4*(len(model.Vertices)+len(model.Indices)+len(model.InverseBind)) + len(
			model.AlbedoQOI,
		) + 2*len(
			model.Joints,
		)
		for _, node := range model.Nodes {
			size += 44 + len(node.Name)
		}
	}
	return size
}

type encoder struct{ data []byte }

func (writer *encoder) u16(value int) {
	writer.data = binary.LittleEndian.AppendUint16(writer.data, uint16(value))
}
func (writer *encoder) u32(value uint32) {
	writer.data = binary.LittleEndian.AppendUint32(writer.data, value)
}
func (writer *encoder) f32(value float32) { writer.u32(math.Float32bits(value)) }
func (writer *encoder) text(value string) {
	writer.u16(len(value))
	writer.data = append(writer.data, value...)
}
func (writer *encoder) floats(values []float32) {
	for _, value := range values {
		writer.f32(value)
	}
}

// Encode validates the complete resource before allocating its canonical bytes.
func Encode(document Document) ([]byte, error) {
	if err := document.Validate(); err != nil {
		return nil, err
	}
	size := encodedSize(document)
	if size > MaxBytes {
		return nil, ErrInvalid
	}
	writer := encoder{data: make([]byte, 0, size)}
	writer.data = append(writer.data, "KCHR"...)
	writer.u16(Version)
	writer.u16(0)
	writer.u16(len(document.Models))
	writer.u16(len(document.Banks))
	for _, bank := range document.Banks {
		writer.text(bank.ID)
		writer.u16(len(bank.Clips))
		for _, clip := range bank.Clips {
			writer.text(clip.Name)
			writer.f32(clip.Duration)
			writer.u16(len(clip.Tracks))
			for _, track := range clip.Tracks {
				writer.text(track.Node)
				writer.data = append(writer.data, track.Path, 0)
				if track.Step {
					writer.data[len(writer.data)-1] = 1
				}
				writer.u32(uint32(len(track.Times)))
				writer.floats(track.Times)
				writer.floats(track.Values)
			}
		}
	}
	for _, model := range document.Models {
		writer.text(model.ID)
		writer.u16(model.Bank)
		writer.u16(len(model.Nodes))
		writer.u16(model.MeshNode)
		writer.u16(len(model.Joints))
		writer.u16(model.Width)
		writer.u16(model.Height)
		writer.u32(uint32(len(model.Vertices)))
		writer.floats(model.Vertices)
		writer.u32(uint32(len(model.Indices)))
		for _, index := range model.Indices {
			writer.u32(index)
		}
		writer.u32(uint32(len(model.AlbedoQOI)))
		writer.data = append(writer.data, model.AlbedoQOI...)
		writer.floats(model.InverseBind)
		for _, node := range model.Nodes {
			writer.text(node.Name)
			writer.u16(node.Parent)
			writer.floats(node.Translation[:])
			writer.floats(node.Rotation[:])
			writer.floats(node.Scale[:])
		}
		for _, joint := range model.Joints {
			writer.u16(int(joint))
		}
	}
	if len(writer.data) != size {
		return nil, ErrInvalid
	}
	return writer.data, nil
}

type decoder struct {
	data []byte
	at   int
	bad  bool
}

func (reader *decoder) take(count int) []byte {
	if count < 0 || count > len(reader.data)-reader.at {
		reader.bad = true
		return nil
	}
	data := reader.data[reader.at : reader.at+count]
	reader.at += count
	return data
}
func (reader *decoder) u16() int {
	data := reader.take(2)
	if data == nil {
		return 0
	}
	return int(binary.LittleEndian.Uint16(data))
}
func (reader *decoder) u32() uint32 {
	data := reader.take(4)
	if data == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}
func (reader *decoder) f32() float32 { return math.Float32frombits(reader.u32()) }
func (reader *decoder) count(maximum int) int {
	count := uint64(reader.u32())
	if count > uint64(maximum) {
		reader.bad = true
		return 0
	}
	return int(count)
}
func (reader *decoder) small(maximum int) int {
	count := reader.u16()
	if count > maximum {
		reader.bad = true
		return 0
	}
	return count
}
func (reader *decoder) text() string {
	count := reader.small(MaxName)
	return string(reader.take(count))
}
func (reader *decoder) floats(count int) []float32 {
	data := reader.take(count * 4)
	if data == nil {
		return nil
	}
	values := make([]float32, count)
	for index := range values {
		values[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
	}
	return values
}

// Decode bounds every count before allocation and rejects trailing bytes.
func Decode(data []byte) (*Document, error) {
	if len(data) < 12 || len(data) > MaxBytes || string(data[:4]) != "KCHR" {
		return nil, ErrInvalid
	}
	reader := decoder{data: data, at: 4}
	if reader.u16() != Version || reader.u16() != 0 {
		return nil, ErrInvalid
	}
	modelCount, bankCount := reader.small(MaxModels), reader.small(MaxBanks)
	document := &Document{Models: make([]Model, modelCount), Banks: make([]Bank, bankCount)}
	for index := range document.Banks {
		bank := &document.Banks[index]
		bank.ID = reader.text()
		bank.Clips = make([]Clip, reader.small(MaxClips))
		keys := 0
		for clipIndex := range bank.Clips {
			clip := &bank.Clips[clipIndex]
			clip.Name = reader.text()
			clip.Duration = reader.f32()
			clip.Tracks = make([]Track, reader.small(MaxTracks))
			for trackIndex := range clip.Tracks {
				track := &clip.Tracks[trackIndex]
				track.Node = reader.text()
				flags := reader.take(2)
				if flags == nil || flags[0] > Scale || flags[1] > 1 {
					return nil, ErrInvalid
				}
				track.Path, track.Step = flags[0], flags[1] != 0
				count := reader.count(MaxKeys - keys)
				keys += count
				dimension := 3
				if track.Path == Rotation {
					dimension = 4
				}
				track.Times = reader.floats(count)
				track.Values = reader.floats(count * dimension)
			}
		}
	}
	for index := range document.Models {
		model := &document.Models[index]
		model.ID = reader.text()
		model.Bank = reader.u16()
		nodeCount := reader.small(MaxNodes)
		model.MeshNode = reader.u16()
		jointCount := reader.small(MaxJoints)
		model.Width = reader.small(MaxTextureSize)
		model.Height = reader.small(MaxTextureSize)
		model.Vertices = reader.floats(reader.count(MaxVertices * VertexStride))
		count := reader.count(MaxIndices)
		raw := reader.take(count * 4)
		if raw == nil {
			return nil, ErrInvalid
		}
		model.Indices = make([]uint32, count)
		for at := range model.Indices {
			model.Indices[at] = binary.LittleEndian.Uint32(raw[at*4:])
		}
		raw = reader.take(reader.count(MaxImageBytes))
		model.AlbedoQOI = append([]byte(nil), raw...)
		model.InverseBind = reader.floats(jointCount * 16)
		model.Nodes = make([]Node, nodeCount)
		for nodeIndex := range model.Nodes {
			node := &model.Nodes[nodeIndex]
			node.Name = reader.text()
			node.Parent = int(int16(reader.u16()))
			for at := range node.Translation {
				node.Translation[at] = reader.f32()
			}
			for at := range node.Rotation {
				node.Rotation[at] = reader.f32()
			}
			for at := range node.Scale {
				node.Scale[at] = reader.f32()
			}
		}
		model.Joints = make([]uint16, jointCount)
		for at := range model.Joints {
			model.Joints[at] = uint16(reader.u16())
		}
	}
	if reader.bad || reader.at != len(data) {
		return nil, ErrInvalid
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return document, nil
}
