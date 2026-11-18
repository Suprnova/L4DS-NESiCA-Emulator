package game

// VersusClass is a versus class: its name, the class points at which it starts, the bonus class
// points it gives, and the five score thresholds of a round's rating, best first.
type VersusClass struct {
	Name            string
	BaseCP, BonusCP int
	Scores          [5]int
}
