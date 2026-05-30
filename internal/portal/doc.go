// Package portal is the M15.2 temporary keyword exit substrate:
// runtime keyword exits with a bounded lifetime tied to the area
// tick clock.
//
// Spec: docs/specs/world-rooms-movement.md §5.6.
//
// The Service owns portal records (id, source room, target room,
// keyword, expiry tick, optional paired-partner id) and delegates
// the on-the-room registration to world.World via AddKeywordExit /
// RemoveKeywordExit. On every area.tick event it sweeps expired
// portals and emits portal.closed.
//
// Per PD-5 (locked) portals creatable via BOTH content YAML
// (declared in area files at boot) AND an admin verb. The Service
// surface is the same for both paths — the difference is the
// caller (pack loader at boot vs. command verb at runtime). v1
// ships the Service + content YAML; the admin verb lands when the
// role-tag system reaches production usability.
package portal
