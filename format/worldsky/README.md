# Project-owned sky resources, version 1 / SDK 0.0.12

`world/sky@1` stores optional skydome artwork and presentation in the level entry
`@world/sky.bin`. The engine owns rendering, while each project owns its PNGs and
authored settings. This candidate feature requires `world/environment@1` and
`world/sectors@1`. It does not alter the compiled world/environment payload or
the frozen `world.Environment` binary encoding.

`Document` contains version 1, a complete `Settings` record, and four optional
images: `CloudSparse`, `CloudDense`, `Background`, and `Horizon`. Each image is one
gzip member containing exactly `width * height * 4` bytes of linear,
premultiplied RGBA8 pixels. The CLI owns bounded PNG decoding, conversion of
straight sRGB color to linear color before alpha premultiplication, and
deterministic gzip packing. Image-free documents are valid. A missing sidecar
uses the host's asset-free gradient and sun, without built-in decorative art.
Missing individual images contribute no artwork. A supplied horizon is the
decorative horizon layer; the legacy backdrop selector does not select engine
artwork. Sky artwork adds no geometry, collision, light or receivers.

Images are bounded to 4,096 pixels per dimension, 4,194,304 total pixels and
16 MiB + 64 KiB packed bytes each. The complete sidecar is at most 24 MiB and
expanded images total at most 64 MiB. `Decode` checks the complete typed payload,
all image dimensions and aggregate limits, then every gzip length, checksum,
trailer and absence of trailing/concatenated members before copying any packed
image payload. Gzip validation uses fixed decompression scratch, rather than
expanded image allocations. Returned packed image buffers own their storage.
`Image.Unpack` reuses destination capacity for upload and repeats strict stream
checks; contents are unspecified on failure.

| Setting                    | Default     | Finite bound         | Meaning                                           |
| -------------------------- | ----------- | -------------------- | ------------------------------------------------- |
| `cloud_uv_scale`           | `[1, 1]`    | Each `(0, 4]`        | Panorama UV scale for both cloud images           |
| `cloud_dense_scroll_scale` | `1`         | `[0, 4]`             | Dense cloud camera-parallax offset multiplier     |
| `cloud_dense_offset`       | `[0, 0]`    | Each `[-1, 1]`       | Dense cloud UV offset                             |
| `cloud_parallax`           | `[0, 0]`    | Each `[-0.01, 0.01]` | Cloud U offset per X/Y unit of camera movement    |
| `horizon_uv_scale`         | `1`         | `(0, 4]`             | Vertical panorama UV scale for the horizon layer  |
| `horizon_parallax`         | `0`         | `[0, 0.01]`          | Horizon camera translation multiplier             |
| `horizon_haze`             | `0`         | `[0, 1]`             | Mix horizon paint toward the sky's horizon color  |
| `horizon_emission`         | `[0, 0, 0]` | Each `[0, 1]`        | Linear RGB tint for warm horizon windows at night |

Runtime camera delta clamps and cloud coverage blending remain bounded rendering
logic. These settings customize authored images without changing illumination
or requiring a host rebuild. The current host has no elapsed-time cloud wind;
cloud offsets change through camera parallax. Source v12 accepts partial setting overrides;
`worldsource.Sky.ResolvedSettings` merges them onto `DefaultSettings` and produces
the complete compiled record.

The handwritten static typed codec uses the ordinary 16-byte `binwire` header
with magic `KSK1`: wire version 1, zero flags/reserved and exact payload length.
Its payload is uint16 document version; 13 float64 setting values in declaration
order; then the four image records in declaration order. Each optional image
starts with a canonical 0/1 presence byte. Present images carry signed int32
width/height, uint32 packed byte length and exactly that many bytes. Nonfinite
floats, negative zero, unsupported versions, malformed presence, truncation and
trailing data are rejected. No runtime reflection or JSON is used.
