package game

// Stage is a campaign the game can play, with its chapters in order.
type Stage struct {
	ID       int
	Name     string
	Chapters []Chapter
}

// Chapter is one map of a stage.
type Chapter struct {
	ID   int
	Name string
}
