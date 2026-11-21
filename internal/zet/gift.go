package zet

import (
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// Gift is a gift as it is configured: what [store.Gift] holds, and for an avatar item the item
// and its color, which become the receiving card's own survivor's resource when it is given.
type Gift struct {
	store.Gift
	// Avatar is the avatar item given, or nil. Gift.Item then holds only its kind and name.
	Avatar *game.AvatarItem
	// Color is the avatar item's color index, 0 to 99.
	Color int
}

// For returns the gift as a card whose chara_id is chara gets it. An avatar item that card's
// survivor cannot wear is left out, so the gift gives only its ZG, and For reports false.
func (g Gift) For(chara int64) (store.Gift, bool) {
	if g.Avatar == nil {
		return g.Gift, true
	}
	gift := g.Gift
	survivor, ok := game.SurvivorOf(chara)
	if !ok || g.Avatar.Resources[survivor] == 0 {
		gift.Item = store.Owned{}
		return gift, false
	}
	gift.Item.ItemID, gift.Item.Count = avatarID(g.Avatar.Resources[survivor], g.Color), 1
	return gift, true
}

// KindOf returns the item kind of an avatar item worn at site.
func KindOf(site game.AvatarSite) store.ItemKind {
	switch site {
	case game.SiteHead:
		return store.Head
	case game.SiteUpper:
		return store.Upper
	case game.SiteLower:
		return store.Lower
	default:
		return store.Accessory
	}
}

// avatarID is the item id the server gives an avatar item worn as resource in color. The game
// reads only an avatar item's resource id and color index, so the id carries both.
func avatarID(resource, color int) int64 { return int64(resource)*100 + int64(color) }

// avatarParts returns the resource id and color index an avatar item id carries.
func avatarParts(id int64) (resource, color int) { return int(id / 100), int(id % 100) }
