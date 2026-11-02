#!/usr/bin/env python3
"""Regenerate current lossless mip fixtures with the SDK-pinned KTX 4.4.2 CLI."""
import argparse
from pathlib import Path
import subprocess
import tempfile


def encode(encoder, target, face_size, faces, layers):
    target.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="karty-ktx-fixture-") as directory:
        directory = Path(directory)
        arguments = [encoder, "create", "--raw", "--format", "R8G8B8A8_SRGB",
                     "--width", str(face_size * faces), "--height", str(face_size),
                     "--layers", str(layers), "--levels", "4", "--zstd", "18", "--threads", "2"]
        for level in range(4):
            size = face_size >> level
            for layer in range(layers):
                row = b"".join(bytes((32 + face * 28, 64 + layer * 24, 96, 255)) * size for face in range(faces))
                name = f"{level * layers + layer:04x}"
                (directory / name).write_bytes(row * size)
                arguments.append(name)
        subprocess.run(arguments + [str(target.resolve())], cwd=directory, check=True)


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--ktx", default="ktx")
options = parser.parse_args()
root = Path(__file__).resolve().parents[3]
encode(options.ktx, root / "codec/ktx2/testdata/srgb-rgba8-mips.ktx2", 8, 1, 2)
encode(options.ktx, root / "format/worldreflection/testdata/prepared.ktx2", 128, 6, 1)
