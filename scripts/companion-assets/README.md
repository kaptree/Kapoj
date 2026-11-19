# Companion asset pipeline

Dev-only tools for turning the source art in `Koj伙伴素材/` into web-ready
companion assets. The source files are never modified.

## Setup

Use a throwaway virtual environment so the app stays free of image-processing
dependencies:

```bash
python3 -m venv /tmp/companion-assets-venv
/tmp/companion-assets-venv/bin/pip install \
  'rembg[cpu]' pillow numpy scipy scikit-image opencv-python-headless
```

`rembg` downloads the `isnet-anime` model on first use.

## Commands

```bash
# Review sample: transparent Q, iris layers, >< happy layer, one outfit
/tmp/companion-assets-venv/bin/python scripts/companion-assets/process.py sample
python3 scripts/companion-assets/check.py --sample

# Full packs (Tasks 2-3)
/tmp/companion-assets-venv/bin/python scripts/companion-assets/process.py outfits
/tmp/companion-assets-venv/bin/python scripts/companion-assets/process.py q
/tmp/companion-assets-venv/bin/python scripts/companion-assets/process.py mascot
python3 scripts/companion-assets/check.py --production
```

Samples go to `tmp/companion-sample/` and stay untracked. Production images go
to `apps/web/public/companion/`; the `mascot` command refreshes the static
`public/mascot/{chibi,bust,full}` art used by the auth pages and home hero from
the neutral Q rig and `outfit-1`.

## Compression policy

- Q layers: lossless WebP first. If a large layer pushes the initial dock set
  over budget, re-encode at `quality=95, method=6, alpha_quality=100` and record
  the SSIM comparison.
- Outfits: lossless first; if a single file is over 300KB, use q95 visual
  lossless (SSIM ≥ 0.95 with a visual check). Cards use q90 and stay ≤80KB.
- `public/` never receives the multi-megabyte source PNGs.
- Initial Q set (`base`, two iris layers, `eye-open`, `mouth-neutral`,
  `blush-soft`): target ≤180KB, hard cap 250KB.
- Static mascot poses (`public/mascot`): chibi ≤120KB (512×512), bust
  ≤220KB (640×747), full ≤320KB (1000×1333); q95 with SSIM ≥0.95 when
  lossless is over budget.

## Q layout

All Q layers share a 768×768 canvas. `base` keeps the face and lash; the two
iris layers are cut from the real iris (smooth ellipse, no white sclera rim).
`eye-happy` is a full `><` emoticon eye patch (skin/blush covers + bold
strokes); the neutral/idle state still uses the iris layers for gaze tracking.
The dock crops the same stack with `dockViewport`; the panel/profile uses the
full canvas.
