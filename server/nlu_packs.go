package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Language packs: per-language trigger phrases for commands (packs/<lang>.json, embedded in the binary; a packs_dir adds to them),
// plus an optional "_lex" lexicon so timers, alarms, colours, cities, names and dates work in that language too.
// A pack is {"<command key>": ["phrase", ...], ..., "_fold": true, "_lex": {...}}.

//go:embed packs/*.json
var embeddedPacks embed.FS

// packTemplates are the commands a pack can trigger. Each returns a fresh result.
var packTemplates = map[string]func() nluResult{}

func init() {
	add := func(key string, f func() (nluResult, bool)) {
		packTemplates[key] = func() nluResult { r, _ := f(); return r }
	}
	add("greeting", func() (nluResult, bool) {
		return res("greeting", "play_animation", map[string]any{"animation_name": "Hi"})
	})
	add("timer_cancel", func() (nluResult, bool) {
		return util("timer_cancel", "timer", map[string]any{"timer": map[string]any{"operation": "cancel"}})
	})
	add("alarm_check", func() (nluResult, bool) {
		return util("alarm_check", "alarm", map[string]any{"alarm": map[string]any{"operation": "check"}})
	})
	add("alarm_remove_all", func() (nluResult, bool) {
		return util("alarm_remove", "alarm", map[string]any{"alarm": map[string]any{"operation": "remove", "index": ""}})
	})
	add("power_off", func() (nluResult, bool) { return res("power_off", "power_off", nil) })
	add("explore", func() (nluResult, bool) { return res("explore", "explore", nil) })
	add("be_quiet", func() (nluResult, bool) { return res("be_quiet", "stay_still", nil) })
	add("look_at_me", func() (nluResult, bool) { return res("look_at_me", "tree_search_and_interact", nil) })
	add("play_by_yourself", func() (nluResult, bool) {
		return res("play_by_yourself", "play_around", map[string]any{"play_type": "normal"})
	})
	add("dance", func() (nluResult, bool) { return res("dance", "dance", nil) })
	add("dance_with_lights", func() (nluResult, bool) {
		return res("dance_with_lights", "dance", map[string]any{"dance_type": "with_lights"})
	})
	add("dance_to_music", func() (nluResult, bool) {
		return res("listen_to_music", "dance", map[string]any{"dance_type": "to_music"})
	})
	for _, d := range []string{"forward", "backward", "left", "right", "turn_around"} {
		d := d
		add(d, func() (nluResult, bool) { return res(d, "basic_move", map[string]any{"type": d}) })
	}
	add("go_home", func() (nluResult, bool) { return res("go_home", "go_home", nil) })
	add("go_off_charger", func() (nluResult, bool) { return res("go_off_charger", "go_off_charger", nil) })
	add("yes", func() (nluResult, bool) { return res("smalltalk_yes", "", nil) })
	add("no", func() (nluResult, bool) { return res("smalltalk_no", "", nil) })
	add("time", func() (nluResult, bool) { return util("time", "time", map[string]any{"time": map[string]any{}}) })
	add("date", func() (nluResult, bool) { return util("date", "date", nil) })
	add("day", func() (nluResult, bool) { return util("day", "day", nil) })
	add("battery", func() (nluResult, bool) { return util("battery", "battery", nil) })
	add("check_update", func() (nluResult, bool) {
		return util("check_update", "update", map[string]any{"update": map[string]any{"operation": "check"}})
	})
	add("install_update", func() (nluResult, bool) {
		return util("update_firmware", "update", map[string]any{"update": map[string]any{"operation": "update"}})
	})
	add("weather", func() (nluResult, bool) {
		return nluResult{Intent: "weather", Behavior: "utilities", Paras: map[string]any{"utility_type": "weather"}, NeedWeather: true}, true
	})
	add("news", func() (nluResult, bool) {
		return nluResult{Intent: "news", Behavior: "utilities", Paras: map[string]any{"utility_type": "news"}, NeedNews: true}, true
	})
	add("take_photo", func() (nluResult, bool) { return util("take_photo", "take_photo", nil) })
	add("take_photo_recog", func() (nluResult, bool) { return util("take_photo_recog", "take_photo_recog", nil) })
	add("show_photo", func() (nluResult, bool) { return util("show_photo", "show_photo", nil) })
	for _, op := range []string{"up", "down", "max", "mid", "min", "mute"} {
		op := op
		add("volume_"+op, func() (nluResult, bool) {
			return util("volume_to", "volume", map[string]any{"volume": map[string]any{"operation": op}})
		})
	}
	add("light_off", func() (nluResult, bool) {
		return util("turn_off_light", "light", map[string]any{"light": map[string]any{"operation": "off", "room": ""}})
	})
	add("light_on", func() (nluResult, bool) {
		return util("turn_on_light", "light", map[string]any{"light": map[string]any{"operation": "set_hsl", "hsl": hsl([3]int{0, 0, 100}), "room": ""}})
	})
	add("chat", func() (nluResult, bool) {
		return res("chatgpt", "featured_game", map[string]any{"game_name": "chatgpt"})
	})
	for _, t := range []string{"age", "about_name", "lucky_number", "color", "sign", "birthday"} {
		t := t
		in := t
		if !strings.HasPrefix(t, "about_") {
			in = "about_" + t
		}
		add("about_"+strings.TrimPrefix(t, "about_"), func() (nluResult, bool) { return res(in, "about_emo", map[string]any{"type": t}) })
	}
	for _, g := range games {
		g := g
		add("game_"+g.name, func() (nluResult, bool) {
			return res("featured_game_"+g.name, "featured_game", map[string]any{"game_name": g.name})
		})
	}
	for _, i := range []struct{ key, action string }{{"music_piano", "piano"}, {"music_drum", "drum"}, {"music_trumpet", "trumpet"}} {
		i := i
		add(i.key, func() (nluResult, bool) {
			return res("play_music", "play_music", map[string]any{"type": "instrument", "action": i.action})
		})
	}
	add("music_sing", func() (nluResult, bool) { return res("play_music", "play_music", map[string]any{"type": "sing"}) })
	for _, w := range animalKeys {
		w := w
		add("animal_"+w, func() (nluResult, bool) {
			return res("animal", "play_animation", map[string]any{"animation_name": animalAnim(w)})
		})
	}

	// --- commands added to cover the vendor's published voice-command list ---
	anim := func(key, name string) {
		add(key, func() (nluResult, bool) { return res(key, "play_animation", map[string]any{"animation_name": name}) })
	}
	add("carry_mode", func() (nluResult, bool) { return res("carry_mode", "carry_mode", nil) })
	add("who_am_i", func() (nluResult, bool) { return res("who_am_i", "tree_search_and_update_name", nil) })
	add("music_random", func() (nluResult, bool) {
		return res("play_music", "play_music", map[string]any{"type": "instrument", "action": "random"})
	})
	add("sleep", func() (nluResult, bool) { return res("sleep", "tree_sleep_and_wake_up", nil) })
	add("update_home_station", func() (nluResult, bool) { return util("update_home_station", "update_home_station", nil) })
	add("dance_together", func() (nluResult, bool) {
		return res("dance_together", "dance", map[string]any{"dance_type": "together"})
	})
	add("music_together", func() (nluResult, bool) {
		return res("play_music", "play_music", map[string]any{"type": "instrument", "action": "dance_together"})
	})
	for _, b := range []struct{ key, action string }{{"bluetooth_enter", "enter"}, {"bluetooth_exit", "exit"}, {"music_start", "start"},
		{"music_stop", "stop"}, {"music_next", "next"}} {
		b := b
		add(b.key, func() (nluResult, bool) {
			return res("play_music", "play_music", map[string]any{"type": "bluetooth", "action": b.action})
		})
	}
	for _, h := range []struct{ key, typ string }{{"about_health", "about_health"}, {"about_comfort", "about_comfort"},
		{"about_happy_birthday", "happy_birthday"}} {
		h := h
		add(h.key, func() (nluResult, bool) { return res(h.key, "about_emo", map[string]any{"type": h.typ}) })
	}
	for _, st := range []string{"boy", "girl"} {
		st := st
		add("aidraw_"+st, func() (nluResult, bool) {
			return res("featured_game_aidraw", "featured_game", map[string]any{"game_name": "aidraw", "draw_style": "anime_" + st})
		})
	}
	// greetings and feelings are animations (names follow the documented ones; the rest are best guesses [V])
	for k, n := range map[string]string{"goodbye": "Byebye", "good_morning": "Good_morning", "good_afternoon": "Hi", "good_evening": "Hi",
		"good_night": "Byebye", "hug": "Hug", "kiss": "Kiss", "love": "Happy", "feel_sad": "Sad", "feel_tired": "Sad", "feel_sick": "Sad",
		"feel_angry": "Angry", "laser_eyes": "laser_eye", "devil_eyes": "devil",
		"festival_new_year": "New_year", "festival_valentines": "Valentines", "festival_work_day": "Work_day",
		"festival_april_fools": "April_fools", "festival_earth_day": "Earth_day", "festival_childrens_day": "Childrens_day",
		"festival_moon_day": "Moon_day", "festival_matrix": "Matrix", "festival_halloween": "Halloween",
		"festival_thanksgiving": "Thanksgiving", "festival_christmas": "Christmas"} {
		anim(k, n)
	}
}

// Keys that must match the whole utterance (a "yes" inside a sentence is not an answer).
var packExact = map[string]bool{"yes": true, "no": true, "greeting": true, "goodbye": true, "good_morning": true,
	"good_afternoon": true, "good_evening": true, "good_night": true}

// Keys the language model may pick on its own: nothing destructive, and nothing that is just flavour.
func llmSafe(key string) bool {
	if key == "power_off" || key == "install_update" || key == "update_home_station" {
		return false
	}
	for _, p := range []string{"festival_", "feel_", "animal_", "about_", "good", "aidraw_", "hug", "kiss", "love", "yes", "no"} {
		if strings.HasPrefix(key, p) {
			return false
		}
	}
	return true
}

// llmKeys lists the keys offered to the model, sorted.
func llmKeys() []string {
	var ks []string
	for k := range packTemplates {
		if llmSafe(k) {
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	return ks
}

type packPhrase struct {
	key    string
	tokens []string
}

type packLang struct {
	fold    bool
	chars   bool // written without spaces (zh, ja, ko): every non-ASCII character is a token
	phrases []packPhrase
	lex     *lexicon
}

type packSet map[string]*packLang // language -> pack

// normalizeLoose lower-cases and keeps letters and digits of any script.
func normalizeLoose(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func (pl *packLang) normalize(s string) string {
	switch {
	case pl.chars && pl.fold:
		return charNormalize(foldDiacritics(s))
	case pl.chars:
		return charNormalize(s)
	case pl.fold:
		return normalizeLoose(foldDiacritics(s))
	}
	return normalizeLoose(s)
}

// addFile merges one pack file into the language's pack.
func (ps packSet) addFile(name string, b []byte) error {
	lang := strings.ToLower(strings.TrimSuffix(filepath.Base(name), ".json"))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	pl := ps[lang]
	if pl == nil {
		pl = &packLang{}
		ps[lang] = pl
	}
	if v, ok := raw["_fold"]; ok {
		if err := json.Unmarshal(v, &pl.fold); err != nil {
			return fmt.Errorf("%s: _fold: %w", name, err)
		}
	}
	if v, ok := raw["_chars"]; ok {
		if err := json.Unmarshal(v, &pl.chars); err != nil {
			return fmt.Errorf("%s: _chars: %w", name, err)
		}
	}
	if v, ok := raw["_lex"]; ok {
		lx := &lexicon{Lang: lang}
		if err := json.Unmarshal(v, lx); err != nil {
			return fmt.Errorf("%s: _lex: %w", name, err)
		}
		lx.Fold = pl.fold
		lx.Chars = pl.chars
		pl.lex = lx.compile()
	}
	for key, v := range raw {
		if strings.HasPrefix(key, "_") {
			continue
		}
		if _, ok := packTemplates[key]; !ok {
			return fmt.Errorf("%s: unknown command key %q", name, key)
		}
		var phrases []string
		if err := json.Unmarshal(v, &phrases); err != nil {
			return fmt.Errorf("%s: %q: %w", name, key, err)
		}
		if len(phrases) > 64 {
			return fmt.Errorf("%s: too many phrases for %q", name, key)
		}
		for _, p := range phrases {
			n := pl.normalize(p)
			if n == "" {
				return fmt.Errorf("%s: empty phrase for %q", name, key)
			}
			pl.phrases = append(pl.phrases, packPhrase{key, strings.Fields(n)})
		}
	}
	return nil
}

// loadPacks loads the packs embedded in the binary, then adds any <lang>.json files from dir.
func loadPacks(dir string) (packSet, error) {
	ps := packSet{}
	ents, err := embeddedPacks.ReadDir("packs")
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		b, err := embeddedPacks.ReadFile("packs/" + e.Name())
		if err != nil {
			return nil, err
		}
		if err := ps.addFile(e.Name(), b); err != nil {
			return nil, err
		}
	}
	if dir == "" {
		return ps, nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if err := ps.addFile(f, b); err != nil {
			return nil, err
		}
	}
	return ps, nil
}

// match finds the command whose phrase is the longest one contained in the utterance.
func (pl *packLang) match(original string) (nluResult, bool) {
	if pl == nil || len(pl.phrases) == 0 {
		return nluResult{}, false
	}
	t := pl.normalize(original)
	tok := strings.Fields(t)
	slack, maxTok := 5, 16
	if pl.chars {
		maxTok = 40 // a character is a smaller unit than a word
	}
	if len(tok) == 0 || len(tok) > maxTok {
		return nluResult{}, false
	}
	best, bestLen, bestChars := "", 0, 0
	for _, p := range pl.phrases {
		ph := strings.Join(p.tokens, " ")
		if packExact[p.key] {
			if ph != t {
				continue
			}
		} else if !strings.Contains(" "+t+" ", " "+ph+" ") {
			continue
		}
		sl := slack
		if pl.chars {
			sl = 3 + len(p.tokens)/2 // short phrases (one or two characters) must make up almost all of the utterance
		}
		if len(tok) > len(p.tokens)+sl { // a phrase must make up most of the utterance, so long sentences stay conversation
			continue
		}
		if len(p.tokens) > bestLen || (len(p.tokens) == bestLen && len(ph) > bestChars) {
			best, bestLen, bestChars = p.key, len(p.tokens), len(ph)
		}
	}
	if best == "" {
		return nluResult{}, false
	}
	return packTemplates[best](), true
}

// parseLex runs the language's lexicon rules (timers, alarms, colours, cities, names, dates, animals): strict ones or loose ones.
func (pl *packLang) parseLex(original string, loose bool) (nluResult, bool) {
	if pl == nil || pl.lex == nil {
		return nluResult{}, false
	}
	rules := pl.lex.strictRules()
	if loose {
		rules = pl.lex.looseRules()
	}
	t := pl.lex.normalize(original)
	tok := strings.Fields(t)
	if len(tok) == 0 {
		return nluResult{}, false
	}
	for _, rule := range rules {
		if r, ok := rule(original, t, tok); ok {
			return r, true
		}
	}
	return nluResult{}, false
}

// parse understands one utterance. Non-English robots: the language's lexicon rules, then its phrases, then the English rules
// (people mix languages); English robots: the English rules, then the English phrase pack.
func (s *server) parse(text, lang string) (nluResult, bool) {
	if c := s.custom.match(text, lang); c != nil { // the owner's own commands come first and may override built-in phrases
		r := nluResult{Intent: "custom_" + c.Name, Behavior: c.Behavior, Paras: c.Paras, Custom: c}
		if r.Paras == nil {
			r.Paras = map[string]any{}
		}
		if c.Intent != "" {
			r.Intent = c.Intent
		}
		return r, true
	}
	if lang != "en" {
		if pl := s.packs[lang]; pl != nil {
			if r, ok := pl.parseLex(text, false); ok {
				return r, true
			}
			if r, ok := pl.match(text); ok {
				return r, true
			}
			if r, ok := pl.parseLex(text, true); ok {
				return r, true
			}
		}
	}
	if r, ok := parseIntent(text); ok {
		return r, true
	}
	return s.packs["en"].match(text)
}
