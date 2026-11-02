package reflectionplan

import (
	sdkworld "github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

func reflectionProbeForSolid(
	document *sdkworld.Document,
	index int,
	groups *[sdkworld.MaxSectors]int,
	volumes *[sdkworld.MaxSectors]worldreflection.Probe,
) int {
	if document.StaticSolids == nil || index < 0 || index >= len(document.StaticSolids.Items) {
		return -1
	}

	solid := document.StaticSolids.Items[index]

	var center sdkworld.Vec2
	for _, point := range solid.Footprint {
		center.X += point.X / float64(len(solid.Footprint))
		center.Y += point.Y / float64(len(solid.Footprint))
	}

	height := ((solid.Top.A+solid.Bottom.A)*center.X + (solid.Top.B+solid.Bottom.B)*center.Y + solid.Top.C + solid.Bottom.C) * reflectionMidpoint
	best := -1
	largest := float32(0)

	for sector, group := range groups {
		if group < 0 || sector >= len(document.Sectors) {
			continue
		}

		cell := document.Sectors[sector]
		contained := true

		for _, wall := range cell.Walls {
			if (wall.End.X-wall.Start.X)*(center.Y-wall.Start.Y)-(wall.End.Y-wall.Start.Y)*(center.X-wall.Start.X) < -1e-7 {
				contained = false

				break
			}
		}

		if contained && height >= float64(volumes[group].Low[2]) && height <= float64(volumes[group].High[2]) {
			volume := volumes[group]

			size := (volume.High[0] - volume.Low[0]) * (volume.High[1] - volume.Low[1]) * (volume.High[2] - volume.Low[2])
			if size > largest {
				best = group
				largest = size
			}
		}
	}

	return best
}
