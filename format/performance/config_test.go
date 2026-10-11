package performance

import (
	"errors"
	"math"
	"testing"
)

func TestValidateRequiresCompleteWidthCoverage(t *testing.T) {
	tests := []struct {
		name   string
		bounds [4][2]int
		valid  bool
	}{
		{name: "defaults", bounds: [4][2]int{{1401, 0}, {0, 0}, {0, 0}, {0, 799}}, valid: true},
		{name: "adjacent", bounds: [4][2]int{{1401, 0}, {1000, 1400}, {800, 999}, {0, 799}}, valid: true},
		{name: "overlapping", bounds: [4][2]int{{1401, 0}, {900, 1500}, {700, 1000}, {0, 799}}, valid: true},
		{name: "single pixel gap", bounds: [4][2]int{{1401, 0}, {1001, 1400}, {800, 999}, {0, 799}}},
		{name: "review reproduction", bounds: [4][2]int{{1401, 0}, {1401, 0}, {1401, 0}, {0, 799}}},
		{name: "uncovered smallest width", bounds: [4][2]int{{1401, 0}, {800, 0}, {800, 0}, {2, 799}}},
		{name: "finite largest width", bounds: [4][2]int{{1401, math.MaxInt32}, {800, 1400}, {0, 799}, {0, 799}}},
		{name: "large adjacent widths", bounds: [4][2]int{{math.MaxInt32, 0}, {800, math.MaxInt32 - 1}, {0, 799}, {0, 799}}, valid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Default()
			for index, bounds := range test.bounds {
				config.Profiles[index].MinWindowWidth = bounds[0]
				config.Profiles[index].MaxWindowWidth = bounds[1]
			}
			err := Validate(config)
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && !errors.Is(err, ErrConfig) {
				t.Fatalf("expected invalid config, got %v", err)
			}
			for index, bounds := range test.bounds {
				profile := config.Profiles[index]
				if profile.MinWindowWidth != bounds[0] || profile.MaxWindowWidth != bounds[1] {
					t.Fatal("validation changed profile order or bounds")
				}
			}
		})
	}
}

func TestNormalizeRejectsAuthoredWidthGap(t *testing.T) {
	authored := Config{Profiles: []Profile{
		{Name: "medium", MinWindowWidth: 1401},
		{Name: "low", MinWindowWidth: 1401},
	}}
	if _, err := Normalize(authored); !errors.Is(err, ErrConfig) {
		t.Fatalf("expected invalid config, got %v", err)
	}
	if _, err := Encode(authored); !errors.Is(err, ErrConfig) {
		t.Fatalf("expected encode to reject width gap, got %v", err)
	}
}
