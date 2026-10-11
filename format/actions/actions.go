// Package actions owns the bounded binary runtime action document format.
package actions

import (
	"fmt"
	"math"
	"regexp"
	"unicode/utf8"

	"github.com/karty-game/karty-sdk/format/level"
)

const Version = 1
const BinaryVersion = uint16(2)
const MaxBytes = 64 * 1024
const MaxSequences = 64
const MaxSteps = 1024
const MaxDepth = 16

// ActorRef describes an authored actor name scoped to the current level mount.
// It has no standalone wire encoding; Step.Args stores the reference as a
// level.Metadata object containing one "actor" string.
type ActorRef struct{ Actor string }
type Document struct {
	Schema    string
	Version   int
	Sequences []Sequence
}
type Sequence struct {
	Name, OnRepeat string
	Steps          []Step
}
type Step struct {
	Action, Condition     string
	Args                  level.Metadata
	WaitFrames            uint64
	Then, Else, OnFailure []Step
}

var namePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`)

func ValidName(name string) bool { return len(name) <= 128 && namePattern.MatchString(name) }
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
		for name, value := range step.Args {
			if !validArgumentName(name) {
				return fmt.Errorf("invalid argument %q", name)
			}
			switch value.Kind {
			case level.ValueKindBool, level.ValueKindInt, level.ValueKindUint:
			case level.ValueKindFloat:
				if math.IsNaN(value.Float) || math.IsInf(value.Float, 0) {
					return fmt.Errorf("invalid numeric argument")
				}
			case level.ValueKindString:
				if len(value.String) > 1024 || !utf8.ValidString(value.String) {
					return fmt.Errorf("argument string exceeds bounds")
				}
			case level.ValueKindObject:
				actor, ok := value.Object.String("actor")
				if !ok || len(value.Object) != 1 || actor == "" || len(actor) > 1024 || !utf8.ValidString(actor) {
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
