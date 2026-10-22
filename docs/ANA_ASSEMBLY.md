# ANA Building-Group Assembly (Jigsaw Method)

## Background

In the client's `graphic/*.ana` archives, large buildings and decorations are
stored as sequences of numbered groups (e.g. `10010`, `10011`, `10012`, …).
Each group holds one or more 256×256 tiles; there is no explicit placement
metadata that says how consecutive groups fit together into a whole building.

## Key insight

Consecutive groups are cut like jigsaw pieces: **the pixel width of one
group's solid straight edge equals the pixel width of a transparent slot in
the neighboring group**. Find the matching widths, and the pieces plug
together with no overlap and no gap.

Verified example (rock + fish-head totem):

- Totem group: top rows are a solid bar, 337 px wide, ~20 px tall.
- Rock group: contains a transparent horizontal slot, 337 px wide,
  at offset (153, 275).
- Placing the totem at (153, 275) seats its top bar exactly into the slot.
  The two sprites join seamlessly.

## Algorithm

Given two groups A (peg) and B (slot), both as RGBA images:

1. **Measure the peg.** Scan the peg's edge rows (top/bottom) for the
   longest run of fully opaque pixels. Record `(width, height)` of the
   solid rectangular bar.
2. **Find the slot.** Scan the slot image row by row for the longest run
   of fully transparent pixels. A run whose width matches the peg's bar
   width (±2 px tolerance) over several consecutive rows is a candidate
   slot. Record its `(x, y, width, height)`.
3. **Assemble.** Composite the peg at the slot's `(x, y)`, drawing the slot
   image on top. The peg's bar fills the transparent region; the rest of
   the peg extends outward with no pixel overlap.

Notes and limits:

- The match is on **width only**; heights may differ (the bar can be
  taller than the slot — only the overlapping region matters).
- If several slots share the same width, disambiguate visually; the
  method narrows candidates, it does not always pick a unique one.
- Works for axis-aligned rectangular cuts. Diagonal or irregular cuts
  need manual placement.
- Always verify the composite visually: a correct joint shows no black
  seam and no doubled edges.

## Tool

`tools/ana_slot_find.py` implements step 2: given an RGBA PNG and a target
width, it prints candidate `(x, y, w, h)` slots. Usage:

```sh
python3 tools/ana_slot_find.py rock.png --width 337
# candidate slot: x=153 y=275 w=337 h=20
```
