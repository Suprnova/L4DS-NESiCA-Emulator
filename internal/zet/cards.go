package zet

import "fmt"

// Cards is the card registry. It currently knows no players, so every card is unregistered.
type Cards struct{}

// Select answers a card lookup with a success result and no player id.
func (Cards) Select(cardID string) string {
	return fmt.Sprintf("1\n1\n%s\n0\n", cardID)
}
