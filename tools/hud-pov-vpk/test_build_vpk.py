import struct
import unittest
import zlib

import build_vpk as builder


class PovVPKTests(unittest.TestCase):
    def test_rebuild_matches_embedded_asset(self):
        self.assertEqual(builder.build(), builder.build())
        self.assertEqual(builder.build(), builder.DEFAULT_OUTPUT.read_bytes())

    def test_only_teamcounter_resource_changes(self):
        before = builder.read_vpk((builder.ROOT / "pov.template.vpk").read_bytes())
        after = builder.read_vpk(builder.build())  # Checks every entry's CRC.
        self.assertEqual(set(before), set(after))
        self.assertEqual(len(after), 3)
        for name in before:
            if name == builder.STYLE_PATH:
                self.assertNotEqual(before[name], after[name])
            else:
                self.assertEqual(before[name], after[name], name)

    def test_preserves_original_css_images_and_metadata(self):
        before = builder.read_vpk((builder.ROOT / "pov.template.vpk").read_bytes())
        after = builder.read_vpk(builder.build())
        _, old_blocks = builder.read_resource(before[builder.STYLE_PATH])
        _, new_blocks = builder.read_resource(after[builder.STYLE_PATH])
        old, new = dict(old_blocks), dict(new_blocks)
        for name in old:
            if name not in (b"DATA", b"SrMa"):
                self.assertEqual(old[name], new[name], name)
        old_pos = builder.panorama_text_offset(old[b"DATA"])
        new_pos = builder.panorama_text_offset(new[b"DATA"])
        self.assertEqual(old[b"DATA"][4:old_pos], new[b"DATA"][4:new_pos])
        original_css = old[b"DATA"][old_pos:]
        override = (builder.ROOT / "hudteamcounter.pov.css").read_bytes()
        css = new[b"DATA"][new_pos:]
        self.assertEqual(css, original_css + b"\n" + override)
        self.assertEqual(struct.unpack_from("<I", new[b"DATA"])[0], zlib.crc32(css))

    def test_rejects_corrupt_template(self):
        blob = bytearray((builder.ROOT / "pov.template.vpk").read_bytes())
        tree_size = struct.unpack_from("<I", blob, 8)[0]
        blob[28 + tree_size] ^= 1
        with self.assertRaisesRegex(ValueError, "CRC mismatch"):
            builder.read_vpk(blob)


if __name__ == "__main__":
    unittest.main()
