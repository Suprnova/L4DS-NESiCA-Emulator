package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// grant queues a gift for the player registered to a card, delivered at the card's next login
// in the reward pop-up's compensation pages. The server may be running.
func grant(args []string) error {
	flags := flag.NewFlagSet("l4ds-server grant", flag.ContinueOnError)
	database := flags.String("db", "l4ds.db", "SQLite database holding the players; it must exist")
	card := flags.String("card", "", "16-digit number of the card whose player gets the gift")
	name := flags.String("name", "", "the gift's name, shown in the login's reward pop-up")
	zet := flags.Int64("zet", 0, "ZG given")
	category := flags.String("item", "", "category of the item given: weapon, skill_item, svp, title, upper, lower, head, or accessory; left out, no item")
	itemID := flags.Int64("item-id", 0, "the weapon's or skill item's database id in the game's catalogue, or the title's number")
	unit := flags.String("unit", "", "an avatar item's setting name in the game's avatar scripts, less any survivor suffix")
	color := flags.Int("color", 0, "an avatar item's color index, 0 to 99")
	itemName := flags.String("item-name", "", "the item's name, shown in the pop-up and on the card once set")
	count := flags.Int64("count", 1, "how many of the item, or the seconds of svp")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("grant: unexpected argument %q", flags.Arg(0))
	}

	var item *config.GrantItem
	if *category != "" {
		item = &config.GrantItem{Category: *category, ID: *itemID, Unit: *unit, Color: *color, Name: *itemName, Count: *count}
	}
	gift, err := config.NewGift(*name, *zet, item)
	if err != nil {
		return fmt.Errorf("grant: %w", err)
	}
	if _, err := os.Stat(*database); err != nil {
		return fmt.Errorf("grant: no database at %s", *database)
	}

	s, err := store.Open(*database)
	if err != nil {
		return err
	}
	defer s.Close()
	player, err := s.PlayerByCard(*card)
	if errors.Is(err, store.ErrNotRegistered) {
		return fmt.Errorf("grant: card %q is not registered", *card)
	}
	if err != nil {
		return err
	}
	given, fits := gift.For(player.CharaID)
	if !fits {
		return fmt.Errorf("grant: %s's survivor cannot wear %s", player.Name, *unit)
	}
	id, err := s.Queue(player.ID, time.Now(), given)
	if err != nil {
		return err
	}
	fmt.Printf("queued gift %d for %s (player %d); it is delivered at the card's next login\n", id, player.Name, player.ID)
	return nil
}
