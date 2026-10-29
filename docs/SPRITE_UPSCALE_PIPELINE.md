# Sprite AI Upscaling Pipeline (2D HD Remaster)

Goal: low-res client sprites → HD-2D-ready pixel sprites, in batch.

## Flow

```
source sprite (single facing, front preferred)
  → 1. HD master (generative redraw)
  → 2. 8-direction statics (master anchors appearance, original anchors pose)
  → 3. 8-direction action mp4s (one clip per direction)
  → 4. frame extraction (ffmpeg, N frames per clip)
  → 5. unified pixelation (deterministic, same params for all frames)
  → into game
```

## Step notes

### 1. HD master
- Input: 1 front-facing source sprite (4x nearest-neighbor upscale before feeding in)
- Output: HD smooth illustration-style master
- Constraints: lock composition/pose/palette, add detail only, no reinterpretation
- Observed: good fidelity on creatures; human faces get "beautified" by the
  model and need extra style anchoring

### 2. 8-direction statics
- Appearance reference = HD master, pose reference = original small sprite
- **Symmetry check**: for symmetric characters, generate only 5 independent
  facings (up, down, upper-right, right, lower-right) and mirror the left
  half; asymmetric characters need all 8
- Mirroring requires: symmetric design + flat lighting, else visible
  artifacts (asymmetric details flip sides, lighting inverts)
- Observed: appearance anchoring works; poses drift when the pose
  reference is too small/blurry

### 3. Action mp4s
- Input: HD facing images (smooth version — do NOT pixelate first,
  the model needs the detail)
- Looping in-place actions (walk/idle), ~10 s per clip
- Observed: risk of gait distortion on humanoids ("marching in place");
  pending validation on creature sprites

### 4. Frame extraction
- ffmpeg extract by time/count, assemble contact sheets for review

### 5. Unified pixelation
- Deterministic op (ffmpeg scale-down + nearest-neighbor upscale), done LAST
- Guarantees identical grain across 8 directions x N frames; per-frame
  "pixel-art style" generation never aligns
- Density tunable (1/10 reads as HD-2D, 1/6 stays smooth)

## Validated findings
- waifu2x-style upscalers blur this art style; rejected
- Generative redraw locks composition/angle/palette but invents details
- Pixelation must be the final step (deterministic) to keep all
  directions/frames consistent

## Open items
- [ ] Restrictive prompt template (pose lock + style lock + no reinterpretation)
- [ ] Action-mp4 quality validation on creature sprites
- [ ] Batch consistency acceptance criteria (drift tolerance)
