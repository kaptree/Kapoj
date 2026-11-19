#!/usr/bin/env python3
"""Build the companion Q avatar layers and outfit portraits.

The script intentionally keeps the source files untouched. It writes review
samples to tmp/companion-sample and production WebP files to
apps/web/public/companion in the later asset tasks.

Run it with the companion asset venv (see README.md), because rembg, numpy,
scipy and opencv are dev-only tools and are not app dependencies.
"""

from __future__ import annotations

import argparse
import io
import json
import math
import sys
from pathlib import Path

import cv2
import numpy as np
from PIL import Image, ImageDraw, ImageFilter
from scipy import ndimage
from skimage.metrics import structural_similarity

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / "Koj伙伴素材"
SAMPLE_DIR = ROOT / "tmp" / "companion-sample"
PUBLIC_DIR = ROOT / "apps" / "web" / "public" / "companion"

Q_SOURCE = SOURCE / "Q版形象.png"
OUTFIT_SOURCES = [SOURCE / f"{i}.png" for i in range(1, 6)]

Q_MASTER = 1536
Q_OUT = 768
OUTFIT_SIZE = (768, 1024)
CARD_SIZE = (384, 512)
OUTFIT_MAX_BYTES = 300 * 1024
CARD_MAX_BYTES = 80 * 1024

OUTFIT_META = (
    ("outfit-1", "红莓礼帽", 0),
    ("outfit-2", "沙棕旅装", 1),
    ("outfit-3", "冰蓝礼装", 2),
    ("outfit-4", "白夜大衣", 4),
    ("outfit-5", "深海制服", 6),
)
ASSETS_JSON = ROOT / "apps/web/src/content/companion/assets.json"

# Eye centers/radii are measured from the blue iris components of the source.
# They are kept here so every run aligns the layers the same way.
EYES = (
    {"cx": 513, "cy": 706, "rx": 62, "ry": 69},
    {"cx": 835, "cy": 752, "rx": 77, "ry": 76},
)
SOCKETS = (
    {"cx": 513, "cy": 706, "rx": 80, "ry": 84},
    {"cx": 835, "cy": 752, "rx": 95, "ry": 91},
)
EYE_BOXES = (
    (420, 600, 615, 815),
    (720, 640, 965, 885),
)
MOUTH = {"cx": 614, "cy": 858, "rx": 58, "ry": 38}


def _rembg_session():
    try:
        from rembg import new_session
    except ImportError as exc:  # pragma: no cover - exercised by the CLI
        raise SystemExit(
            "rembg is required for matting. Create the dev venv described in "
            "scripts/companion-assets/README.md."
        ) from exc
    return new_session("isnet-anime")


def matte(image: Image.Image, session) -> Image.Image:
    from rembg import remove

    rgba = remove(image, session=session, alpha_matting=False).convert("RGBA")
    alpha = np.array(rgba.getchannel("A"))
    alpha[alpha < 8] = 0
    rgba.putalpha(Image.fromarray(alpha))
    return rgba


def _ellipse_mask(shape: tuple[int, int], cx: float, cy: float, rx: float, ry: float) -> np.ndarray:
    yy, xx = np.ogrid[: shape[0], : shape[1]]
    return ((xx - cx) / rx) ** 2 + ((yy - cy) / ry) ** 2 <= 1.0


def _iris_mask(rgb: np.ndarray, eye: dict, box: tuple[int, int, int, int]) -> np.ndarray:
    """A smooth ellipse fitted to the blue iris, minus the upper lash. A
    smooth edge is more important than including the last pixel of the blue
    rim; a jagged mask reads as a white scratch when the iris moves."""
    h, w = rgb.shape[:2]
    iris = _ellipse_mask((h, w), eye["cx"], eye["cy"], eye["rx"], eye["ry"])
    dark = rgb.max(axis=2) < 105
    yy = np.indices((h, w))[0]
    lash = iris & dark & (yy < eye["cy"] - eye["ry"] * 0.12)
    return iris & ~lash

def _nearest_sclera_fill(rgb: np.ndarray, eye: dict, iris: np.ndarray) -> np.ndarray:
    """Fill the iris disc with the nearest white/blue sclera pixel, preserving
    the original sclera shading instead of painting a flat white circle."""
    h, w = rgb.shape[:2]
    r, g, b = rgb[:, :, 0], rgb[:, :, 1], rgb[:, :, 2]
    eye_area = _ellipse_mask((h, w), eye["cx"], eye["cy"], eye["rx"] + 10, eye["ry"] + 10)
    bright = rgb.min(axis=2) > 120
    sclera = eye_area & ~iris & bright & (b >= r - 6)
    if not sclera.any():
        sclera = eye_area & ~iris & bright
    if not sclera.any():
        fill = np.empty_like(rgb)
        fill[:] = (246, 247, 252)
        return fill
    _, indices = ndimage.distance_transform_edt(~sclera, return_indices=True)
    return rgb[indices[0], indices[1]]


def _add_eye_shadow(base: Image.Image, eye: dict, iris: np.ndarray) -> Image.Image:
    """A soft upper-eyelid shadow on the sclera gives the eye depth when the
    iris moves."""
    arr = np.array(base)
    h, w = arr.shape[:2]
    eye_area = _ellipse_mask((h, w), eye["cx"], eye["cy"], eye["rx"] + 8, eye["ry"] + 8)
    sclera = eye_area & ~iris & (arr[:, :, 3] > 0)
    yy = np.indices((h, w))[0]
    t = np.clip((eye["cy"] + eye["ry"] * 0.4 - yy) / (eye["ry"] * 1.2), 0, 1)
    alpha = (t * 55 * sclera).astype(np.uint8)
    shadow = np.zeros((h, w, 4), dtype=np.uint8)
    shadow[:, :, 0] = 45
    shadow[:, :, 1] = 55
    shadow[:, :, 2] = 80
    shadow[:, :, 3] = alpha
    out = base.copy()
    out.alpha_composite(Image.fromarray(shadow, "RGBA"))
    return out


def _feather(mask: np.ndarray, radius: int = 3) -> Image.Image:
    alpha = Image.fromarray((mask * 255).astype(np.uint8), "L")
    return alpha.filter(ImageFilter.GaussianBlur(radius))


def _tapered_polygon(points: list[tuple[float, float]], widths: list[float]) -> list[tuple[float, float]]:
    left: list[tuple[float, float]] = []
    right: list[tuple[float, float]] = []
    for i, (x, y) in enumerate(points):
        if i == 0:
            dx = points[1][0] - x
            dy = points[1][1] - y
        elif i == len(points) - 1:
            dx = x - points[i - 1][0]
            dy = y - points[i - 1][1]
        else:
            dx = points[i + 1][0] - points[i - 1][0]
            dy = points[i + 1][1] - points[i - 1][1]
        norm = math.hypot(dx, dy) or 1.0
        nx, ny = -dy / norm, dx / norm
        width = max(0.1, widths[i]) / 2
        left.append((x + nx * width, y + ny * width))
        right.append((x - nx * width, y - ny * width))
    return left + right[::-1]


def _draw_closed_eye(draw: ImageDraw.ImageDraw, eye: dict, scale: int, outer_sign: float) -> None:
    cx, cy = eye["cx"] * scale, eye["cy"] * scale
    rx, ry = eye["rx"] * scale, eye["ry"] * scale
    points: list[tuple[float, float]] = []
    widths: list[float] = []
    for i in range(31):
        t = i / 30
        x = cx - rx * 0.56 + 1.12 * rx * t
        y = cy + ry * 0.02 - math.sin(math.pi * t) * ry * 0.34
        points.append((x, y))
        widths.append((1.4 + 6.0 * math.sin(math.pi * t)) * scale)
    draw.polygon(_tapered_polygon(points, widths), fill=(34, 27, 34, 255))

    # Outer lash flick, tapered at the tip.
    ox = cx + outer_sign * rx * 0.56
    oy = cy + ry * 0.02
    tipx = cx + outer_sign * rx * 0.80
    tipy = cy - ry * 0.30
    lash = [(ox, oy), ((ox + tipx) / 2, (oy + tipy) / 2 - ry * 0.06), (tipx, tipy)]
    draw.polygon(_tapered_polygon(lash, [5.5 * scale, 3.2 * scale, 0.7 * scale]), fill=(34, 27, 34, 255))

    # A very soft upper crease.
    crease: list[tuple[float, float]] = []
    crease_w: list[float] = []
    for i in range(21):
        t = i / 20
        x = cx - rx * 0.45 + 0.9 * rx * t
        y = cy - ry * 0.14 - math.sin(math.pi * t) * ry * 0.08
        crease.append((x, y))
        crease_w.append(2.0 * scale * (0.4 + 0.6 * math.sin(math.pi * t)))
    draw.polygon(_tapered_polygon(crease, crease_w), fill=(205, 150, 145, 95))


def _blush_overlay(size: tuple[int, int], eyes: tuple[dict, ...]) -> Image.Image:
    w, h = size
    overlay = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    for eye in eyes:
        cx, cy, rx, ry = eye["cx"], eye["cy"], eye["rx"], eye["ry"]
        draw.ellipse(
            (cx - rx * 0.78, cy + ry * 0.28, cx + rx * 0.78, cy + ry * 0.92),
            fill=(255, 145, 155, 42),
        )
    return overlay.filter(ImageFilter.GaussianBlur(18))


def _inpaint_patch(rgb: np.ndarray, mask: np.ndarray, radius: int) -> Image.Image:
    mask8 = (mask.astype(np.uint8) * 255)
    inpainted = cv2.inpaint(rgb, mask8, radius, cv2.INPAINT_TELEA)
    patch = Image.fromarray(inpainted, "RGB").convert("RGBA")
    patch.putalpha(_feather(mask, 3))
    return patch


def build_q_layers(source: Image.Image, session) -> dict[str, Image.Image]:
    rgba = matte(source, session)
    rgb = np.array(rgba.convert("RGB")).astype(np.uint8)
    h, w = rgb.shape[:2]
    base = rgba.copy()
    base_pixels = np.array(base)

    iris_masks: list[np.ndarray] = []
    iris_layers: list[Image.Image] = []
    for eye, box in zip(EYES, EYE_BOXES):
        iris = _iris_mask(rgb, eye, box)
        iris_masks.append(iris)
        fill = _nearest_sclera_fill(rgb, eye, iris)
        iris_layer = ndimage.binary_erosion(iris, iterations=1)
        base_pixels[iris, 0] = fill[iris, 0]
        base_pixels[iris, 1] = fill[iris, 1]
        base_pixels[iris, 2] = fill[iris, 2]
        base_pixels[iris, 3] = 255

        layer = np.zeros((h, w, 4), dtype=np.uint8)
        layer[iris_layer, 0] = rgb[iris_layer, 0]
        layer[iris_layer, 1] = rgb[iris_layer, 1]
        layer[iris_layer, 2] = rgb[iris_layer, 2]
        layer[iris_layer, 3] = 255
        alpha = Image.fromarray(layer[:, :, 3], "L").filter(ImageFilter.GaussianBlur(0.7))
        layer[:, :, 3] = np.array(alpha)
        iris_layers.append(Image.fromarray(layer, "RGBA"))

    base = Image.fromarray(base_pixels, "RGBA")
    for eye, iris in zip(EYES, iris_masks):
        base = _add_eye_shadow(base, eye, iris)

    eye_open = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    for layer in iris_layers:
        eye_open.alpha_composite(layer)

    return {
        "base": base,
        "iris-left": iris_layers[0],
        "iris-right": iris_layers[1],
        "eye-open": eye_open,
        "eye-happy": build_happy_eyes(rgb),
        "mouth-smile": build_smile(rgb),
    }


def _skin_colour(rgb: np.ndarray, sockets: tuple[dict, ...]) -> tuple[int, int, int]:
    h, w = rgb.shape[:2]
    r, g, b = rgb[:, :, 0], rgb[:, :, 1], rgb[:, :, 2]
    yy = np.indices((h, w))[0]
    ring = np.zeros((h, w), dtype=bool)
    lower = np.zeros((h, w), dtype=bool)
    for socket in sockets:
        outer = _ellipse_mask((h, w), socket["cx"], socket["cy"], socket["rx"] + 22, socket["ry"] + 22)
        inner = _ellipse_mask((h, w), socket["cx"], socket["cy"], socket["rx"], socket["ry"])
        ring |= outer & ~inner
        lower |= outer & ~inner & (yy > socket["cy"])
    skin = (r > 190) & (g > 150) & (b > 140) & (r > b + 5)
    upper_pixels = rgb[skin & ring & ~lower]
    lower_pixels = rgb[skin & lower]
    if len(upper_pixels) == 0 or len(lower_pixels) == 0:
        pixels = rgb[ring]
        return tuple(int(v) for v in np.median(pixels, axis=0))
    upper_colour = np.median(upper_pixels, axis=0)
    lower_colour = np.median(lower_pixels, axis=0)
    # The forehead is lighter; the cheek carries the blush. Blend toward the
    # cheek so the closed-eye patch does not read as a pale sticker.
    colour = upper_colour * 0.42 + lower_colour * 0.58
    return tuple(int(v) for v in colour)

def build_happy_eyes(rgb: np.ndarray) -> Image.Image:
    """A stylized >< emoticon face.

    The eye sockets are covered with a soft blush-toned skin patch, then the
    two eyes are drawn as bold > < strokes. This reads as a cute emoticon at
    dock size and avoids the pale-patch/black-inpaint artifacts of a fully
    closed-eye reconstruction.
    """
    h, w = rgb.shape[:2]
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))

    halo = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    hd = ImageDraw.Draw(halo)
    for eye in EYES:
        cx, cy, rx, ry = eye["cx"], eye["cy"], eye["rx"], eye["ry"]
        hd.ellipse(
            (cx - rx * 1.95, cy - ry * 1.75, cx + rx * 1.95, cy + ry * 1.85),
            fill=(255, 150, 150, 50),
        )
    layer.alpha_composite(halo.filter(ImageFilter.GaussianBlur(28)))

    cover = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    cd = ImageDraw.Draw(cover)
    for eye in EYES:
        cx, cy, rx, ry = eye["cx"], eye["cy"], eye["rx"], eye["ry"]
        cd.ellipse(
            (cx - rx * 1.36, cy - ry * 1.30, cx + rx * 1.36, cy + ry * 1.30),
            fill=(252, 214, 208, 255),
        )
        cd.ellipse(
            (cx - rx * 0.95, cy - ry * 0.85, cx + rx * 0.95, cy + ry * 0.85),
            fill=(252, 232, 226, 255),
        )
    layer.alpha_composite(cover)

    colour = (36, 27, 32, 255)
    stroke = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    sd = ImageDraw.Draw(stroke)
    for index, eye in enumerate(EYES):
        cx, cy, rx, ry = eye["cx"], eye["cy"], eye["rx"], eye["ry"]
        sign = 1.0 if index == 0 else -1.0
        points: list[tuple[float, float]] = []
        widths: list[float] = []
        for i in range(21):
            t = i / 20
            if t <= 0.5:
                u = t / 0.5
                x = cx - sign * rx * 0.75 + sign * rx * 1.32 * u
                y = cy - ry * 0.50 + ry * 0.50 * u
            else:
                u = (t - 0.5) / 0.5
                x = cx + sign * rx * 0.57 - sign * rx * 1.32 * u
                y = cy + ry * 0.50 * u
            points.append((x, y))
            widths.append(4.4 + 15.0 * math.sin(math.pi * t))
        sd.polygon(_tapered_polygon(points, widths), fill=colour)
    layer.alpha_composite(stroke)
    return layer

def build_smile(rgb: np.ndarray) -> Image.Image:
    h, w = rgb.shape[:2]
    mask = _ellipse_mask((h, w), MOUTH["cx"], MOUTH["cy"], MOUTH["rx"], MOUTH["ry"])
    patch = _inpaint_patch(rgb, mask, 5)

    scale = 2
    overlay = Image.new("RGBA", (w * scale, h * scale), (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    cx, cy = MOUTH["cx"] * scale, MOUTH["cy"] * scale
    points: list[tuple[float, float]] = []
    widths: list[float] = []
    for i in range(25):
        t = i / 24
        points.append((cx - 30 * scale + 60 * scale * t, cy - 2 * scale + math.sin(math.pi * t) * 15 * scale))
        widths.append((1.2 + 4.0 * math.sin(math.pi * t)) * scale)
    draw.polygon(_tapered_polygon(points, widths), fill=(118, 45, 52, 255))
    overlay = overlay.resize((w, h), Image.Resampling.LANCZOS)
    patch.alpha_composite(overlay)
    patch.putalpha(_feather(mask, 6))
    return patch


def resize_layer(layer: Image.Image, size: int = Q_OUT) -> Image.Image:
    resized = layer.resize((size, size), Image.Resampling.LANCZOS)
    arr = np.array(resized)
    arr[arr[:, :, 3] < 8, 3] = 0
    return Image.fromarray(arr, "RGBA")


def build_outfit(source: Image.Image, session, width: int = OUTFIT_SIZE[0], height: int = OUTFIT_SIZE[1]) -> Image.Image:
    rgba = matte(source, session)
    alpha = np.array(rgba.getchannel("A"))
    alpha[alpha < 8] = 0
    bbox = Image.fromarray(alpha, "L").getbbox()
    if bbox is None:
        raise SystemExit("matte produced an empty image")
    x0, y0, x1, y1 = bbox
    pad = max(8, int(max(x1 - x0, y1 - y0) * 0.02))
    x0, y0 = max(0, x0 - pad), max(0, y0 - pad)
    x1, y1 = min(rgba.width, x1 + pad), min(rgba.height, y1 + pad)
    character = rgba.crop((x0, y0, x1, y1))

    # Fit the character to 92% of the 3:4 canvas and centre it. The source is
    # already cut at the thighs, so this keeps the visible "thigh-up" framing
    # while adding transparent side margins instead of cutting the body.
    target_w = int(width * 0.92)
    target_h = int(height * 0.92)
    scale = min(target_w / character.width, target_h / character.height)
    new_size = (max(1, round(character.width * scale)), max(1, round(character.height * scale)))
    character = character.resize(new_size, Image.Resampling.LANCZOS)
    canvas = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    canvas.alpha_composite(character, ((width - new_size[0]) // 2, (height - new_size[1]) // 2))
    return canvas


def _webp_bytes(image: Image.Image, **kwargs) -> bytes:
    buffer = io.BytesIO()
    image.save(buffer, "WEBP", method=6, **kwargs)
    return buffer.getvalue()


def _ssim(reference: Image.Image, candidate: Image.Image) -> float:
    reference_rgb = np.array(reference.convert("RGB"))
    candidate_rgb = np.array(candidate.convert("RGB"))
    return float(
        structural_similarity(reference_rgb, candidate_rgb, channel_axis=2, data_range=255)
    )


def save_budget_webp(
    image: Image.Image,
    path: Path,
    max_bytes: int,
    start_quality: int = 95,
    min_ssim: float = 0.995,
) -> tuple[int, str, float]:
    """Persist the smallest acceptable WebP.

    Lossless is always tried first. If it exceeds max_bytes, try increasing
    lossy qualities until both the byte budget and the SSIM floor are met.
    """
    path.parent.mkdir(parents=True, exist_ok=True)
    lossless = _webp_bytes(image, lossless=True)
    if len(lossless) <= max_bytes:
        path.write_bytes(lossless)
        return len(lossless), "lossless", 1.0

    for quality in range(start_quality, 100):
        data = _webp_bytes(image, quality=quality, alpha_quality=100)
        if len(data) > max_bytes:
            continue
        decoded = Image.open(io.BytesIO(data)).convert("RGBA")
        score = _ssim(image, decoded)
        if score >= min_ssim:
            path.write_bytes(data)
            return len(data), f"q{quality}", score

    # The budget is the hard requirement; keep the start quality and report
    # the score so the oversized/below-target case is visible in the table.
    data = _webp_bytes(image, quality=start_quality, alpha_quality=100)
    path.write_bytes(data)
    decoded = Image.open(io.BytesIO(data)).convert("RGBA")
    return len(data), f"q{start_quality}*", _ssim(image, decoded)


def run_outfits() -> None:
    outfit_dir = PUBLIC_DIR / "outfits"
    outfit_dir.mkdir(parents=True, exist_ok=True)
    session = _rembg_session()
    entries: list[dict[str, object]] = []
    print(f"{'id':10} {'full':>9} {'quality':>9} {'ssim':>7} {'card':>8}")
    for outfit_id, name, level in OUTFIT_META:
        index = int(outfit_id.split("-")[1])
        image = build_outfit(Image.open(OUTFIT_SOURCES[index - 1]).convert("RGB"), session)
        full_path = outfit_dir / f"{outfit_id}.webp"
        full_size, quality, score = save_budget_webp(
            image, full_path, OUTFIT_MAX_BYTES, start_quality=95, min_ssim=0.95
        )
        card = image.resize(CARD_SIZE, Image.Resampling.LANCZOS)
        card_path = outfit_dir / f"{outfit_id}-card.webp"
        card_size, card_quality, _ = save_budget_webp(
            card, card_path, CARD_MAX_BYTES, start_quality=90, min_ssim=0.95
        )
        entries.append(
            {
                "id": outfit_id,
                "name": name,
                "level": level,
                "src": f"/companion/outfits/{outfit_id}.webp",
                "card": f"/companion/outfits/{outfit_id}-card.webp",
                "width": OUTFIT_SIZE[0],
                "height": OUTFIT_SIZE[1],
            }
        )
        print(
            f"{outfit_id:10} {full_size / 1024:8.1f}K {quality:>9} {score:7.4f} "
            f"{card_size / 1024:7.1f}K ({card_quality})"
        )

    payload: dict[str, object] = {"version": 1, "outfits": entries}
    if ASSETS_JSON.exists():
        try:
            existing = json.loads(ASSETS_JSON.read_text())
            existing["version"] = 1
            existing["outfits"] = entries
            payload = existing
        except json.JSONDecodeError:
            pass
    ASSETS_JSON.parent.mkdir(parents=True, exist_ok=True)
    ASSETS_JSON.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n")
    print(f"manifest written to {ASSETS_JSON.relative_to(ROOT)}")


def save_png(image: Image.Image, path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    image.save(path, "PNG")


def save_webp(image: Image.Image, path: Path, lossless: bool = True) -> int:
    path.parent.mkdir(parents=True, exist_ok=True)
    if lossless:
        image.save(path, "WEBP", lossless=True, method=6)
    else:
        image.save(path, "WEBP", quality=95, method=6, alpha_quality=100)
    return path.stat().st_size


def run_sample() -> None:
    SAMPLE_DIR.mkdir(parents=True, exist_ok=True)
    session = _rembg_session()
    q_layers = build_q_layers(Image.open(Q_SOURCE).convert("RGB"), session)
    # Large layers use q95 visual-lossless to stay inside the initial budget;
    # small layers stay bitwise lossless.
    lossy_layers = {"base", "eye-open", "eye-happy"}
    for name, layer in q_layers.items():
        resized = resize_layer(layer)
        save_png(resized, SAMPLE_DIR / "q" / f"{name}.png")
        save_webp(resized, SAMPLE_DIR / "q" / f"{name}.webp", lossless=name not in lossy_layers)

    outfit = build_outfit(Image.open(OUTFIT_SOURCES[0]).convert("RGB"), session)
    save_png(outfit, SAMPLE_DIR / "outfits" / "outfit-1.png")
    save_webp(outfit, SAMPLE_DIR / "outfits" / "outfit-1.webp", lossless=False)
    print(f"sample written to {SAMPLE_DIR.relative_to(ROOT)}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("sample", "q", "outfits"))
    args = parser.parse_args()

    if args.command == "sample":
        run_sample()
        return
    if args.command == "q":
        raise SystemExit("full Q pack is generated in Task 3")
    if args.command == "outfits":
        run_outfits()
        return
    parser.print_help()
    sys.exit(2)


if __name__ == "__main__":
    main()
