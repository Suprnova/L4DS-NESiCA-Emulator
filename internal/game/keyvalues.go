package game

import "strings"

// keyValueBlocks reads the innermost blocks of a KeyValues file, by name: each "name" { ... }
// whose body holds only "key" "value" pairs. Comments are skipped.
func keyValueBlocks(text string) map[string]map[string]string {
	var tokens []string
	for _, match := range token.FindAllString(text, -1) {
		if !strings.HasPrefix(match, "//") {
			tokens = append(tokens, match)
		}
	}

	blocks := map[string]map[string]string{}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i+1] != "{" || tokens[i] == "{" || tokens[i] == "}" {
			continue
		}
		block := map[string]string{}
		j := i + 2
		for ; j+1 < len(tokens) && tokens[j] != "}" && tokens[j] != "{" && tokens[j+1] != "{"; j += 2 {
			block[unquote(tokens[j])] = unquote(tokens[j+1])
		}
		if j < len(tokens) && tokens[j] == "}" {
			blocks[unquote(tokens[i])] = block
		}
	}
	return blocks
}

func unquote(token string) string { return strings.Trim(token, `"`) }
