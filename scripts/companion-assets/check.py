#!/usr/bin/env python3
"""Check the companion asset sample and production packs.

The sample gate is intentionally strict about dimensions and alpha so a
half-processed image cannot pass visual review. Production checks are added
alongside the full asset pipeline in Tasks 2-3.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
SAMPLE_DIR = ROOT / "tmp" / "companion-sample"
PRODUCTION_DIR = ROOT / "apps" / "web" / "public" / "companion"
WEB_PUBLIC = ROOT / "apps" / "web" / "public"

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
    assets_path = ROOT / "apps/web/src/content/companion/assets.json"
    if not assets_path.is_file():
        fail("missing apps/web/src/content/companion/assets.json")
    try:
        assets = json.loads(assets_path.read_text())
    except json.JSONDecodeError as exc:
        fail(f"assets.json is invalid: {exc}")

    outfits = assets.get("outfits")
    if not isinstance(outfits, list) or len(outfits) != 5:
        fail("assets.json must contain five outfits")

    for outfit in outfits:
        outfit_id = outfit.get("id", "?")
        src = Path(str(outfit.get("src", "")))
        card = Path(str(outfit.get("card", "")))
        full_path = WEB_PUBLIC / str(src).lstrip("/")
        card_path = WEB_PUBLIC / str(card).lstrip("/")
        image = require_rgba(full_path, OUTFIT_SIZE)
        corners_transparent(image, f"{outfit_id} full")
        alpha_bbox_nonempty(image, f"{outfit_id} full")
        if full_path.stat().st_size > OUTFIT_MAX_BYTES:
            fail(f"{outfit_id} full is over {OUTFIT_MAX_BYTES} bytes")
        card_image = require_rgba(card_path, (384, 512))
        corners_transparent(card_image, f"{outfit_id} card")
        alpha_bbox_nonempty(card_image, f"{outfit_id} card")
        if card_path.stat().st_size > CARD_MAX_BYTES:
            fail(f"{outfit_id} card is over {CARD_MAX_BYTES} bytes")

    q = assets.get("q")
    if not isinstance(q, dict):
        fail("assets.json must contain the Q rig")
    if q.get("canvas") != {"width": 768, "height": 768}:
        fail("Q canvas must be 768x768")
    viewport = q.get("dockViewport", {})
    if viewport.get("width", 0) <= 0 or viewport.get("x", 0) + viewport.get("width", 0) > 768:
        fail("dockViewport is outside the Q canvas")

    layers = q.get("layers", {})
    named_layers: list[tuple[str, dict]] = [
        ("base", layers.get("base", {})),
        ("irisLeft", layers.get("irisLeft", {})),
        ("irisRight", layers.get("irisRight", {})),
        ("eyeOpen", layers.get("eyeOpen", {})),
        ("eyeClosed", layers.get("eyeClosed", {})),
    ]
    for group in ("eyes", "mouths", "blush", "fx"):
        named_layers.extend((f"{group}.{key}", value) for key, value in layers.get(group, {}).items())

    for name, ref in named_layers:
        src = WEB_PUBLIC / str(ref.get("src", "")).lstrip("/")
        image = require_rgba(src, (768, 768))
        if name != "mouths.neutral":
            has_transparency_and_content(image, name)
        if src.stat().st_size > PUBLIC_MAX_BYTES:
            fail(f"{name} is {src.stat().st_size} bytes, over {PUBLIC_MAX_BYTES}")

    initial_keys = ("base", "irisLeft", "irisRight", "eyeOpen", "mouths.neutral", "blush.soft")
    initial_sizes = {
        key: (WEB_PUBLIC / str(dict(named_layers)[key].get("src", "")).lstrip("/")).stat().st_size
        for key in initial_keys
    }
    initial = sum(initial_sizes.values())
    if initial > 250 * 1024:
        fail(f"initial Q set is {initial} bytes, over the 250KB hard cap")
    if initial > 180 * 1024:
        print(f"WARN: initial Q set is {initial / 1024:.1f}K, above the 180K target")

    check_webp_sizes(PRODUCTION_DIR, PUBLIC_MAX_BYTES, "public")
    print(f"production assets ok (5 outfits, Q rig, initial {initial / 1024:.1f}K)")


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
