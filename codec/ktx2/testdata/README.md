# Upstream encoder interoperability fixture

`opaque-array.ktx2` is an 820-byte output of official Khronos KTX-Software 4.4.2
Linux x86_64 `ktx`. Its SHA-256 is
`34485369bc4feef09cbca96cf742e07729a3d9053502eb9eb0584a267d848743`.
The official upstream binary archive SHA-256 was
`a8781bad05f9624edbf910b7f258cd0a4ba7d3e63b49ecc0a0ab440bf6a0a245`.

Generate eighteen tightly packed raw RGBA8 inputs, one per mip/layer. Mips are
0..8, layers are 0..1, dimensions are `max(1,256>>mip)` square, and every pixel
is `(85+25*layer,85,85,255)`. Pass inputs in mip-major then layer-major order:

```sh
ktx create --raw --format R8G8B8A8_UNORM --width 256 --height 256 \
  --layers 2 --levels 9 --encode uastc --uastc-quality 4 --zstd 18 \
  --threads 2 <eighteen-raw-inputs> opaque-array.ktx2
```

No mip generation, normal-map preset or channel swizzle is requested. The encoder
validly optimizes constant-1 alpha to DFD channel type RGB. It is consequently
an interoperability regression for implicit-alpha support. Inspection of this
fixture proves container acceptance, not runtime transcoding or GPU rendering.

`linear-rgba8-array.ktx2` was produced with official Khronos KTX-Software 4.4.2:

```sh
ktx create --raw --format R8G8B8A8_UNORM --width 4 --height 4 --layers 3 \
  --levels 1 --zstd 18 layer0.rgba layer1.rgba layer2.rgba linear-rgba8-array.ktx2
```

Each input is a tightly packed 4×4 image with constant RGBA
`(30 + 30*layer, 70, 110, 120 + 10*layer)`. The output is linear and has one mip.
