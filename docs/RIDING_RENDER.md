# Mount / Riding Render Mechanism

## Summary

When a character rides a pet, the client does **not** use a pre-combined
sprite. It renders two sprites at runtime, stacked at the same position:

1. The pet sprite (full body, all directions/frames as usual).
2. A seated rider sprite drawn on top, at the same anchor point.

## Rider sitting frames

Each character has a dedicated set of sitting frames: objects
**1032–1039** (one per facing direction, 8 directions). By convention, the
same object number in every character file holds that character's sitting
pose — the number is a per-character agreement, not a shared image.

The sitting frame's bounding box already encodes the seat offset (e.g.
bbox y in [-20, +38], origin at the seat point), so no extra offset table
is needed: draw the rider at the pet's position and the art lines up.

## Client behavior (observed)

- The ride-on handler only writes a mount-state field on the character
  object; it does not swap or rebuild any sprite.
- The actual drawing goes through the central sprite renderer, which
  draws pet first, rider second, at the same coordinates.
- Dismount clears the state field; nothing else changes.

## Implication for reimplementation

To reproduce riding: per character, prepare one sitting animation set
(8 directions); at runtime, draw it over the pet sprite at the pet's
anchor. No combined artwork is required.
