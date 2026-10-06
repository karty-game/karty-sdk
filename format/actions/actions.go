// Package actions owns the versioned authored action and actor-reference format.
// Runtime IDs and Go function names never appear in serialized action documents.
package actions

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"unicode/utf8"
)

// Schema is the canonical editor/build schema. SDK bundles ship an exact snapshot.
//
//go:embed schema.json
var Schema []byte

const Version = 1
const MaxBytes = 64 * 1024
const MaxSequences = 64
const MaxSteps = 1024
const MaxDepth = 16

type ActorRef struct {
	Actor string `json:"actor"`
}
type Document struct {
	Schema    string     `json:"$schema,omitempty"`
	Version   int        `json:"version"`
	Sequences []Sequence `json:"sequences"`
}
type Sequence struct {
	Name     string `json:"name"`
	OnRepeat string `json:"onRepeat,omitempty"`
	Steps    []Step `json:"steps"`
}

func rejectFields(data []byte, allowed ...string) error {
	var fields map[string]json.RawMessage
	if err := decode(data, &fields); err != nil {
		return err
	}
	for name := range fields {
		known := false
		for _, candidate := range allowed {
			known = known || name == candidate
		}
		if !known {
			return fmt.Errorf("unsupported field %s", name)
		}
	}
	return nil
}

func (d *Document) UnmarshalJSON(data []byte) error {
	if err := rejectFields(data, "$schema", "version", "sequences"); err != nil {
		return err
	}
	type wire Document
	var value wire
	if err := decode(data, &value); err != nil {
		return err
	}
	*d = Document(value)
	return nil
}

func (s *Sequence) UnmarshalJSON(data []byte) error {
	if err := rejectFields(data, "name", "onRepeat", "steps"); err != nil {
		return err
	}
	type wire Sequence
	var value wire
	if err := decode(data, &value); err != nil {
		return err
	}
	*s = Sequence(value)
	return nil
}

type Step struct {
	Action     string                     `json:"action,omitempty"`
	Condition  string                     `json:"condition,omitempty"`
	Args       map[string]json.RawMessage `json:"args,omitempty"`
	WaitFrames uint64                     `json:"waitFrames,omitempty"`
	Then, Else []Step                     `json:"-"`
	OnFailure  []Step                     `json:"onFailure,omitempty"`
}

// wireStep preserves the case-sensitive branch fields on the public format.
type wireStep struct {
	Action     string                     `json:"action,omitempty"`
	Condition  string                     `json:"condition,omitempty"`
	Args       map[string]json.RawMessage `json:"args,omitempty"`
	WaitFrames uint64                     `json:"waitFrames,omitempty"`
	Then       []Step                     `json:"then,omitempty"`
	Else       []Step                     `json:"else,omitempty"`
	OnFailure  []Step                     `json:"onFailure,omitempty"`
}

func (s *Step) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := decode(data, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{"onFailure": true}
	if _, ok := fields["action"]; ok {
		allowed["action"], allowed["args"] = true, true
	} else if _, ok := fields["condition"]; ok {
		allowed["condition"], allowed["args"], allowed["then"], allowed["else"] = true, true, true, true
	} else {
		allowed = map[string]bool{"waitFrames": true}
	}
	for name := range fields {
		if !allowed[name] {
			return fmt.Errorf("unsupported step field %s", name)
		}
	}
	if allowed["args"] {
		if _, ok := fields["args"]; !ok || bytes.Equal(fields["args"], []byte("null")) {
			return fmt.Errorf("action arguments must be an object")
		}
	}
	if allowed["condition"] {
		if _, ok := fields["then"]; !ok || bytes.Equal(fields["then"], []byte("null")) {
			return fmt.Errorf("condition needs a then array")
		}
	}
	for _, name := range []string{"then", "else", "onFailure"} {
		if raw, ok := fields[name]; ok && bytes.Equal(raw, []byte("null")) {
			return fmt.Errorf("branch must be an array")
		}
	}
	if raw, ok := fields["waitFrames"]; ok {
		var number json.Number
		if err := json.Unmarshal(raw, &number); err != nil {
			return err
		}
		n, err := number.Float64()
		if err != nil || n < 1 || n > 1e9 || math.Trunc(n) != n {
			return fmt.Errorf("invalid waitFrames")
		}
		fields["waitFrames"], _ = json.Marshal(uint64(n))
		data, _ = json.Marshal(fields)
	}
	var w wireStep
	if err := decode(data, &w); err != nil {
		return err
	}
	*s = Step(w)
	return nil
}
func (s Step) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if s.Action != "" {
		fields["action"] = s.Action
		fields["args"] = s.Args
	}
	if s.Condition != "" {
		fields["condition"] = s.Condition
		fields["args"] = s.Args
		fields["then"] = s.Then
	}
	if s.WaitFrames != 0 {
		fields["waitFrames"] = s.WaitFrames
	}
	if len(s.Else) != 0 {
		fields["else"] = s.Else
	}
	if len(s.OnFailure) != 0 {
		fields["onFailure"] = s.OnFailure
	}
	return json.Marshal(fields)
}

var namePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`)

func ValidName(name string) bool { return len(name) <= 128 && namePattern.MatchString(name) }
func decode(data []byte, target any) error {
	if err := checkKeys(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func checkKeys(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate JSON key")
			}
			seen[name] = true
			if err = checkKeys(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case json.Delim('['):
		for decoder.More() {
			if err = checkKeys(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	return nil
}
func Decode(data []byte) (Document, error) {
	var document Document
	if len(data) > MaxBytes || !utf8.Valid(data) {
		return document, fmt.Errorf("actions document exceeds bounds or contains invalid UTF-8")
	}
	if err := decode(data, &document); err != nil {
		return document, err
	}
	return document, Validate(document)
}
func Encode(document Document) ([]byte, error) {
	if err := Validate(document); err != nil {
		return nil, err
	}
	data, err := json.Marshal(document)
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("actions document exceeds byte bound")
	}
	return data, err
}
func Validate(document Document) error {
	if document.Version != Version || document.Sequences == nil || len(document.Sequences) > MaxSequences || len(document.Schema) > 1024 ||
		!utf8.ValidString(document.Schema) {
		return fmt.Errorf("unsupported actions version or sequence limit")
	}
	seen := map[string]bool{}
	count := 0
	for _, sequence := range document.Sequences {
		if !ValidName(sequence.Name) || seen[sequence.Name] || sequence.Steps == nil {
			return fmt.Errorf("duplicate or invalid sequence %q", sequence.Name)
		}
		seen[sequence.Name] = true
		if sequence.OnRepeat != "" && sequence.OnRepeat != "ignore" && sequence.OnRepeat != "restart" && sequence.OnRepeat != "parallel" {
			return fmt.Errorf("invalid onRepeat for %q", sequence.Name)
		}
		if err := validateSteps(sequence.Steps, 1, &count); err != nil {
			return fmt.Errorf("sequence %s: %w", sequence.Name, err)
		}
	}
	return nil
}
func validateSteps(steps []Step, depth int, count *int) error {
	if len(steps) == 0 {
		return nil
	}
	if depth > MaxDepth {
		return fmt.Errorf("branch depth exceeds %d", MaxDepth)
	}
	for index, step := range steps {
		*count++
		if *count > MaxSteps {
			return fmt.Errorf("step limit exceeded")
		}
		operations := 0
		if step.Action != "" {
			operations++
		}
		if step.Condition != "" {
			operations++
		}
		if step.WaitFrames != 0 {
			operations++
		}
		if operations != 1 || (step.Action != "" && !ValidName(step.Action)) || (step.Condition != "" && !ValidName(step.Condition)) {
			return fmt.Errorf("step %d must specify one valid action, condition or wait", index)
		}
		if step.WaitFrames > 1000000000 ||
			(step.WaitFrames != 0 && (step.Args != nil || len(step.Then)+len(step.Else)+len(step.OnFailure) != 0)) ||
			(step.Action != "" && len(step.Then)+len(step.Else) != 0) {
			return fmt.Errorf("invalid fields on step %d", index)
		}
		if (step.Action != "" || step.Condition != "") && step.Args == nil {
			return fmt.Errorf("action needs an argument object")
		}
		if step.Condition != "" && step.Then == nil {
			return fmt.Errorf("condition needs a then array")
		}
		if len(step.Args) > 16 {
			return fmt.Errorf("argument limit exceeded")
		}
		for name, data := range step.Args {
			if !validArgumentName(name) || len(data) > MaxBytes || !utf8.Valid(data) {
				return fmt.Errorf("invalid argument %q", name)
			}
			var value any
			if err := decode(data, &value); err != nil {
				return err
			}
			switch v := value.(type) {
			case bool, float64:
			case string:
				if len(v) > 1024 {
					return fmt.Errorf("argument string exceeds bound")
				}
			case map[string]any:
				actor, ok := v["actor"].(string)
				if !ok || len(v) != 1 || actor == "" || len(actor) > 1024 {
					return fmt.Errorf("invalid actor reference")
				}
			default:
				return fmt.Errorf("unsupported argument %q", name)
			}
		}
		for _, branch := range [][]Step{step.Then, step.Else, step.OnFailure} {
			if err := validateSteps(branch, depth+1, count); err != nil {
				return err
			}
		}
	}
	return nil
}

func validArgumentName(name string) bool {
	if len(name) == 0 || len(name) > 128 || name == "_" {
		return false
	}
	for i, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
