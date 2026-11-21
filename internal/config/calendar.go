package config

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/maintenance"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

// Calendar is a calendar file: what the server offers and when. calendar.example.yaml describes
// every key.
type Calendar struct {
	// Maintenance is when the server is down for maintenance; left out, never.
	Maintenance Maintenance  `yaml:"maintenance"`
	Versus      []VersusUnit `yaml:"versus"`
	Events      []Event      `yaml:"events"`
	Grants      []Grant      `yaml:"grants"`
	// CoopStages are the national co-op stage lists; with none open, national co-op offers every
	// stage.
	CoopStages   []StageList     `yaml:"coop_stages"`
	DailyGifts   []DailyGift     `yaml:"daily_gifts"`
	LoginLadders []LoginLadder   `yaml:"login_ladders"`
	Returning    []ReturningPack `yaml:"returning"`
	PlayLadders  []PlayLadder    `yaml:"play_ladders"`
}

// DailyGift is a gift every card gets once a service day, at its first login of the day while
// the gift is open, in the login pop-up's daily division, which shows no name.
type DailyGift struct {
	ID   int  `yaml:"id"`
	When When `yaml:"when"`
	// MaxLevel limits the gift to cards at this level or below; 0 or left out, every card.
	MaxLevel int   `yaml:"max_level"`
	Zet      int64 `yaml:"zet"`
	// Item is a weapon, a skill item, or svp, the only items the daily division shows.
	Item *GrantItem `yaml:"item"`
}

// LoginLadder is a list of gifts, one for each service day a card logs in during a run.
type LoginLadder struct {
	ID   int    `yaml:"id"`
	Name string `yaml:"name"`
	When When   `yaml:"when"`
	// Rounds is how many times a card goes through the steps in a run; 0 or left out, without end.
	Rounds int          `yaml:"rounds"`
	Steps  []LadderGift `yaml:"steps"`
}

// LadderGift is one step of a login ladder: ZG, an item, or both.
type LadderGift struct {
	Zet  int64      `yaml:"zet"`
	Item *GrantItem `yaml:"item"`
}

// ReturningPack is a gift for a card that comes back after AbsentDays service days or more without
// a login or a play, at its first login during a run, once per run.
type ReturningPack struct {
	ID         int        `yaml:"id"`
	Name       string     `yaml:"name"`
	When       When       `yaml:"when"`
	AbsentDays int        `yaml:"absent_days"`
	Zet        int64      `yaml:"zet"`
	Item       *GrantItem `yaml:"item"`
}

// PlayLadder is a set of gifts for counts of a card's plays during a run, given in the result of
// the play that reaches each count.
type PlayLadder struct {
	ID    int        `yaml:"id"`
	When  When       `yaml:"when"`
	Count PlayCount  `yaml:"count"`
	Steps []PlayStep `yaml:"steps"`
}

// PlayCount is which plays a play ladder counts.
type PlayCount struct {
	// Mode is coop, event, single, versus, or any; left out, any.
	Mode string `yaml:"mode"`
	// Stage is the stage id counted; 0 or left out, any.
	Stage int `yaml:"stage"`
	// Cleared counts only chapters cleared in co-op or single play.
	Cleared bool `yaml:"cleared"`
}

// PlayStep is the gift for reaching At counted plays, shown under its name.
type PlayStep struct {
	At   int        `yaml:"at"`
	Name string     `yaml:"name"`
	Zet  int64      `yaml:"zet"`
	Item *GrantItem `yaml:"item"`
}

// When is when a calendar entry is open: a first run from Start until End, repeated Every so
// often, the last run starting no later than Until. Times are RFC 3339 timestamps, such as
// "2027-04-22T08:00:00+09:00", or dates in the server's time zone, each standing for its service
// day, from 08:00 to 08:00 the next morning: a Start date means its 08:00, and an End or Until
// date the 08:00 after it. Every is a number of
// days, weeks, or years, such as "1d", "2w", or "1y"; left out, the entry does not repeat.
type When struct {
	Start string `yaml:"start"`
	End   string `yaml:"end"`
	Every string `yaml:"every"`
	Until string `yaml:"until"`
}

// Maintenance configures the maintenance windows, during which games are offline in maintenance.
type Maintenance struct {
	// Warn is how long before a window games are told it is coming, such as "30m"; 0 or left out,
	// they are not told.
	Warn time.Duration `yaml:"warn"`
	// Windows are the windows, which may overlap or extend each other.
	Windows []MaintenanceWindow `yaml:"windows"`
}

// MaintenanceWindow is a maintenance window.
type MaintenanceWindow struct {
	When When `yaml:"when"`
}

// VersusUnit is one map of the national versus schedule, offered when its When says.
type VersusUnit struct {
	ID      int `yaml:"id"`
	Stage   int `yaml:"stage"`
	Chapter int `yaml:"chapter"`
	// Difficulty is easy, normal, advanced or expert.
	Difficulty string `yaml:"difficulty"`
	// Cost is the SVP charged to enter; 0 charges the operator's versus setting.
	Cost       int    `yaml:"cost"`
	TakeWeapon bool   `yaml:"take_weapon"`
	GameMode   string `yaml:"game_mode"`
	When       When   `yaml:"when"`
}

// Event is a national or store event. It is offered when its When says (always, without one)
// during its daily hours (all day, without them).
type Event struct {
	ID int `yaml:"id"`
	// Type is national, store, national_ex or store_ex. A store event is offered only when its
	// id equals the cabinet's store event id.
	Type    string `yaml:"type"`
	Stage   int    `yaml:"stage"`
	Chapter int    `yaml:"chapter"`
	// Difficulty is easy, normal, advanced or expert; God shows a fifth "GOD" badge.
	Difficulty     string `yaml:"difficulty"`
	God            bool   `yaml:"god"`
	Cost           int    `yaml:"cost"`
	TakeWeapon     bool   `yaml:"take_weapon"`
	TakeDisposable bool   `yaml:"take_disposable"`
	MultiMatch     bool   `yaml:"multi_match"`
	GameMode       string `yaml:"game_mode"`
	Name           string `yaml:"name"`
	// Detail is shown on the announcement; "|" starts a new line.
	Detail   string `yaml:"detail"`
	MinLevel int    `yaml:"min_level"`
	MaxLevel int    `yaml:"max_level"`
	MinClass int    `yaml:"min_class"`
	MaxClass int    `yaml:"max_class"`
	When     When   `yaml:"when"`
	// DailyOpen and DailyClose are times of day in the server's time zone, as "HH:MM"; a close
	// before the open runs past midnight.
	DailyOpen  string `yaml:"daily_open"`
	DailyClose string `yaml:"daily_close"`
}

// Grant is a gift every card gets once in each run of the grant, at its first login during the
// run: ZG, an item, or both, shown in the login's reward pop-up under its name.
type Grant struct {
	// ID tells grants apart: a card gets each run of an id once, so an id must not be reused for
	// another grant. It is at least 1.
	ID   int    `yaml:"id"`
	Name string `yaml:"name"`
	When When   `yaml:"when"`
	Zet  int64  `yaml:"zet"`
	// Item is the item given; left out, none.
	Item *GrantItem `yaml:"item"`
}

// GrantItem is the item a grant gives.
type GrantItem struct {
	// Category is weapon, skill_item, svp, title, upper, lower, head, or accessory. upper and
	// lower are clothing, head a hat or other head accessory, and accessory one worn on the face,
	// upper body, or lower body.
	Category string `yaml:"category"`
	// ID is the weapon's or skill item's database id in the game's catalogue, or the title's own
	// number; svp and avatar items have none.
	ID int64 `yaml:"id"`
	// Unit is an avatar item's setting name in the game's avatar scripts, less any survivor
	// suffix (firefighter_upper_avt for firefighter_upper_avt_y and the others).
	Unit string `yaml:"unit"`
	// Color is an avatar item's color index, 0 to 99.
	Color int `yaml:"color"`
	// Name is shown in the pop-up, and on the card once the item is set; svp has none.
	Name string `yaml:"name"`
	// Count is how many weapons or skill items, or for svp the seconds; other items are one.
	Count int64 `yaml:"count"`
}

// StageList is a national co-op stage list, offered when its When says.
type StageList struct {
	When When `yaml:"when"`
	// Stages are stage ids, in the order offered.
	Stages []int `yaml:"stages"`
}

// loadCalendar reads the calendar file at path, with keys it does not know an error.
func loadCalendar(path string) (Calendar, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Calendar{}, err
	}
	var c Calendar
	loader, err := yaml.NewLoader(bytes.NewReader(data), yaml.WithKnownFields())
	if err != nil {
		return Calendar{}, err
	}
	if err := loader.Load(&c); err != nil && !errors.Is(err, io.EOF) {
		return Calendar{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// span reads w as a calendar span; the zero When is the always-open zero span. Start and End are
// needed for any other.
func (w When) span() (calendar.Span, error) {
	if w == (When{}) {
		return calendar.Span{}, nil
	}
	if w.Start == "" || w.End == "" {
		return calendar.Span{}, errors.New("when needs a start and an end")
	}
	start, errStart := moment(w.Start, false)
	end, errEnd := moment(w.End, true)
	every, errEvery := repeat(w.Every)
	var until time.Time
	var errUntil error
	if w.Until != "" {
		until, errUntil = moment(w.Until, true)
	}
	if err := errors.Join(errStart, errEnd, errEvery, errUntil); err != nil {
		return calendar.Span{}, err
	}
	span := calendar.Span{Start: start, End: end, Every: every, Until: until}
	return span, span.Check()
}

// moment reads a time: an RFC 3339 timestamp, or a date in the server's time zone, which means the
// start of its service day when it starts something and the end of it when it ends something.
func moment(text string, ending bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, text); err == nil {
		return t, nil
	}
	date, err := time.ParseInLocation(time.DateOnly, text, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("time %q is neither an RFC 3339 timestamp nor a date", text)
	}
	if ending {
		date = date.AddDate(0, 0, 1)
	}
	return date.Add(calendar.DayTurn), nil
}

// repeat reads how often a span repeats: a number of days, weeks, or years, such as "7d", "2w", or
// "1y"; "" never repeats.
func repeat(text string) (calendar.Every, error) {
	if text == "" {
		return calendar.Every{}, nil
	}
	n, err := strconv.Atoi(text[:len(text)-1])
	if err != nil || n < 1 {
		return calendar.Every{}, fmt.Errorf("every %q is not a count of days, weeks, or years, such as 7d, 2w, or 1y", text)
	}
	switch text[len(text)-1] {
	case 'd':
		return calendar.Every{Days: n}, nil
	case 'w':
		return calendar.Every{Days: 7 * n}, nil
	case 'y':
		return calendar.Every{Years: n}, nil
	}
	return calendar.Every{}, fmt.Errorf("every %q is not a count of days, weeks, or years, such as 7d, 2w, or 1y", text)
}

// MaintenanceSchedule returns the maintenance windows, or the first one the calendar gets wrong.
func (c Calendar) MaintenanceSchedule() (maintenance.Schedule, error) {
	if c.Maintenance.Warn < 0 {
		return maintenance.Schedule{}, fmt.Errorf("maintenance.warn %s is negative", c.Maintenance.Warn)
	}
	schedule := maintenance.Schedule{Warn: c.Maintenance.Warn}
	for i, w := range c.Maintenance.Windows {
		span, err := w.When.span()
		if err == nil && span.Always() {
			err = errors.New("needs a when")
		}
		if err != nil {
			return maintenance.Schedule{}, fmt.Errorf("maintenance window %d: %w", i+1, err)
		}
		schedule.Windows = append(schedule.Windows, span)
	}
	return schedule, nil
}

// difficulties maps difficulty names to the game's difficulty plus 1.
var difficulties = map[string]int{"easy": 1, "normal": 2, "advanced": 3, "expert": 4}

// eventTypes maps event type names to the game's event types.
var eventTypes = map[string]int{"national": zet.EventNational, "store": zet.EventStore, "national_ex": zet.EventNationalEX, "store_ex": zet.EventStoreEX}

// Schedule returns the versus units, events, grants, and national co-op stage lists as the game
// server offers them, or the first entry the calendar gets wrong. Versus units, grants, and stage
// lists need a when; an event without one is always open.
func (c Calendar) Schedule() (zet.Schedule, error) {
	var schedule zet.Schedule
	for _, unit := range c.Versus {
		difficulty, err := lookup("versus unit", unit.ID, "difficulty", difficulties, unit.Difficulty)
		if err != nil {
			return zet.Schedule{}, err
		}
		when, err := required(unit.When)
		if err != nil {
			return zet.Schedule{}, fmt.Errorf("versus unit %d: %w", unit.ID, err)
		}
		schedule.Versus = append(schedule.Versus, zet.VersusUnit{
			ID: unit.ID, StageID: unit.Stage, ChapterID: unit.Chapter, Difficulty: difficulty,
			Cost: unit.Cost, TakeWeapon: unit.TakeWeapon, GameMode: unit.GameMode, When: when,
		})
	}
	for _, event := range c.Events {
		converted, err := event.convert()
		if err != nil {
			return zet.Schedule{}, err
		}
		schedule.Events = append(schedule.Events, converted)
	}
	ids := map[int]bool{}
	for _, grant := range c.Grants {
		if ids[grant.ID] {
			return zet.Schedule{}, fmt.Errorf("grant %d is listed twice", grant.ID)
		}
		ids[grant.ID] = true
		converted, err := grant.convert()
		if err != nil {
			return zet.Schedule{}, fmt.Errorf("grant %d: %w", grant.ID, err)
		}
		schedule.Grants = append(schedule.Grants, converted)
	}
	for i, list := range c.CoopStages {
		converted, err := list.convert()
		if err != nil {
			return zet.Schedule{}, fmt.Errorf("co-op stage list %d: %w", i+1, err)
		}
		schedule.CoopStages = append(schedule.CoopStages, converted)
	}
	if err := c.campaigns(&schedule); err != nil {
		return zet.Schedule{}, err
	}
	return schedule, nil
}

// playModes maps play count modes to the matching modes (game_mode) they count.
var playModes = map[string][]int64{"any": nil, "coop": {1}, "event": {2, 3, 4}, "single": {5}, "versus": {7, 8, 9, 10}}

// campaigns adds the calendar's campaigns to schedule, or returns the first one it gets wrong:
// an id below 1 or used twice in its kind, a bad or missing when, a bad gift, a daily gift item
// other than a weapon, skill item, or svp, a ladder without steps or a name, a returning pack
// without its days away, or a play count without a known mode or a step count.
func (c Calendar) campaigns(schedule *zet.Schedule) error {
	ids := map[string]map[int]bool{}
	check := func(kind string, id int, when When) (calendar.Span, error) {
		if ids[kind] == nil {
			ids[kind] = map[int]bool{}
		}
		switch {
		case id < 1:
			return calendar.Span{}, fmt.Errorf("%s %d: id must be at least 1", kind, id)
		case ids[kind][id]:
			return calendar.Span{}, fmt.Errorf("%s %d is listed twice", kind, id)
		}
		ids[kind][id] = true
		span, err := required(when)
		if err != nil {
			return calendar.Span{}, fmt.Errorf("%s %d: %w", kind, id, err)
		}
		return span, nil
	}

	for _, daily := range c.DailyGifts {
		when, err := check("daily gift", daily.ID, daily.When)
		if err != nil {
			return err
		}
		if daily.Item != nil && !slices.Contains([]string{"weapon", "skill_item", "svp"}, daily.Item.Category) {
			return fmt.Errorf("daily gift %d: the daily division shows only a weapon, a skill item, or svp", daily.ID)
		}
		gift, err := NewGift("daily", daily.Zet, daily.Item)
		if err != nil {
			return fmt.Errorf("daily gift %d: %w", daily.ID, err)
		}
		schedule.DailyGifts = append(schedule.DailyGifts, zet.DailyGift{ID: daily.ID, When: when, MaxLevel: daily.MaxLevel, Gift: gift})
	}

	for _, ladder := range c.LoginLadders {
		when, err := check("login ladder", ladder.ID, ladder.When)
		if err != nil {
			return err
		}
		if len(ladder.Steps) == 0 || ladder.Rounds < 0 {
			return fmt.Errorf("login ladder %d: needs steps and rounds of 0 or more", ladder.ID)
		}
		converted := zet.LoginLadder{ID: ladder.ID, Name: ladder.Name, When: when, Rounds: ladder.Rounds}
		for i, step := range ladder.Steps {
			gift, err := NewGift(ladder.Name, step.Zet, step.Item)
			if err != nil {
				return fmt.Errorf("login ladder %d, step %d: %w", ladder.ID, i+1, err)
			}
			converted.Steps = append(converted.Steps, gift)
		}
		schedule.LoginLadders = append(schedule.LoginLadders, converted)
	}

	for _, pack := range c.Returning {
		when, err := check("returning pack", pack.ID, pack.When)
		if err != nil {
			return err
		}
		if pack.AbsentDays < 1 {
			return fmt.Errorf("returning pack %d: absent_days must be at least 1", pack.ID)
		}
		gift, err := NewGift(pack.Name, pack.Zet, pack.Item)
		if err != nil {
			return fmt.Errorf("returning pack %d: %w", pack.ID, err)
		}
		schedule.Returning = append(schedule.Returning, zet.ReturningPack{ID: pack.ID, When: when, AbsentDays: pack.AbsentDays, Gift: gift})
	}

	for _, ladder := range c.PlayLadders {
		when, err := check("play ladder", ladder.ID, ladder.When)
		if err != nil {
			return err
		}
		mode := cmp.Or(ladder.Count.Mode, "any")
		modes, known := playModes[mode]
		if !known {
			return fmt.Errorf("play ladder %d: unknown mode %q", ladder.ID, mode)
		}
		if len(ladder.Steps) == 0 {
			return fmt.Errorf("play ladder %d: needs steps", ladder.ID)
		}
		converted := zet.PlayLadder{ID: ladder.ID, When: when, Filter: store.ResultFilter{Modes: modes, StageID: int64(ladder.Count.Stage), Cleared: ladder.Count.Cleared}}
		for i, step := range ladder.Steps {
			if step.At < 1 {
				return fmt.Errorf("play ladder %d, step %d: at must be at least 1", ladder.ID, i+1)
			}
			gift, err := NewGift(step.Name, step.Zet, step.Item)
			if err != nil {
				return fmt.Errorf("play ladder %d, step %d: %w", ladder.ID, i+1, err)
			}
			converted.Steps = append(converted.Steps, zet.PlayStep{At: step.At, Gift: gift})
		}
		schedule.PlayLadders = append(schedule.PlayLadders, converted)
	}
	return nil
}

// required reads a When that must be given.
func required(w When) (calendar.Span, error) {
	span, err := w.span()
	if err == nil && span.Always() {
		err = errors.New("needs a when")
	}
	return span, err
}

func (event Event) convert() (zet.Event, error) {
	difficulty, err := lookup("event", event.ID, "difficulty", difficulties, event.Difficulty)
	if err != nil {
		return zet.Event{}, err
	}
	kind, err := lookup("event", event.ID, "type", eventTypes, event.Type)
	if err != nil {
		return zet.Event{}, err
	}
	when, err := event.When.span()
	if err != nil {
		return zet.Event{}, fmt.Errorf("event %d: %w", event.ID, err)
	}
	dailyOpen, err := timeOfDay(event.ID, event.DailyOpen)
	if err != nil {
		return zet.Event{}, err
	}
	dailyClose, err := timeOfDay(event.ID, event.DailyClose)
	if err != nil {
		return zet.Event{}, err
	}
	special := 0
	if event.God {
		special = 1
	}
	return zet.Event{
		ID: event.ID, Type: kind, StageID: event.Stage, ChapterID: event.Chapter, Difficulty: difficulty,
		SpecialMode: special, Cost: event.Cost, TakeWeapon: event.TakeWeapon, TakeDisposable: event.TakeDisposable,
		MultiMatch: event.MultiMatch, GameMode: event.GameMode, Name: event.Name, Detail: event.Detail,
		MinLevel: event.MinLevel, MaxLevel: event.MaxLevel, MinClassID: event.MinClass, MaxClassID: event.MaxClass,
		When: when, DailyOpen: dailyOpen, DailyClose: dailyClose,
	}, nil
}

// convert returns the grant as the game server hands it out, or what the calendar gets wrong: an
// id below 1, a bad or missing when, or a bad gift (see [NewGift]).
func (g Grant) convert() (zet.Grant, error) {
	if g.ID < 1 {
		return zet.Grant{}, errors.New("id must be at least 1")
	}
	when, err := required(g.When)
	if err != nil {
		return zet.Grant{}, err
	}
	gift, err := NewGift(g.Name, g.Zet, g.Item)
	if err != nil {
		return zet.Grant{}, err
	}
	return zet.Grant{ID: g.ID, When: when, Gift: gift}, nil
}

// convert returns the stage list as the game server offers it, or what the calendar gets wrong: a
// bad or missing when, no stages, or a stage that is not one of build 1.26's.
func (l StageList) convert() (zet.StageList, error) {
	when, err := required(l.When)
	if err != nil {
		return zet.StageList{}, err
	}
	if len(l.Stages) == 0 {
		return zet.StageList{}, errors.New("lists no stages")
	}
	for _, id := range l.Stages {
		if !slices.ContainsFunc(game.Stages, func(stage game.Stage) bool { return stage.ID == id }) {
			return zet.StageList{}, fmt.Errorf("stage %d is not one of the game's", id)
		}
	}
	return zet.StageList{When: when, Stages: l.Stages}, nil
}

// itemCategories maps grant item categories to the game's item categories.
var itemCategories = map[string]store.ItemKind{
	"weapon": store.Weapon, "skill_item": store.SkillItem, "svp": store.SVP, "title": store.Title,
	"upper": store.Upper, "lower": store.Lower, "head": store.Head, "accessory": store.Accessory,
}

// NewGift returns the gift of name, zg ZG, and item (none when nil), or what is wrong with it: no
// name, negative ZG, nothing to give, an unknown item category, or an item without what its
// category needs. A weapon or skill item needs an id, a name, and a count, svp a count, a title an
// id and a name, and an avatar item a name, a color from 0 to 99, and a unit worn where its
// category says.
func NewGift(name string, zg int64, item *GrantItem) (zet.Gift, error) {
	switch {
	case name == "":
		return zet.Gift{}, errors.New("name must not be empty")
	case zg < 0:
		return zet.Gift{}, errors.New("zet is negative")
	case zg == 0 && item == nil:
		return zet.Gift{}, errors.New("gives neither zet nor an item")
	}
	gift := zet.Gift{Gift: store.Gift{Name: name, Zet: zg}}
	if item == nil {
		return gift, nil
	}
	kind, ok := itemCategories[item.Category]
	if !ok {
		return zet.Gift{}, fmt.Errorf("unknown item category %q", item.Category)
	}
	gift.Item = store.Owned{Kind: kind, ItemID: item.ID, Name: item.Name, Count: item.Count}
	switch {
	case kind.Wearable():
		avatar, found := game.Avatar(item.Unit)
		switch {
		case !found:
			return zet.Gift{}, fmt.Errorf("unknown avatar unit %q", item.Unit)
		case zet.KindOf(avatar.Site) != kind:
			return zet.Gift{}, fmt.Errorf("avatar unit %s is not a %s item", item.Unit, item.Category)
		case item.Color < 0 || item.Color > 99:
			return zet.Gift{}, fmt.Errorf("color %d is not 0 to 99", item.Color)
		case item.Name == "":
			return zet.Gift{}, fmt.Errorf("a %s needs a name", item.Category)
		}
		gift.Avatar, gift.Color, gift.Item.ItemID = &avatar, item.Color, 0
	case kind == store.SVP:
		if item.Count < 1 {
			return zet.Gift{}, errors.New("svp count must be at least 1")
		}
	case item.ID < 1 || item.Name == "":
		return zet.Gift{}, fmt.Errorf("a %s needs an id and a name", item.Category)
	case kind.Stacks() && item.Count < 1:
		return zet.Gift{}, errors.New("item count must be at least 1")
	}
	if !kind.Stacks() && kind != store.SVP {
		gift.Item.Count = 1
	}
	return gift, nil
}

func lookup(what string, id int, key string, values map[string]int, name string) (int, error) {
	value, ok := values[name]
	if !ok {
		return 0, fmt.Errorf("%s %d: unknown %s %q", what, id, key, name)
	}
	return value, nil
}

// timeOfDay reads an "HH:MM" time of day in the server's time zone, as an instant on 2 January
// 1970: the game compares only the UTC time of day, and its time functions reject times before
// 1970. An empty time is the zero time.
func timeOfDay(event int, text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("15:04", text)
	if err != nil {
		return time.Time{}, fmt.Errorf("event %d: daily time %q is not HH:MM", event, text)
	}
	return time.Date(1970, 1, 2, parsed.Hour(), parsed.Minute(), 0, 0, time.Local), nil
}
