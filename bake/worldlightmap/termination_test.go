package worldlightmapbake

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image/color"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestAbsorbingSurfacesEndIndirectPaths(t *testing.T) {
	t.Parallel()
	d, l, materials := roomWithDensity(t, 0, 4)
	for i := range materials {
		materials[i].Albedo = solid(color.NRGBA{A: 255})
	}
	const samples = 64
	result, err := Bake(context.Background(), d, l, materials, Options{Samples: samples, Bounces: 4, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Stats.BounceRays, uint64(result.Stats.ReceiverTexels*samples); got != want {
		t.Fatalf("absorbing point-light paths cast %d rays, want %d", got, want)
	}
	if result.Stats.ShadowRays > uint64(result.Stats.ReceiverTexels) {
		t.Fatal("absorbing surfaces cast unnecessary indirect shadow rays")
	}
	if got := fmt.Sprintf(
		"%x",
		sha256.Sum256(result.Image.Pix),
	); got != "3a3190fdf7f5cba837d0a8334d85373bb7587633bfca691daeeaa00450f91a59" {
		t.Fatal("absorbing point-light pixels changed from the pre-termination producer")
	}

	d.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	d.Lighting = nil
	l, err = worldlightmap.Compile(d, worldlightmap.Options{TexelsPerUnit: 4, PageSize: 512, Padding: 2})
	if err != nil {
		t.Fatal(err)
	}
	result, err = BakeEnvironment(context.Background(), d, l, materials, world.EnvironmentSettings{TimeOfDay: 12, CloudAmount: 1},
		Options{Samples: samples, Bounces: 4, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Stats.BounceRays, uint64(result.Stats.ReceiverTexels*samples); got != want {
		t.Fatalf("absorbing environment paths cast %d rays, want %d", got, want)
	}
	if got := fmt.Sprintf(
		"%x",
		sha256.Sum256(result.Image.Pix),
	); got != "a02f66f4d0e8e4d0f870826cd3893191097ce10d5bef658c1a2f5f73b1378385" {
		t.Fatal("absorbing environment pixels changed from the pre-termination producer")
	}
	// Keep the receiver away from coincident wall/floor endpoints, whose
	// historical self-hit tolerances are covered by producer compatibility tests.
	x, y := floorAt(l, vec{X: 2, Y: 2})
	for basis := range 3 {
		p := result.Image.NRGBAAt(x+basis*l.Pages[0].Width, y)
		if p.A == 0 || p.R != 0 || p.G != 0 || p.B != 0 {
			t.Fatal("closed absorbing interior receiver gained light or lost coverage")
		}
	}
}
