package entities

import "github.com/Jasrags/AnotherMUD/internal/keyword"

// Compile-time assertion that ItemInstance satisfies the keyword
// resolver's Named contract. M5.5 (equip) and M5.6 (inventory ops)
// rely on this; a missing Name() or Keywords() method should break
// the build here rather than at a distant call site.
var _ keyword.Named = (*ItemInstance)(nil)
