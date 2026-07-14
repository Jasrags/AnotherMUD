package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ShowRoomTipsForTest exposes the unexported room-tip seam to the external test
// package (ui-rendering-help §12).
func (c *Context) ShowRoomTipsForTest(ctx context.Context, r *world.Room, lvl light.Level) {
	c.maybeShowRoomTips(ctx, r, lvl)
}
