#!/usr/bin/env python3
"""Find transparent slot candidates in an RGBA sprite.

A "slot" is a horizontal run of fully transparent pixels whose width
matches a peg piece's solid edge width (see docs/ANA_ASSEMBLY.md).

Usage:
    python3 ana_slot_find.py image.png --width 337 [--tol 2] [--min-h 10]
"""
import argparse
import sys

from PIL import Image
import numpy as np


def find_slots(path, width, tol=2, min_h=10):
    img = Image.open(path).convert("RGBA")
    alpha = np.array(img)[:, :, 3] > 0  # True = opaque
    h, w = alpha.shape

    # Per row: longest transparent run and its start x
    rows = []
    for y in range(h):
        row = ~alpha[y]
        best_len, best_x, cur, cur_x = 0, 0, 0, 0
        for x in range(w):
            if row[x]:
                if cur == 0:
                    cur_x = x
                cur += 1
                if cur > best_len:
                    best_len, best_x = cur, cur_x
            else:
                cur = 0
        rows.append((best_len, best_x))

    # Group consecutive rows whose run width matches the target
    cands = []
    y0 = None
    for y, (ln, x) in enumerate(rows):
        ok = abs(ln - width) <= tol
        if ok and y0 is None:
            y0, x0 = y, x
        elif not ok and y0 is not None:
            if y - y0 >= min_h:
                cands.append((x0, y0, ln, y - y0))
            y0 = None
    if y0 is not None and h - y0 >= min_h:
        cands.append((x0, y0, rows[h - 1][0], h - y0))
    return cands


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("image", help="RGBA PNG to scan")
    ap.add_argument("--width", type=int, required=True,
                    help="peg bar width in px to match")
    ap.add_argument("--tol", type=int, default=2,
                    help="width tolerance in px")
    ap.add_argument("--min-h", type=int, default=10,
                    help="minimum slot height in rows")
    args = ap.parse_args()

    cands = find_slots(args.image, args.width, args.tol, args.min_h)
    if not cands:
        print("no slot candidates found", file=sys.stderr)
        return 1
    for x, y, w, hh in cands:
        print(f"candidate slot: x={x} y={y} w={w} h={hh}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
