package main

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// commlinkOnboarding implements session.CommlinkOnboarding: it reads the pack's
// first-entry fixer message off the configured NPC template and detects a carried
// commlink by tag. The engine stays pack-neutral — the message text lives on the
// fixer NPC's `commlink_welcome` property, so no world vocabulary leaks into code.
type commlinkOnboarding struct {
	fixer string // qualified fixer mob template id (ANOTHERMUD_COMMLINK_FIXER)
	mobs  *mob.Templates
	items *item.Templates
	store *entities.Store
}

const (
	commlinkTag         = "commlink"         // the item tag that marks a commlink
	commlinkWelcomeProp = "commlink_welcome" // the fixer NPC property holding the message
)

// Welcome reads the configured fixer's onboarding message. Missing NPC / property
// / empty text all report "not configured" so the caller silently skips.
func (c *commlinkOnboarding) Welcome() (string, bool) {
	if c == nil || c.mobs == nil {
		return "", false
	}
	t, err := c.mobs.Get(mob.TemplateID(c.fixer))
	if err != nil {
		return "", false
	}
	msg, _ := t.Properties[commlinkWelcomeProp].(string)
	msg = strings.TrimSpace(msg)
	return msg, msg != ""
}

// CarriesCommlink reports whether any carried entity is a commlink-tagged item.
func (c *commlinkOnboarding) CarriesCommlink(inventory []entities.EntityID) bool {
	if c == nil || c.store == nil || c.items == nil {
		return false
	}
	for _, id := range inventory {
		ent, ok := c.store.GetByID(id)
		if !ok {
			continue
		}
		inst, ok := ent.(*entities.ItemInstance)
		if !ok {
			continue
		}
		tpl, err := c.items.Get(inst.TemplateID())
		if err != nil {
			continue
		}
		for _, tag := range tpl.Tags {
			if tag == commlinkTag {
				return true
			}
		}
	}
	return false
}
