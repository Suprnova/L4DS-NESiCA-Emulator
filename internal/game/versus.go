package game

import (
	"errors"
	"fmt"
	"io"
	"strconv"
)

// VersusClass is a versus class: its name, the class points at which it starts, the bonus class
// points it gives, and the five score thresholds of a round's rating, best first.
type VersusClass struct {
	Name            string
	BaseCP, BonusCP int
	Scores          [5]int
}

// ParseVersusClasses reads the game's VersusClassDataTable.txt: one "VersusClassData_N" block per
// class, N counting from 0, with "pName", "baseCP", "bonusCP" and "aScores[0]" to "aScores[4]".
func ParseVersusClasses(r io.Reader) ([]VersusClass, error) {
	text, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	blocks := keyValueBlocks(string(text))
	var classes []VersusClass
	for n := 0; ; n++ {
		block, found := blocks[fmt.Sprintf("VersusClassData_%d", n)]
		if !found {
			break
		}
		class := VersusClass{Name: block["pName"]}
		numbers := map[string]*int{"baseCP": &class.BaseCP, "bonusCP": &class.BonusCP}
		for i := range class.Scores {
			numbers[fmt.Sprintf("aScores[%d]", i)] = &class.Scores[i]
		}
		for key, value := range numbers {
			if *value, err = strconv.Atoi(block[key]); err != nil {
				return nil, fmt.Errorf("versus class %d %s: %w", n, key, err)
			}
		}
		classes = append(classes, class)
	}
	if len(classes) == 0 {
		return nil, errors.New("game: versus class table has no classes")
	}
	return classes, nil
}
