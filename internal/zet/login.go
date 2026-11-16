package zet

import (
	"net/http"
	"strconv"
	"strings"
)

// Lines of the login reply that carry no data: the count of each empty group (titles, dailys,
// thrus, eventprizes, comps).
const emptyGroups = 5

// defaultConfig lists the config_values of the login reply: master volume, bgm volume,
// mouse horizontal and vertical reverse, mouse sensitivity and acceleration, key map, then
// the controller, mouse button and radio bindings.
var defaultConfig = []int{
	5, 5, 0, 0, 3, 0, 1,
	0, 0, 0, 0,
	0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
}

// login answers with a success header, no bonuses and the default player settings.
func login(w http.ResponseWriter, _ *http.Request) {
	lines := []string{"1", "1", "0"}
	for range emptyGroups {
		lines = append(lines, "0")
	}
	for _, value := range defaultConfig {
		lines = append(lines, strconv.Itoa(value))
	}
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}
