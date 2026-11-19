#!/usr/bin/env python3
"""Check the companion asset sample and production packs.

The sample gate is intentionally strict about dimensions and alpha so a
half-processed image cannot pass visual review. Production checks are added
alongside the full asset pipeline in Tasks 2-3.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
SAMPLE_DIR = ROOT / "tmp" / "companion-sample"
PRODUCTION_DIR = ROOT / "apps" / "web" / "public" / "companion"

Q_SIZE = (768, 768)
OUTFIT_SIZE = (768, 1024)

SAMPLE_FILES = (
    "q/base.png",
    "q/iris-left.png",
    "q/iris-right.png",
    "q/eye-happy.png",
    "q/mouth-smile.png",
    "outfits/outfit-1.png",
)

# The initial dock layer set must stay below this hard cap. The target is
# 180KB; the hard cap is 250KB per the design spec.
Q_INITIAL_FILES = (
    "q/base.webp",
    "q/iris-left.webp",
    "q/iris-right.webp",
    "q/eye-open.webp",
    "q/mouth-neutral.webp",
    "q/blush-soft.webp",
)
Q_INITIAL_HARD_CAP = 250 * 1024
OUTFIT_MAX_BYTES = 300 * 1024
CARD_MAX_BYTES = 80 * 1024
PUBLIC_MAX_BYTES = 350 * 1024


def fail(message: str) -> None:
    print(f"FAIL: {message}")
    raise SystemExit(1)


def require_file(path: Path) -> Image.Image:
    if not path.is_file():
        fail(f"missing {path.relative_to(ROOT)}")
    return Image.open(path)


def require_rgba(path: Path, size: tuple[int, int]) -> Image.Image:
    image = require_file(path)
    if image.size != size:
        fail(f"{path.relative_to(ROOT)} is {image.size}, expected {size}")
    if image.mode != "RGBA":
        fail(f"{path.relative_to(ROOT)} mode is {image.mode}, expected RGBA")
    return image


def alpha_bbox_nonempty(image: Image.Image, label: str) -> None:
    bbox = image.getchannel("A").getbbox()
    if bbox is None:
        fail(f"{label} has an empty alpha bbox")


def corners_transparent(image: Image.Image, label: str) -> None:
    alpha = image.getchannel("A")
    w, h = image.size
    for xy in ((0, 0), (w - 1, 0), (0, h - 1), (w - 1, h - 1)):
        if alpha.getpixel(xy) != 0:
            fail(f"{label} corner {xy} is not transparent")


def has_transparency_and_content(image: Image.Image, label: str) -> None:
    alpha = image.getchannel("A")
    lo, hi = alpha.getextrema()
    if lo != 0:
        fail(f"{label} has no transparent pixels")
    if hi == 0:
        fail(f"{label} is fully transparent")


def check_webp_sizes(directory: Path, max_bytes: int, label: str) -> None:
    for path in sorted(directory.rglob("*.webp")):
        size = path.stat().st_size
        if size > max_bytes:
            fail(f"{label} {path.relative_to(ROOT)} is {size} bytes, over {max_bytes}")


def check_sample() -> None:
    for relative in SAMPLE_FILES:
        require_file(SAMPLE_DIR / relative)

    base = require_rgba(SAMPLE_DIR / "q/base.png", Q_SIZE)
    corners_transparent(base, "q/base.png")
    alpha_bbox_nonempty(base, "q/base.png")

    for name in ("q/iris-left.png", "q/iris-right.png"):
        layer = require_rgba(SAMPLE_DIR / name, Q_SIZE)
        has_transparency_and_content(layer, name)

    for name in ("q/eye-happy.png", "q/mouth-smile.png"):
        layer = require_rgba(SAMPLE_DIR / name, Q_SIZE)
        has_transparency_and_content(layer, name)

    outfit = require_rgba(SAMPLE_DIR / "outfits/outfit-1.png", OUTFIT_SIZE)
    corners_transparent(outfit, "outfits/outfit-1.png")
    alpha_bbox_nonempty(outfit, "outfits/outfit-1.png")

    check_webp_sizes(SAMPLE_DIR, OUTFIT_MAX_BYTES, "sample")
    print("sample assets ok")


def check_production() -> None:
    if not PRODUCTION_DIR.is_dir():
        fail("missing apps/web/public/companion")
    # Task 2/3 extend this to read assets.json and verify every referenced
    # file, dimensions, and the Q initial-set budget.
    check_webp_sizes(PRODUCTION_DIR, PUBLIC_MAX_BYTES, "public")
    print("production assets ok (basic size scan)")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--sample", action="store_true")
    parser.add_argument("--production", action="store_true")
    parser.add_argument("--sizes", action="store_true")
    args = parser.parse_args()

    if args.sample:
        check_sample()
        return
    if args.production:
        check_production()
        return
    parser.print_help()
    sys.exit(2)


if __name__ == "__main__":
    main()
