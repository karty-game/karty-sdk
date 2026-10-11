package actions

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"

	"github.com/karty-game/karty-sdk/format/level"
)

func Encode(document Document) ([]byte, error) {
	if err := Validate(document); err != nil {
		return nil, err
	}
	w := actionWriter{data: []byte{'K', 'T', 'Y', 'A', 2, 0, 0, 0}}
	w.data = binary.LittleEndian.AppendUint32(w.data, uint32(document.Version))
	if !w.blob([]byte(document.Schema)) {
		return nil, malformed()
	}
	w.data = binary.LittleEndian.AppendUint32(w.data, uint32(len(document.Sequences)))
	for _, sequence := range document.Sequences {
		if !w.blob([]byte(sequence.Name)) || !w.blob([]byte(sequence.OnRepeat)) || !w.steps(sequence.Steps) {
			return nil, malformed()
		}
	}
	if len(w.data) > MaxBytes {
		return nil, malformed()
	}
	return w.data, nil
}

func Decode(data []byte) (Document, error) {
	if len(data) < 20 || len(data) > MaxBytes || string(data[:4]) != "KTYA" ||
		binary.LittleEndian.Uint16(data[4:6]) != BinaryVersion || binary.LittleEndian.Uint16(data[6:8]) != 0 {
		return Document{}, malformed()
	}
	// First pass checks the complete nested batch and argument metadata without
	// allocating branch slices. The second pass only sees validated wire counts.
	for _, allocate := range []bool{false, true} {
		r := actionReader{data: data, offset: 8, allocate: allocate}
		document, ok := r.document()
		if !ok || r.offset != len(data) {
			return Document{}, malformed()
		}
		if allocate {
			return document, Validate(document)
		}
	}
	return Document{}, malformed()
}

func malformed() error { return fmt.Errorf("invalid binary actions document") }

type actionWriter struct{ data []byte }

func (w *actionWriter) blob(data []byte) bool {
	if len(data) > MaxBytes-4 || len(w.data) > MaxBytes-4-len(data) {
		return false
	}
	w.data = binary.LittleEndian.AppendUint32(w.data, uint32(len(data)))
	w.data = append(w.data, data...)
	return true
}
func (w *actionWriter) steps(steps []Step) bool {
	count := uint32(0)
	if steps != nil {
		count = uint32(len(steps) + 1)
	}
	w.data = binary.LittleEndian.AppendUint32(w.data, count)
	for _, step := range steps {
		if !w.blob([]byte(step.Action)) || !w.blob([]byte(step.Condition)) {
			return false
		}
		w.data = binary.LittleEndian.AppendUint64(w.data, step.WaitFrames)
		var args []byte
		if step.Args != nil {
			var err error
			args, err = level.EncodeMetadata(step.Args)
			if err != nil {
				return false
			}
		}
		if !w.blob(args) || !w.steps(step.Then) || !w.steps(step.Else) || !w.steps(step.OnFailure) {
			return false
		}
	}
	return len(w.data) <= MaxBytes
}

type actionReader struct {
	data          []byte
	offset, count int
	allocate      bool
}

func (r *actionReader) take(n int) ([]byte, bool) {
	if n < 0 || n > len(r.data)-r.offset {
		return nil, false
	}
	v := r.data[r.offset : r.offset+n]
	r.offset += n
	return v, true
}
func (r *actionReader) u32() (uint32, bool) {
	v, ok := r.take(4)
	if !ok {
		return 0, false
	}
	return binary.LittleEndian.Uint32(v), true
}
func (r *actionReader) blob(maximum int) ([]byte, bool) {
	n, ok := r.u32()
	if !ok || uint64(n) > uint64(maximum) {
		return nil, false
	}
	return r.take(int(n))
}
func (r *actionReader) text(maximum int) (string, bool) {
	v, ok := r.blob(maximum)
	if !ok || !utf8.Valid(v) {
		return "", false
	}
	if r.allocate {
		return string(v), true
	}
	return "", true
}
func (r *actionReader) document() (Document, bool) {
	version, ok := r.u32()
	if !ok || version != Version {
		return Document{}, false
	}
	schema, ok := r.text(1024)
	if !ok {
		return Document{}, false
	}
	n, ok := r.u32()
	if !ok || n > MaxSequences || uint64(n)*12 > uint64(len(r.data)-r.offset) {
		return Document{}, false
	}
	v := Document{Version: int(version), Schema: schema}
	if r.allocate {
		v.Sequences = make([]Sequence, int(n))
	}
	for index := 0; index < int(n); index++ {
		name, ok := r.text(128)
		if !ok {
			return Document{}, false
		}
		repeat, ok := r.text(128)
		if !ok {
			return Document{}, false
		}
		steps, ok := r.steps(1)
		if !ok {
			return Document{}, false
		}
		if r.allocate {
			v.Sequences[index] = Sequence{Name: name, OnRepeat: repeat, Steps: steps}
		}
	}
	return v, true
}
func (r *actionReader) steps(depth int) ([]Step, bool) {
	n, ok := r.u32()
	if !ok {
		return nil, false
	}
	if n == 0 {
		return nil, true
	}
	n--
	if n > uint32(MaxSteps-r.count) || (n > 0 && depth > MaxDepth) || uint64(n)*32 > uint64(len(r.data)-r.offset) {
		return nil, false
	}
	var steps []Step
	if r.allocate {
		steps = make([]Step, int(n))
	}
	for index := 0; index < int(n); index++ {
		r.count++
		if r.count > MaxSteps {
			return nil, false
		}
		action, ok := r.text(128)
		if !ok {
			return nil, false
		}
		condition, ok := r.text(128)
		if !ok {
			return nil, false
		}
		wait, ok := r.take(8)
		if !ok {
			return nil, false
		}
		args, ok := r.blob(level.MaxMetadataSize)
		if !ok || (len(args) != 0 && level.ValidateMetadata(args) != nil) {
			return nil, false
		}
		step := Step{Action: action, Condition: condition, WaitFrames: binary.LittleEndian.Uint64(wait)}
		if r.allocate && len(args) > 0 {
			step.Args, _ = level.DecodeMetadata(args)
		}
		step.Then, ok = r.steps(depth + 1)
		if !ok {
			return nil, false
		}
		step.Else, ok = r.steps(depth + 1)
		if !ok {
			return nil, false
		}
		step.OnFailure, ok = r.steps(depth + 1)
		if !ok {
			return nil, false
		}
		if r.allocate {
			steps[index] = step
		}
	}
	return steps, true
}
