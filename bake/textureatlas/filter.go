// Package textureatlas filters build-time atlas mip chains.
package textureatlas

import "math"

func Mip(source []byte, width, height int, normal bool) []byte {
	if width < 2 || height < 2 || width > 8192 || height > 8192 || len(source) != width*height*4 {
		return nil
	}
	nextWidth, nextHeight := width/2, height/2

	pixels := make([]byte, nextWidth*nextHeight*4)
	for row := range nextHeight {
		for column := range nextWidth {
			average := mipAverage{}
			// Area footprints retain the last row/column of odd-sized sources.
			for sourceRow := row * height / nextHeight; sourceRow < (row+1)*height/nextHeight; sourceRow++ {
				for sourceColumn := column * width / nextWidth; sourceColumn < (column+1)*width/nextWidth; sourceColumn++ {
					average.add(source, (sourceRow*width+sourceColumn)*4, normal)
				}
			}

			pixel := average.pixel(normal)
			copy(pixels[(row*nextWidth+column)*4:], pixel[:])
		}
	}

	return pixels
}

type mipAverage struct {
	channels       [3]float64
	alpha, samples float64
}

func (average *mipAverage) add(source []byte, offset int, normal bool) {
	opacity := float64(source[offset+3]) / 255
	average.alpha += opacity

	average.samples++
	for channel := range average.channels {
		value := float64(source[offset+channel]) / 255
		if normal {
			average.channels[channel] += value*2 - 1
		} else {
			average.channels[channel] += linear(value) * opacity
		}
	}
}
func (average *mipAverage) pixel(normal bool) [4]byte {
	pixel := [4]byte{}

	if normal {
		channels := average.channels

		length := math.Sqrt(channels[0]*channels[0] + channels[1]*channels[1] + channels[2]*channels[2])
		if length < .000001 {
			channels, length = [3]float64{0, 0, 1}, 1
		}

		for channel := range channels {
			pixel[channel] = quantize(channels[channel]/length*.5 + .5)
		}

		pixel[3] = 255
	} else {
		if average.alpha > 0 {
			for channel, value := range average.channels {
				pixel[channel] = quantize(srgb(value / average.alpha))
			}
		}

		pixel[3] = quantize(average.alpha / average.samples)
	}

	return pixel
}

func linear(value float64) float64 {
	if value <= 0.04045 {
		return value / 12.92
	}

	return math.Pow((value+0.055)/1.055, 2.4)
}
func srgb(value float64) float64 {
	if value <= 0.0031308 {
		return value * 12.92
	}

	return 1.055*math.Pow(value, 1/2.4) - 0.055
}
func quantize(value float64) byte { return byte(min(255, max(0, math.Round(value*255)))) }

func Coverage(pixels []byte, scale float64) float64 {
	if len(pixels) == 0 || len(pixels)%4 != 0 {
		return 0
	}
	covered := 0

	for index := 3; index < len(pixels); index += 4 {
		if math.Round(float64(pixels[index])*scale) >= 128 {
			covered++
		}
	}

	return float64(covered) / float64(len(pixels)/4)
}
func PreserveCoverage(pixels []byte, target float64) {
	best, difference := 1.0, math.Abs(Coverage(pixels, 1)-target)

	low, high := 0.0, 256.0
	for range 16 {
		scale := (low + high) / 2

		coverage := Coverage(pixels, scale)
		if delta := math.Abs(coverage - target); delta < difference {
			best, difference = scale, delta
		}

		if coverage < target {
			low = scale
		} else {
			high = scale
		}
	}

	for index := 3; index < len(pixels); index += 4 {
		pixels[index] = byte(min(255, math.Round(float64(pixels[index])*best)))
	}
}
