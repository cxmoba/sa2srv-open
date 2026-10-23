# Map Object Placement (OBJ1)

## Block structure

Map files contain an `OBJ1` block listing placed objects. Layout:

```
'OBJ1' u32 size | u16 version | u32 count | count * (5 x u16)
```

Each 10-byte record:

| Offset | Type | Field    | Meaning                              |
|--------|------|----------|--------------------------------------|
| 0      | u16  | X        | subcell X coordinate (anchor)        |
| 2      | u16  | Y        | subcell Y coordinate (anchor)        |
| 4      | u16  | tid      | object id (references sprite group) |
| 6      | u16  | variant  | client-translated before use        |
| 8      | u16  | status   | state selector                      |

`size` must equal `14 + count * 10`.

## Isometric projection

Subcell `(X, Y)` maps to screen pixels via:

```
sx = 24 * (x + y)
sy = 16 * (y - x)
```

Place the object's **origin** (as defined by its sprite bounding box)
at `(sx, sy)`. The sprite's bbox extends relative to that point.

## Assembling multi-part objects

Buildings split across several sprite groups are placed as separate
records with nearby `(X, Y)` anchors. To reassemble:

1. Project each record's `(X, Y)` with the formula above.
2. Draw each sprite with its origin at the projected point.
3. Draw order follows record order (later records on top).

No per-group relative offsets are stored anywhere — the map anchors
are the single source of truth for placement. Pixel-level "slot"
matching between groups is a consequence of correct anchor placement,
not the mechanism.
