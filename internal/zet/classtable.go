package zet

import "net/http"

// classTable answers the startup class table with the versus classes only: the header, no
// co-op classes, no co-op ratings, the versus classes, and no versus ratings. Each versus class
// is its id (counted from 1), name, starting class points, bonus class points, and its five
// rating score thresholds. With the co-op classes and both rating lists empty, the game keeps
// using its own tables for them, as it does without a reply; its versus round result, though,
// reads the versus classes from this reply alone.
func (s *Server) classTable(w http.ResponseWriter, _ *http.Request) {
	lines := []any{1, 1, 0, 0, 0, len(s.classes)}
	for i, class := range s.classes {
		lines = append(lines, i+1, class.Name, class.BaseCP, class.BonusCP, len(class.Scores))
		for _, score := range class.Scores {
			lines = append(lines, score)
		}
	}
	lines = append(lines, 0)
	w.Write([]byte(lineBody(lines)))
}
