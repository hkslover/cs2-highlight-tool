"""Patch the shipped POV template using only Python's standard library.

Panorama v3 DATA layout follows ValveResourceFormat's Panorama reader.
Only hudteamcounter.vcss_c changes; the two other HUD resources stay byte-identical.
"""

import argparse
import hashlib
import struct
import tempfile
import zlib
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DEFAULT_OUTPUT = ROOT.parents[1] / "internal/producegame/assets/pov.vpk"
STYLE_PATH = "panorama/styles/hud/hudteamcounter.vcss_c"


def read_vpk(blob):
    signature, version, tree_size, data_size, _, _, _ = struct.unpack_from("<7I", blob)
    if signature != 0x55AA1234 or version != 2:
        raise ValueError("Expected a VPK v2 template")
    pos = 28
    data_start = pos + tree_size

    def string():
        nonlocal pos
        end = blob.index(0, pos, data_start)
        value = blob[pos:end].decode("utf-8")
        pos = end + 1
        return value

    files = {}
    while extension := string():
        while directory := string():
            while name := string():
                crc, preload, archive, offset, size, terminator = struct.unpack_from("<IHHIIH", blob, pos)
                pos += 18
                if archive != 0x7FFF or terminator != 0xFFFF or offset + size > data_size:
                    raise ValueError("Unsupported or truncated VPK entry")
                content = blob[pos:pos + preload] + blob[data_start + offset:data_start + offset + size]
                pos += preload
                if zlib.crc32(content) != crc:
                    raise ValueError("VPK entry CRC mismatch")
                files[f"{directory}/{name}.{extension}"] = content
    if pos != data_start:
        raise ValueError("Invalid VPK tree size")
    return files


def write_vpk(files):
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
    prefix = struct.pack("<7I", 0x55AA1234, 2, len(tree), len(payload), 0, 48, 0) + tree + payload
    # VPK v2 mandates MD5 for archive integrity, not for security/authentication.
    hashes = (hashlib.md5(tree, usedforsecurity=False).digest()
              + hashlib.md5(b"", usedforsecurity=False).digest())
    return prefix + hashes + hashlib.md5(prefix + hashes, usedforsecurity=False).digest()


def read_resource(blob):
    size, header_version, version, relative, count = struct.unpack_from("<IHHII", blob)
    if size != len(blob) or header_version != 12 or version != 3:
        raise ValueError("Expected a Panorama v3 resource")
    blocks = []
    for i in range(count):
        at = 8 + relative + i * 12
        name, offset, length = struct.unpack_from("<4sII", blob, at)
        start = at + 4 + offset
        if start + length > len(blob):
            raise ValueError("Truncated Panorama resource")
        blocks.append((name, blob[start:start + length]))
    return version, blocks


def panorama_text_offset(data):
    count = struct.unpack_from("<H", data, 4)[0]
    pos = 6
    for _ in range(count):
        # Null-terminated image path + width/height (uint16) + CRC32.
        pos = data.index(0, pos) + 1 + 8
    if pos > len(data):
        raise ValueError("Truncated Panorama image references")
    return pos


def patch_style(blob, override):
    override.decode("utf-8")
    version, blocks = read_resource(blob)
    data = dict(blocks)[b"DATA"]
    pos = panorama_text_offset(data)
    css = data[pos:] + b"\n" + override
    new_data = struct.pack("<I", zlib.crc32(css)) + data[4:pos] + css
    result = bytearray(16 + 12 * sum(name != b"SrMa" for name, _ in blocks))
    kept = [(name, new_data if name == b"DATA" else content)
            for name, content in blocks if name != b"SrMa"]
    struct.pack_into("<IHHII", result, 0, 0, 12, version, 8, len(kept))
    for i, (name, content) in enumerate(kept):
        result.extend(b"\0" * (-len(result) % 16))
        start = len(result)
        at = 16 + i * 12
        struct.pack_into("<4sII", result, at, name, start - at - 4, len(content))
        result.extend(content)
    struct.pack_into("<I", result, 0, len(result))
    return bytes(result)


def build():
    files = read_vpk((ROOT / "pov.template.vpk").read_bytes())
    files[STYLE_PATH] = patch_style(files[STYLE_PATH], (ROOT / "hudteamcounter.pov.css").read_bytes())
    return write_vpk(files)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()
    content = build()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=args.output.parent, delete=False) as target:
            temporary = Path(target.name)
            target.write(content)
        temporary.replace(args.output)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
    print(f"Built: {args.output}\nBytes: {len(content)}\nSHA-256: {hashlib.sha256(content).hexdigest()}")


if __name__ == "__main__":
    main()
