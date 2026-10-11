"""Build brand.vpk from the editable CSS and RGBA logo in this directory.

Resource layouts follow ValveResourceFormat's Panorama/Texture readers.
This constructs binary resources, rather than invoking Valve resourcecompiler.
"""

import argparse
import hashlib
import json
import struct
import tempfile
import zlib
from pathlib import Path

from PIL import Image


ROOT = Path(__file__).resolve().parent
DEFAULT_OUTPUT_DIR = ROOT.parents[1] / "internal" / "producegame" / "assets"
STYLE_PATH = "panorama/styles/hud/hud.vcss_c"
TEXTURE_PATH = "panorama/images/custom/cs2ht_brand_logo_rgba.vtex_c"


def read_blocks(blob):
    size, header_version, version, relative, count = struct.unpack_from("<IHHII", blob)
    if size != len(blob) or header_version != 12:
        raise ValueError("Invalid HUD resource template")
    blocks = []
    for i in range(count):
        at = 8 + relative + i * 12
        name, offset, length = struct.unpack_from("<4sII", blob, at)
        start = at + 4 + offset
        if start + length > len(blob):
            raise ValueError("Truncated HUD resource template")
        blocks.append((name, blob[start:start + length]))
    return version, blocks


def resource(version, blocks):
    blob = bytearray(16 + len(blocks) * 12)
    struct.pack_into("<IHHII", blob, 0, 0, 12, version, 8, len(blocks))
    for i, (name, data) in enumerate(blocks):
        blob.extend(b"\0" * (-len(blob) % 16))
        start = len(blob)
        at = 16 + i * 12
        struct.pack_into("<4sII", blob, at, name, start - at - 4, len(data))
        blob.extend(data)
    struct.pack_into("<I", blob, 0, len(blob))
    return bytes(blob)


def style_resource(template, css):
    version, blocks = read_blocks(template)
    data = dict(blocks)[b"DATA"]
    if version != 3 or struct.unpack_from("<H", data, 4)[0] != 0:
        raise ValueError("Unsupported HUD template format")
    css.decode("utf-8")
    new_data = struct.pack("<IH", zlib.crc32(css), 0) + css
    # Preserve dependency metadata; omit the obsolete source map.
    return resource(version, [
        (name, new_data if name == b"DATA" else content)
        for name, content in blocks if name != b"SrMa"
    ])


def rgba_texture(image_path):
    with Image.open(image_path) as image:
        if image.mode != "RGBA" or image.size != (384, 128):
            raise ValueError("logo.png must be a 384x128 RGBA image")
        if image.getchannel("A").getextrema()[0] != 0:
            raise ValueError("logo.png must contain transparent pixels")
        pixels = image.tobytes()
    data = struct.pack("<HH4fHHHBBIII", 1, 1 | 2 | 8,
                       0, 0, 0, 0, 384, 128, 1, 4, 1, 0, 0, 0)
    # RGBA8888 single-mip pixels follow DATA, outside resource FileSize.
    return resource(1, [(b"DATA", data)]) + pixels


def vpk(files):
    grouped = {}
    for name, data in files.items():
        path = Path(name)
        grouped.setdefault(path.suffix[1:], {}).setdefault(path.parent.as_posix(), []).append((path.stem, data))
    tree = bytearray()
    payload = bytearray()
    for extension in sorted(grouped):
        tree.extend(extension.encode() + b"\0")
        for directory in sorted(grouped[extension]):
            tree.extend(directory.encode() + b"\0")
            for name, data in sorted(grouped[extension][directory]):
                tree.extend(name.encode() + b"\0")
                tree.extend(struct.pack("<IHHIIH", zlib.crc32(data), 0, 0x7FFF,
                                        len(payload), len(data), 0xFFFF))
                payload.extend(data)
            tree.extend(b"\0")
        tree.extend(b"\0")
    tree.extend(b"\0")
    header = struct.pack("<7I", 0x55AA1234, 2, len(tree), len(payload), 0, 48, 0)
    prefix = header + tree + payload
    hashes = hashlib.md5(tree).digest() + hashlib.md5(b"").digest()
    return prefix + hashes + hashlib.md5(prefix + hashes).digest()


def atomic_write(path, content):
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=path.parent, prefix=path.name + ".", delete=False) as target:
            temporary = Path(target.name)
            target.write(content)
        temporary.replace(path)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description="Build brand.vpk without modifying hud.brand.css or logo.png.")
    parser.add_argument("--image", type=Path, default=ROOT / "logo.png")
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR,
                        help="VPK output directory (default: internal/producegame/assets)")
    args = parser.parse_args()
    css = (ROOT / "hud.brand.css").read_bytes()
    template = (ROOT / "hud.template.vcss_c").read_bytes()
    files = {
        STYLE_PATH: style_resource(template, css),
        TEXTURE_PATH: rgba_texture(args.image),
    }
    content = vpk(files)
    output = args.output_dir / "brand.vpk"
    report = {
        "vpk": output.name,
        "bytes": len(content),
        "sha256": hashlib.sha256(content).hexdigest(),
        "image_dimensions": [384, 128],
        "entries": {name: {"bytes": len(value), "crc32": f"{zlib.crc32(value):08x}"}
                    for name, value in files.items()},
        "method": "Panorama DATA CRC rebuild from hud.brand.css; 384x128 single-mip RGBA8888 texture",
        "runtime_tested": False,
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    atomic_write(output, content)
    atomic_write(ROOT / "manifest.json", (json.dumps(report, indent=2) + "\n").encode("utf-8"))
    print(f"Built: {output}\nBytes: {len(content)}\nSHA-256: {report['sha256']}")


if __name__ == "__main__":
    main()
