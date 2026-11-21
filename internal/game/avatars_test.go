package game

import (
	"cmp"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestAvatarItems_MatchInstalledGame(t *testing.T) {
	dir := os.Getenv("L4DS_GAME_DIR")
	if dir == "" {
		t.Skip("L4DS_GAME_DIR is not set")
	}
	client, err := pe.Open(filepath.Join(dir, "left4dead2", "bin", "client.dll"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	parts := nameTable(t, client, 0x1081e4d0, 0xad)
	accessories := nameTable(t, client, 0x1081ea38, 0x50)
	heads := map[int]int{}
	for _, pair := range pairTable(t, client, 0x1081ecb8, 0x1f) {
		heads[pair[1]] = pair[0]
	}

	units := avatarUnits(t, filepath.Join(dir, "left4dead2", "scripts", "avataritem"))
	got := rebuildAvatarItems(t, units, parts, accessories, heads)

	if !reflect.DeepEqual(got, AvatarItems) {
		t.Errorf("AvatarItems differ from the installed game:\n%+v", got)
	}
}

func TestSurvivorOf_NamesTheSurvivorOfAChara(t *testing.T) {
	for chara, want := range map[int64]Survivor{1: Yusuke, 2: Sara, 3: Haruka, 4: Jordan} {
		if got, ok := SurvivorOf(chara); !ok || got != want {
			t.Errorf("SurvivorOf(%d) = %v, %v, want %v", chara, got, ok, want)
		}
	}
	for _, chara := range []int64{0, 5} {
		if _, ok := SurvivorOf(chara); ok {
			t.Errorf("SurvivorOf(%d) found a survivor", chara)
		}
	}
}

func TestAvatarAt_FindsTheItemASurvivorWears(t *testing.T) {
	want, _ := Avatar("santamoustache_face_eqp")

	got, ok := AvatarAt(SiteFace, Sara, want.Resources[Sara])

	if !ok || got.Name != want.Name {
		t.Fatalf("AvatarAt() = %+v, %v", got, ok)
	}
}

// avatarUnit is a unit of an avatar script, with its survivor blocks' replacements.
type avatarUnit struct {
	setting, site, only string
	// replacement is each survivor's "modelname", for those whose block replaces avatar_hair.
	replacement map[string]string
}

var survivorNames = []string{"yusuke", "sara", "haruka", "jordan"}

var onlyBits = map[string]int{"": 0xf, "yusuke": 1, "sara": 2, "haruka": 4, "jordan": 8, "male": 9, "female": 6}

var avatarSites = map[string]AvatarSite{
	"acce_head": SiteHead, "avatar_upper": SiteUpper, "avatar_lower": SiteLower,
	"acce_face": SiteFace, "acce_upper": SiteAccUpper, "acce_lower": SiteAccLower,
}

// rebuildAvatarItems builds the avatar items from the scripts' units and client.dll's tables, as
// AvatarItems holds them.
func rebuildAvatarItems(t *testing.T, units []avatarUnit, parts, accessories map[string]int, heads map[int]int) []AvatarItem {
	t.Helper()
	byName := map[string]*AvatarItem{}
	for _, unit := range units {
		site, known := avatarSites[unit.site]
		if !known {
			continue
		}
		table := accessories
		if strings.HasPrefix(unit.site, "avatar_") {
			table = parts
		}
		resource, found := table[unit.setting]
		if !found {
			t.Errorf("%s has no resource id", unit.setting)
			continue
		}
		name, survivors := unit.setting, onlyBits[unit.only]
		for i, suffix := range []string{"_y", "_s", "_h", "_j"} {
			if base, cut := strings.CutSuffix(unit.setting, suffix); cut {
				name, survivors = base, 1<<i
			}
		}
		item := byName[name]
		if item == nil {
			item = &AvatarItem{Name: name, Site: site}
			byName[name] = item
		}
		if site == SiteHead {
			if resource = heads[resource]; resource == 0 {
				t.Errorf("%s has no head resource id", unit.setting)
			}
			for i, survivor := range survivorNames {
				item.Hair[i] = parts[unit.replacement[survivor]]
			}
		}
		for i := range Survivors {
			if survivors&(1<<i) != 0 {
				item.Resources[i] = resource
			}
		}
	}

	items := make([]AvatarItem, 0, len(byName))
	for _, item := range byName {
		items = append(items, *item)
	}
	slices.SortFunc(items, func(a, b AvatarItem) int {
		return cmp.Or(cmp.Compare(a.Site, b.Site), strings.Compare(a.Name, b.Name))
	})
	return items
}

// avatarUnits reads every unit of the avatar scripts in dir.
func avatarUnits(t *testing.T, dir string) []avatarUnit {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var units []avatarUnit
	for _, file := range files {
		for _, script := range parseKeyValues(readText(t, file)).blocks {
			for _, unit := range script.block("unit").blocks {
				parsed := avatarUnit{setting: unit.values["settingname"], site: unit.values["equipmentsite"], only: unit.values["only"], replacement: map[string]string{}}
				for _, survivor := range survivorNames {
					if block := unit.block(survivor); block.values["replacement"] == "avatar_hair" {
						parsed.replacement[survivor] = block.values["modelname"]
					}
				}
				units = append(units, parsed)
			}
		}
	}
	return units
}

// keyValues is a KeyValues block: its values and its blocks, by key, in file order.
type keyValues struct {
	values map[string]string
	keys   []string
	blocks []*keyValues
}

// block returns the block under key, or an empty one.
func (kv *keyValues) block(key string) *keyValues {
	if i := slices.Index(kv.keys, key); i >= 0 {
		return kv.blocks[i]
	}
	return &keyValues{values: map[string]string{}}
}

var keyValueToken = regexp.MustCompile(`"([^"]*)"|[{}]|//[^\n]*|[^\s"{}]+`)

// parseKeyValues reads a KeyValues file, quoted or not.
func parseKeyValues(text string) *keyValues {
	root := &keyValues{values: map[string]string{}}
	stack := []*keyValues{root}
	key, pending := "", false
	for _, match := range keyValueToken.FindAllStringSubmatch(text, -1) {
		current := stack[len(stack)-1]
		switch token := match[0]; {
		case strings.HasPrefix(token, "//"):
		case token == "{":
			child := &keyValues{values: map[string]string{}}
			current.keys, current.blocks = append(current.keys, key), append(current.blocks, child)
			stack, pending = append(stack, child), false
		case token == "}":
			stack, pending = stack[:len(stack)-1], false
		case pending:
			current.values[key], pending = cmp.Or(match[1], strings.Trim(token, `"`)), false
		default:
			key, pending = cmp.Or(match[1], strings.Trim(token, `"`)), true
		}
	}
	return root
}

// nameTable reads a client.dll table of n pairs of a name pointer and an id, at va.
func nameTable(t *testing.T, file *pe.File, va uint32, n int) map[string]int {
	t.Helper()
	names := map[string]int{}
	for _, pair := range pairTable(t, file, va, n) {
		names[cString(t, file, uint32(pair[0]))] = pair[1]
	}
	return names
}

// pairTable reads n pairs of 32-bit values at va.
func pairTable(t *testing.T, file *pe.File, va uint32, n int) [][2]int {
	t.Helper()
	data := imageBytes(t, file, va, 8*n)
	pairs := make([][2]int, n)
	for i := range pairs {
		pairs[i] = [2]int{int(binary.LittleEndian.Uint32(data[8*i:])), int(binary.LittleEndian.Uint32(data[8*i+4:]))}
	}
	return pairs
}

func cString(t *testing.T, file *pe.File, va uint32) string {
	t.Helper()
	data := imageBytes(t, file, va, 128)
	if end := slices.Index(data, 0); end >= 0 {
		data = data[:end]
	}
	return string(data)
}

// imageBytes reads n bytes at a virtual address of the image.
func imageBytes(t *testing.T, file *pe.File, va uint32, n int) []byte {
	t.Helper()
	base := file.OptionalHeader.(*pe.OptionalHeader32).ImageBase
	rva := va - base
	for _, section := range file.Sections {
		if rva >= section.VirtualAddress && rva+uint32(n) <= section.VirtualAddress+section.Size {
			data := make([]byte, n)
			if _, err := section.ReadAt(data, int64(rva-section.VirtualAddress)); err != nil {
				t.Fatal(err)
			}
			return data
		}
	}
	t.Fatalf("address %#x is in no section", va)
	return nil
}
