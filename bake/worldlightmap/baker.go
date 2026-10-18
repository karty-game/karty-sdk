package worldlightmapbake

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"sync"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// Baker shares immutable transport preparation across receiver chunks and
// environment variants of one world. Keep document, materials and their images
// unchanged until the Baker is discarded. It retains only the last transport
// scope; concurrent calls are safe and still own separate receiver/output data.
type Baker struct {
	document             world.Document
	materials            []Material
	mu                   sync.Mutex
	prepared             *preparedTransport
	scope                []byte
	sampling, reflecting bool
	shadowDigest         [32]byte
}

// NewBaker creates a lazy, world-bound baker. Each Bake or BakeEnvironment call
// performs complete input validation before using shared preparation.
func NewBaker(document world.Document, materials []Material) *Baker {
	return &Baker{document: document, materials: materials}
}

// Bake uses the existing point-light producer with shared world preparation.
func (b *Baker) Bake(ctx context.Context, layout worldlightmap.Layout, options Options) (Result, error) {
	if b == nil {
		return Result{}, fmt.Errorf("nil shared baker")
	}
	return bake(ctx, b.document, layout, b.materials, options, nil, b)
}

// BakeEnvironment uses the existing environment producer with shared preparation.
func (b *Baker) BakeEnvironment(ctx context.Context, layout worldlightmap.Layout, settings world.EnvironmentSettings,
	options Options) (Result, error) {
	if b == nil {
		return Result{}, fmt.Errorf("nil shared baker")
	}
	environment, err := world.ResolveEnvironmentSettings(b.document.Environment, settings)
	if err != nil {
		return Result{}, err
	}
	return bake(ctx, b.document, layout, b.materials, options, &environment, b)
}

type preparedTransport struct {
	surfaces      []worldlightmap.Surface
	materials     []reflectance
	surfaceFrames []sampleFrame
	accel         accelerator
	triangles     int
}

func (b *Baker) transport(ctx context.Context, document *world.Document, inputs bakeInputs, sampling bool) (*preparedTransport, error) {
	if b == nil {
		return prepareTransport(ctx, document, inputs, sampling)
	}
	scope := make([]byte, (len(inputs.surfaces)+7)/8)
	for i, surface := range inputs.surfaces {
		if len(surface.Polygons) > 0 {
			scope[i/8] |= 1 << (i % 8)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reflecting := inputs.options.Bounces > 0
	shadowDigest := shadowTrianglesDigest(inputs.options.ShadowTriangles)
	if b.prepared != nil && bytes.Equal(scope, b.scope) && sampling == b.sampling && reflecting == b.reflecting &&
		shadowDigest == b.shadowDigest {
		return b.prepared, nil
	}
	prepared, err := prepareTransport(ctx, document, inputs, sampling)
	if err != nil {
		return nil, err
	}
	b.prepared, b.scope, b.sampling, b.reflecting = prepared, scope, sampling, reflecting
	b.shadowDigest = shadowDigest
	return prepared, nil
}

func prepareTransport(ctx context.Context, document *world.Document, inputs bakeInputs, sampling bool) (*preparedTransport, error) {
	p := &preparedTransport{surfaces: inputs.surfaces, materials: make([]reflectance, len(inputs.surfaces))}
	if sampling {
		p.surfaceFrames = make([]sampleFrame, len(inputs.surfaces))
	}
	albedos := make(map[uint32]*image.NRGBA)
	for i, surface := range inputs.surfaces {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if sampling {
			p.surfaceFrames[i] = newSampleFrame(surface.Normal)
		}
		id, uv, origin, tangent, reflectanceCap := materialOf(document, surface.Binding)
		r := reflectance{mapping: uv, origin: origin, tangent: tangent, cap: reflectanceCap}
		if inputs.options.Bounces > 0 && len(surface.Polygons) > 0 {
			pixels := albedos[id]
			if pixels == nil {
				var err error
				pixels, err = prepareAlbedo(ctx, inputs.images[id])
				if err != nil {
					return nil, err
				}
				albedos[id] = pixels
			}
			r.image = pixels
			r.width, r.height = float64(pixels.Rect.Dx()), float64(pixels.Rect.Dy())
		}
		p.materials[i] = r
		for _, polygon := range surface.Polygons {
			for j := 1; j+1 < len(polygon); j++ {
				t := triangle{a: polygon[0], b: polygon[j], c: polygon[j+1], surface: i}
				if degenerate(t) {
					continue
				}
				t.bounds = emptyBox().point(t.a).point(t.b).point(t.c)
				p.accel.triangles = append(p.accel.triangles, t)
			}
		}
	}
	for _, points := range inputs.options.ShadowTriangles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		t := triangle{a: points[0], b: points[1], c: points[2], surface: -1}
		t.bounds = emptyBox().point(t.a).point(t.b).point(t.c)
		p.accel.triangles = append(p.accel.triangles, t)
	}
	p.triangles = len(p.accel.triangles)
	p.accel.prepare()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p, nil
}

func degenerate(t triangle) bool {
	return dot(cross(sub(t.b, t.a), sub(t.c, t.a)), cross(sub(t.b, t.a), sub(t.c, t.a))) < 1e-28
}
