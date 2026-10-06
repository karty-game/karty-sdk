package worldlightmap

import (
	"fmt"
	"github.com/karty-game/karty-sdk/format/world"
	"testing"
)

func chainDocument(count int) world.Document {
	d := world.Document{Version: world.Version}
	for i := 0; i < count; i++ {
		x := float64(i)
		points := []world.Vec2{{X: x, Y: 0}, {X: x + 1, Y: 0}, {X: x + 1, Y: 1}, {X: x, Y: 1}}
		s := world.Sector{ID: fmt.Sprint(i), SourceRoom: "chain", Floor: world.Plane{}, Ceiling: world.Plane{C: 1}}
		for j, p := range points {
			s.Walls = append(s.Walls, world.Wall{Start: p, End: points[(j+1)%4], Portal: -1})
		}
		if i < count-1 {
			s.Walls[1].Portal = int32(i + 1)
			s.Walls[1].PortalWall = 4
		}
		if i > 0 {
			s.Walls[3].Portal = int32(i - 1)
			s.Walls[3].PortalWall = 2
		}
		d.Sectors = append(d.Sectors, s)
	}
	return d
}

func BenchmarkLightmapLayout(b *testing.B) {
	for _, count := range []int{1, 64, 256} {
		d := chainDocument(count)
		options := Options{TexelsPerUnit: 1, PageSize: 1024}
		layout, err := Compile(d, options)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("Compile%dSectors", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Compile(d, options); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("Validate%dSectors", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := Validate(&layout, &d); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
