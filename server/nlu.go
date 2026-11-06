package main

import (
	"regexp"
	"strconv"
	"strings"
)

// Native-style language understanding: turns what the robot heard into the intent/behaviour/parameters its handler
// expects (catalogue: spec-clean/cloud_api.md, public emo-libre/Documentation Intents/ and Behaviors/). Deterministic,
// no model call, microseconds. Phrases are our own wording. Anything unmatched falls through to free chat.

type nluResult struct {
	Intent      string
	Behavior    string // may be "" (small-talk answers carry no behaviour)
	Paras       map[string]any
	NeedWeather bool           // fill utilities.weather from the weather backend
	NeedNews    bool           // fill utilities.news from the news backend
	City        string         // weather for this city instead of the robot's own
	Say         string         // if set, speak this prompt key instead (e.g. a backend is missing)
	Custom      *customCommand // set when the owner's own command matched; answerIntent carries it out
}

func res(intent, behavior string, paras map[string]any) (nluResult, bool) {
	if paras == nil {
		paras = map[string]any{}
	}
	return nluResult{Intent: intent, Behavior: behavior, Paras: paras}, true
}

func util(intent, kind string, extra map[string]any) (nluResult, bool) {
	p := map[string]any{"utility_type": kind}
	for k, v := range extra {
		p[k] = v
	}
	return res(intent, "utilities", p)
}

// ---- text helpers ----

func words(t string) []string { return strings.Fields(t) }

func padded(t string) string { return " " + t + " " }

// hasPhrase: whole-word(s) containment in normalized text.
func hasPhrase(t string, phrases ...string) bool {
	p := padded(t)
	for _, s := range phrases {
		if strings.Contains(p, " "+s+" ") {
			return true
		}
	}
	return false
}

var numWords = map[string]int{
	"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17,
	"eighteen": 18, "nineteen": 19, "twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60, "seventy": 70,
	"eighty": 80, "ninety": 90, "a": 1, "an": 1,
}

// digitsWithSuffix reads "5th", "1st", "22nd", "3rd".
func digitsWithSuffix(w string) (int, bool) {
	for _, suf := range []string{"st", "nd", "rd", "th"} {
		if strings.HasSuffix(w, suf) {
			if n, err := strconv.Atoi(strings.TrimSuffix(w, suf)); err == nil && n >= 0 && n < 100000 {
				return n, true
			}
		}
	}
	return 0, false
}

// number reads a number starting at tokens[i]: digits, "seven", "twenty five". Returns value and tokens used.
func number(tok []string, i int) (int, int) {
	if i >= len(tok) {
		return 0, 0
	}
	if n, err := strconv.Atoi(tok[i]); err == nil && n >= 0 && n < 100000 {
		return n, 1
	}
	if n, ok := digitsWithSuffix(tok[i]); ok {
		return n, 1
	}
	if v, ok := numWords[tok[i]]; ok && tok[i] != "a" && tok[i] != "an" {
		if v >= 20 && v%10 == 0 && i+1 < len(tok) {
			if w, ok := numWords[tok[i+1]]; ok && w >= 1 && w <= 9 && tok[i+1] != "a" {
				return v + w, 2
			}
		}
		return v, 1
	}
	return 0, 0
}

// ---- main entry ----

// parseIntent returns the first matching rule. Order matters: specific before general.
func parseIntent(original string) (nluResult, bool) {
	t := normalizeText(original)
	if t == "" {
		return nluResult{}, false
	}
	tok := words(t)
	for _, rule := range rules {
		if r, ok := rule(original, t, tok); ok {
			return r, true
		}
	}
	return nluResult{}, false
}

type ruleFn func(orig, t string, tok []string) (nluResult, bool)

var rules []ruleFn

func init() {
	rules = []ruleFn{
		ruleSmallTalk, ruleNameBirthday, ruleChatStart, englishLex.ruleEyes, ruleLight, ruleVolume, englishLex.ruleTimer, englishLex.ruleAlarm,
		ruleClockDate, ruleBattery, ruleUpdate, englishLex.ruleWeatherCity, ruleWeather, ruleNews, rulePhoto, ruleAIDraw, ruleGames,
		ruleMusic, ruleDance, ruleMove, ruleAboutEmo, ruleAnimals, ruleSimple,
	}
}

// ---- small-talk answers: context-free, the robot decides whether it asked a question ----

var yesWords = map[string]bool{"yes": true, "yeah": true, "yep": true, "sure": true, "ok": true, "okay": true,
	"of course": true, "yes please": true, "absolutely": true}
var noWords = map[string]bool{"no": true, "nope": true, "nah": true, "no thanks": true, "not now": true, "no thank you": true}

func ruleSmallTalk(orig, t string, tok []string) (nluResult, bool) {
	if yesWords[t] {
		return res("smalltalk_yes", "", nil)
	}
	if noWords[t] {
		return res("smalltalk_no", "", nil)
	}
	// a number 1..10 on its own, or "i choose / number / i play at N"
	rest := tok
	for len(rest) > 0 && (rest[0] == "i" || rest[0] == "choose" || rest[0] == "play" || rest[0] == "at" || rest[0] == "number" || rest[0] == "pick") {
		rest = rest[1:]
	}
	if len(rest) > 0 {
		if n, used := number(rest, 0); used == len(rest) && n >= 1 && n <= 10 {
			return res("user_choose_number", "", nil)
		}
	}
	return nluResult{}, false
}

// ---- names and birthdays ----

// Strong forms accept any case; the weaker "I'm / it's / that's" forms need a capitalised name (speech-to-text capitalises
// names) so that "that's nice" or "I am human" are not mistaken for introductions.
var nameStrong = regexp.MustCompile(`(?i)^\s*(?:my name is|my name's|call me|this is)\s+([\p{L}][\p{L}'-]{0,30})\s*[.!]?\s*$`)
var nameWeak = regexp.MustCompile(`^\s*(?i:i am|i'm|it is|it's|that is|that's)\s+(\p{Lu}[\p{L}'-]{0,30})\s*[.!]?\s*$`)

var months = map[string]int{"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6, "july": 7, "august": 8,
	"september": 9, "october": 10, "november": 11, "december": 12}

func ruleNameBirthday(orig, t string, tok []string) (nluResult, bool) {
	if hasPhrase(t, "who am i", "what s my name", "what is my name", "do you know me", "do you know who i am", "do you know my name") {
		return res("who_am_i", "tree_search_and_update_name", nil)
	}
	if hasPhrase(t, "my birthday is", "i was born on", "i was born", "my birthday") && !hasPhrase(t, "when", "what") {
		mo, day := 0, 0
		for i, w := range tok {
			if m, ok := months[w]; ok {
				mo = m
				if n, used := number(tok, i+1); used > 0 {
					day = n
				} else if i > 0 {
					if n, used := number(tok, i-1); used > 0 {
						day = n
					} else if i > 1 && (tok[i-1] == "of" || tok[i-1] == "th" || tok[i-1] == "st" || tok[i-1] == "nd" || tok[i-1] == "rd") {
						day, _ = number(tok, i-2)
					}
				}
			}
		}
		if mo == 0 {
			if m := regexp.MustCompile(`\b(\d{1,2})[./-](\d{1,2})\b`).FindStringSubmatch(orig); m != nil {
				a, _ := strconv.Atoi(m[1])
				b, _ := strconv.Atoi(m[2])
				mo, day = a, b // month first; the robot reads "%d.%d" as month, day
			}
		}
		if mo >= 1 && mo <= 12 && day >= 1 && day <= 31 {
			return res("my_birthday_is", "tree_search_and_update_name", map[string]any{"birth_date": strconv.Itoa(mo) + "." + strconv.Itoa(day)})
		}
		return nluResult{}, false
	}
	m := nameStrong.FindStringSubmatch(orig)
	if m == nil {
		m = nameWeak.FindStringSubmatch(orig)
	}
	if m != nil {
		n := strings.ToLower(m[1])
		stop := map[string]bool{"fine": true, "good": true, "great": true, "ok": true, "okay": true, "hungry": true, "tired": true,
			"sad": true, "happy": true, "here": true, "back": true, "ready": true, "sorry": true, "bored": true, "cold": true, "hot": true}
		if !stop[n] {
			return res("my_name_is", "tree_search_and_update_name", map[string]any{"name": strings.ToUpper(n[:1]) + n[1:]})
		}
	}
	return nluResult{}, false
}

func ruleChatStart(orig, t string, tok []string) (nluResult, bool) {
	if hasPhrase(t, "let s chat", "lets chat", "chat with me", "talk with me", "talk to me", "let s talk", "lets talk", "have a chat", "connect to openai", "connect to chatgpt", "connect to chat gpt", "connect to open ai", "open chatgpt") {
		return res("chatgpt", "featured_game", map[string]any{"game_name": "chatgpt"})
	}
	if isChatModeRequest(tok) {
		return res("chatgpt", "featured_game", map[string]any{"game_name": "chatgpt"})
	}
	return nluResult{}, false
}

// Speech-to-text often mangles "ChatGPT mode" (heard on the owner's robot: "chat GPP mode", "Jack Cheepity Mode", "chat qbt mode",
// "GPD mode"). chatWords/gptWords are the sounds of the two halves; spelled-out letters ("g p t") are joined before the check.
var (
	chatWords = map[string]bool{"chat": true, "chad": true, "jack": true, "chap": true, "check": true, "chatgpt": true, "chatbot": true, "talk": true}
	gptWords  = map[string]bool{"gpt": true, "gpp": true, "gpd": true, "gbt": true, "gbd": true, "qbt": true, "qpt": true, "gt": true,
		"cheepity": true, "cheapity": true, "chipiti": true, "chepity": true, "jeepity": true, "gpt4": true, "gpt3": true}
	chatModeLeadIn = []string{"please", "enter", "start", "open", "activate", "launch", "turn", "on", "switch", "go", "to", "into", "the", "use", "let", "s", "lets", "emo", "hey"}
)

// isChatModeRequest reports a short command of the form [lead-in words] <chat-word>? <gpt-word>? "mode", with at least one of the two.
func isChatModeRequest(tok []string) bool {
	if len(tok) < 2 || len(tok) > 7 || tok[len(tok)-1] != "mode" {
		return false
	}
	lead := map[string]bool{}
	for _, w := range chatModeLeadIn {
		lead[w] = true
	}
	i := 0
	for i < len(tok)-1 && lead[tok[i]] {
		i++
	}
	var core strings.Builder // the words before "mode", with single letters joined: "g p t" -> "gpt"
	var parts []string
	for _, w := range tok[i : len(tok)-1] {
		if len(w) == 1 {
			core.WriteString(w)
			continue
		}
		if core.Len() > 0 {
			parts = append(parts, core.String())
			core.Reset()
		}
		parts = append(parts, w)
	}
	if core.Len() > 0 {
		parts = append(parts, core.String())
	}
	switch len(parts) {
	case 1: // a sound-alike ("jack", "check") is not enough on its own
		return gptWords[parts[0]] || map[string]bool{"chat": true, "chatgpt": true, "chatbot": true, "talk": true}[parts[0]]
	case 2:
		return chatWords[parts[0]] && (gptWords[parts[1]] || chatWords[parts[1]])
	}
	return false
}

// ---- light ----

var colours = []struct {
	words []string
	hsl   [3]int
}{
	{[]string{"red"}, [3]int{0, 100, 100}}, {[]string{"orange"}, [3]int{30, 100, 100}}, {[]string{"yellow"}, [3]int{50, 100, 100}},
	{[]string{"green"}, [3]int{120, 100, 100}}, {[]string{"cyan", "turquoise", "teal"}, [3]int{180, 100, 100}},
	{[]string{"blue"}, [3]int{240, 100, 100}}, {[]string{"purple", "violet"}, [3]int{280, 100, 100}},
	{[]string{"magenta"}, [3]int{300, 100, 100}}, {[]string{"pink"}, [3]int{330, 100, 100}},
	{[]string{"white"}, [3]int{0, 0, 100}}, {[]string{"cold", "cool"}, [3]int{190, 10, 100}}, {[]string{"warm"}, [3]int{40, 30, 100}},
}

func hsl(h [3]int) map[string]any { return map[string]any{"0": h[0], "1": h[1], "2": h[2]} }

func colourOf(t string) ([3]int, bool) {
	for _, c := range colours {
		if hasPhrase(t, c.words...) {
			return c.hsl, true
		}
	}
	return [3]int{}, false
}

func ruleLight(orig, t string, tok []string) (nluResult, bool) {
	isLight := hasPhrase(t, "light", "lights", "lamp", "lamps", "lighting")
	if isLight && hasPhrase(t, "off") {
		return util("turn_off_light", "light", map[string]any{"light": map[string]any{"operation": "off", "room": ""}})
	}
	c, hasColour := colourOf(t)
	// a bare colour ("yellow") or a light request with a colour; plain "turn on the light" -> white
	pure := hasColour && !hasPhrase(t, "cold", "cool", "warm") // "I'm cold" is not a light request
	if hasColour && (isLight || (pure && len(tok) <= 2)) {
		return util("turn_on_light", "light", map[string]any{"light": map[string]any{"operation": "set_hsl", "hsl": hsl(c), "room": ""}})
	}
	if isLight && hasPhrase(t, "on", "turn on", "switch on") {
		return util("turn_on_light", "light", map[string]any{"light": map[string]any{"operation": "set_hsl", "hsl": hsl([3]int{0, 0, 100}), "room": ""}})
	}
	return nluResult{}, false
}

// ---- volume ----

func ruleVolume(orig, t string, tok []string) (nluResult, bool) {
	hasVol := hasPhrase(t, "volume", "louder", "quieter", "softer", "mute", "unmute")
	if !hasVol && !hasPhrase(t, "turn it up", "turn it down", "speak up") {
		return nluResult{}, false
	}
	op := ""
	switch {
	case hasPhrase(t, "mute"):
		op = "mute"
	case hasPhrase(t, "max", "maximum", "full", "loudest", "highest"):
		op = "max"
	case hasPhrase(t, "min", "minimum", "lowest", "quietest", "low"):
		op = "min"
	case hasPhrase(t, "medium", "mid", "middle", "half", "normal"):
		op = "mid"
	case hasPhrase(t, "up", "louder", "higher", "increase", "raise", "speak up"):
		op = "up"
	case hasPhrase(t, "down", "quieter", "softer", "lower", "decrease", "reduce"):
		op = "down"
	}
	if op == "" {
		return nluResult{}, false
	}
	return util("volume_to", "volume", map[string]any{"volume": map[string]any{"operation": op}})
}

// ---- timer / alarm ----

// clock reads a time of day as "HH:MM" from normalized tokens. ok=false if none found.
func clock(tok []string) (string, bool) {
	ampm := func(i int) string {
		if i < len(tok) {
			switch {
			case tok[i] == "a" && i+1 < len(tok) && tok[i+1] == "m", tok[i] == "am":
				return "am"
			case tok[i] == "p" && i+1 < len(tok) && tok[i+1] == "m", tok[i] == "pm":
				return "pm"
			}
		}
		return ""
	}
	fmtT := func(h, m int, ap string) (string, bool) {
		if ap == "pm" && h < 12 {
			h += 12
		}
		if ap == "am" && h == 12 {
			h = 0
		}
		if h < 0 || h > 23 || m < 0 || m > 59 {
			return "", false
		}
		return pad2(h) + ":" + pad2(m), true
	}
	for i := 0; i < len(tok); i++ {
		switch tok[i] {
		case "noon":
			return "12:00", true
		case "midnight":
			return "00:00", true
		case "half":
			if i+1 < len(tok) && tok[i+1] == "past" {
				if h, used := number(tok, i+2); used > 0 {
					return fmtT(h, 30, ampm(i+2+used))
				}
			}
		case "quarter":
			if i+2 < len(tok) && (tok[i+1] == "past" || tok[i+1] == "to") {
				if h, used := number(tok, i+2); used > 0 {
					if tok[i+1] == "past" {
						return fmtT(h, 15, ampm(i+2+used))
					}
					return fmtT((h+23)%24, 45, ampm(i+2+used))
				}
			}
		}
		// "7 30 pm", "7 pm", "seven thirty", "19 05"
		h, used := number(tok, i)
		if used == 0 || (used == 1 && h > 23) {
			continue
		}
		next := i + used
		if ap := ampm(next); ap != "" {
			return fmtT(h, 0, ap)
		}
		if m, u2 := number(tok, next); u2 > 0 && m <= 59 && (len(tok[next]) == 2 || u2 == 2 || m >= 10 || tok[next] == "oh") {
			if ap := ampm(next + u2); ap != "" || i > 0 && tok[i-1] == "at" {
				return fmtT(h, m, ap)
			}
		}
		if i > 0 && tok[i-1] == "at" && h <= 23 {
			return fmtT(h, 0, "")
		}
	}
	return "", false
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// ---- clock, date, battery, update ----

func ruleClockDate(orig, t string, tok []string) (nluResult, bool) {
	switch {
	case t == "time" || t == "the time" || hasPhrase(t, "what time is it", "what s the time", "what is the time", "tell me the time", "current time", "got the time"):
		return util("time", "time", map[string]any{"time": map[string]any{}})
	case t == "date" || t == "the date" || hasPhrase(t, "what s the date", "what is the date", "today s date", "what is today s date", "what date is it"):
		return util("date", "date", nil)
	case hasPhrase(t, "what day is it", "what day is today", "which day is it", "what s the day"):
		return util("day", "day", nil)
	}
	return nluResult{}, false
}

func ruleBattery(orig, t string, tok []string) (nluResult, bool) {
	if t == "battery" || hasPhrase(t, "battery level", "your battery", "how much battery", "how charged", "charge level", "battery status", "how s your battery") {
		return util("battery", "battery", nil)
	}
	return nluResult{}, false
}

func ruleUpdate(orig, t string, tok []string) (nluResult, bool) {
	if hasPhrase(t, "update homestation", "update home station", "update the homestation", "update the home station", "update my homestation") {
		return util("update_home_station", "update_home_station", nil)
	}
	if hasPhrase(t, "check for update", "check for updates", "is there an update", "any updates", "check update", "check updates", "software update") {
		return util("check_update", "update", map[string]any{"update": map[string]any{"operation": "check"}})
	}
	if hasPhrase(t, "install firmware", "install the latest firmware", "install latest firmware", "update to the latest firmware", "update firmware", "update yourself", "install the update", "install update", "install the firmware", "upgrade yourself") {
		return util("update_firmware", "update", map[string]any{"update": map[string]any{"operation": "update"}})
	}
	return nluResult{}, false
}

func ruleWeather(orig, t string, tok []string) (nluResult, bool) {
	// a request for the weather, not a conversation that merely mentions it ("tell me about the weather in the middle ages")
	direct := hasPhrase(t, "what s the weather", "what is the weather", "how s the weather", "tell me the weather", "what s the forecast")
	if len(tok) > 8 || (asksAbout(tok) && !direct) {
		return nluResult{}, false
	}
	if t == "weather" || direct || hasPhrase(t, "the weather", "weather forecast", "is it raining", "is it sunny", "is it cold outside", "is it hot outside", "weather today") {
		return nluResult{Intent: "weather", Behavior: "utilities", Paras: map[string]any{"utility_type": "weather"}, NeedWeather: true}, true
	}
	return nluResult{}, false
}

func ruleNews(orig, t string, tok []string) (nluResult, bool) {
	if t == "news" || hasPhrase(t, "what s new today", "whats new today", "what is new today", "the news", "latest news", "tell me the news", "read the news", "any news", "what s the news", "today s news", "news headlines", "headlines") {
		if len(tok) <= 7 && !asksAbout(tok) || t == "what s the news" || hasPhrase(t, "tell me the news", "what s new today", "whats new today", "what is new today") {
			return nluResult{Intent: "news", Behavior: "utilities", Paras: map[string]any{"utility_type": "news"}, NeedNews: true}, true
		}
	}
	return nluResult{}, false
}

func rulePhoto(orig, t string, tok []string) (nluResult, bool) {
	switch {
	case hasPhrase(t, "take a photo", "take a picture", "take a selfie", "take photo", "take picture", "photo please", "selfie"):
		return util("take_photo", "take_photo", nil)
	case hasPhrase(t, "what do you see", "what can you see", "what did you see", "what is this", "what s this", "what am i holding", "describe this", "recognize this", "identify this"):
		return util("take_photo_recog", "take_photo_recog", nil)
	case hasPhrase(t, "show me the photos", "show my photos", "show photos", "show me my pictures", "show the photos", "show me the pictures", "show pictures", "show the pictures"):
		return util("show_photo", "show_photo", nil)
	}
	return nluResult{}, false
}

// ---- games and music ----

var games = []struct {
	name    string
	phrases []string
}{
	{"angry_emo", []string{"angry emo"}},
	{"pig", []string{"100 points", "hundred points", "one hundred points", "play pig", "pig game"}},
	{"birthday", []string{"it s my birthday", "its my birthday", "today is my birthday", "it is my birthday"}},
	{"rps", []string{"rock paper scissors", "rock paper scissor", "play rps"}},
	{"shot", []string{"hands up", "freeze", "stick em up", "stick them up", "don t move", "do not move"}},
	{"flip_a_coin", []string{"flip a coin", "toss a coin", "heads or tails", "coin flip", "coin toss"}},
	{"ttt", []string{"tic tac toe", "noughts and crosses", "naughts and crosses", "play ttt",
		"tic tac 2", "tic tac two", "tic tac to", "tick tack toe", "tick tac toe"}}, // how speech-to-text has heard it on the owner's robot
	{"lucky_fruit", []string{"lucky fruit", "slot machine", "fruit machine"}},
	{"roll_a_dice", []string{"roll a dice", "roll the dice", "roll a die", "roll the die", "throw a dice", "throw the dice"}},
	{"fix_bugs", []string{"fix bugs", "fix the bugs", "fix some bugs", "debug"}},
	{"fireworks", []string{"fireworks", "firework", "fire works"}},
	{"zombie", []string{"zombie", "zombies"}},
	{"sticker", []string{"sticker", "stickers"}},
	{"battle", []string{"battle", "fight mode", "ready for action", "weapons", "get your weapon"}},
	{"exercise", []string{"exercise", "workout", "work out", "fitness"}},
	{"camping", []string{"camping", "campfire", "go camping"}},
	{"magic", []string{"magic trick", "do magic", "do a magic trick", "show me magic", "show your magic", "show me your magic"}},
	{"pose", []string{"strike a pose", "do a pose", "make a pose", "pose for me"}},
	{"drawing", []string{"drawing game", "let s draw", "lets draw", "draw with me", "show me your paintings", "show me your drawing", "show me your drawings", "show your paintings", "show me your painting"}},
	{"show_something", []string{"show me something", "show something", "surprise me", "do something cool", "show me a trick"}},
	{"snake", []string{"snake game", "play snake"}},
	{"paint", []string{"paint with me", "let s paint", "lets paint", "painting game", "put on your glasses", "put on glasses", "wear your glasses"}},
}

// asksAbout: the utterance starts like a question about a topic ("what is dance", "tell me about camping"), which is
// conversation, not a command to do that thing.
func asksAbout(tok []string) bool {
	if len(tok) == 0 {
		return false
	}
	switch tok[0] {
	case "what", "why", "how", "who", "when", "where", "which", "explain", "describe", "tell", "teach", "is", "are", "does", "do":
		// "do the dog" / "what does the fox say" style animal and "do a magic trick" requests are handled by their own rules
		return !(tok[0] == "do" && len(tok) <= 4)
	}
	return false
}

// commandish: a short utterance, or one that explicitly asks to play/start something. Keeps long conversational
// sentences ("tell me about the history of dance") out of the command rules and in free chat.
func commandish(t string, tok []string, max int) bool {
	return len(tok) <= max || hasPhrase(t, "play", "let s", "lets", "let me", "start", "can we", "i want to")
}

func ruleGames(orig, t string, tok []string) (nluResult, bool) {
	if hasPhrase(t, "what s your ability", "what is your ability", "what are your abilities", "what can you do", "what are you able to do") {
		return res("featured_game_show_something", "featured_game", map[string]any{"game_name": "show_something"})
	}
	if asksAbout(tok) {
		return nluResult{}, false
	}
	if !commandish(t, tok, 4) {
		return nluResult{}, false
	}
	for _, g := range games {
		if hasPhrase(t, g.phrases...) {
			return res("featured_game_"+g.name, "featured_game", map[string]any{"game_name": g.name})
		}
	}
	return nluResult{}, false
}

func ruleMusic(orig, t string, tok []string) (nluResult, bool) {
	if asksAbout(tok) {
		return nluResult{}, false
	}
	if len(tok) > 7 {
		return nluResult{}, false
	}
	bt := func(action string) (nluResult, bool) {
		return res("play_music", "play_music", map[string]any{"type": "bluetooth", "action": action})
	}
	// Bluetooth speaker mode and its transport controls (the stop/next words only mean this while it is on; the robot decides)
	switch {
	case hasPhrase(t, "bluetooth") && hasPhrase(t, "off", "exit", "stop", "disconnect", "leave", "close"):
		return bt("exit")
	case hasPhrase(t, "bluetooth") && hasPhrase(t, "connect", "pair", "enter", "on", "speaker", "start", "open"):
		return bt("enter")
	case hasPhrase(t, "start the music", "start music", "resume the music", "resume music"):
		return bt("start")
	case hasPhrase(t, "stop the music", "stop music", "pause the music", "pause music"):
		return bt("stop")
	case t == "next one" || t == "next song" || t == "next" || hasPhrase(t, "next track", "skip this song", "skip song", "play the next song"):
		return bt("next")
	}
	for _, in := range []struct{ word, action string }{{"piano", "piano"}, {"drums", "drum"}, {"drum", "drum"}, {"trumpet", "trumpet"}} {
		if hasPhrase(t, in.word) && hasPhrase(t, "play", "let s", "lets", "can you", "show") {
			return res("play_music", "play_music", map[string]any{"type": "instrument", "action": in.action})
		}
	}
	switch {
	case hasPhrase(t, "play music together", "play together", "make music together", "play instruments together"):
		return res("play_music", "play_music", map[string]any{"type": "instrument", "action": "dance_together"})
	case hasPhrase(t, "play an instrument", "play some music", "play instrument", "play music"):
		return res("play_music", "play_music", map[string]any{"type": "instrument", "action": "random"})
	case hasPhrase(t, "sing", "sing a song", "sing for me"):
		return res("play_music", "play_music", map[string]any{"type": "sing"})
	}
	return nluResult{}, false
}

func ruleDance(orig, t string, tok []string) (nluResult, bool) {
	if asksAbout(tok) {
		return nluResult{}, false
	}
	if len(tok) > 6 {
		return nluResult{}, false
	}
	switch {
	case hasPhrase(t, "dance to the music", "listen to the music", "dance to music", "dance to this", "dance to the song"):
		return res("listen_to_music", "dance", map[string]any{"dance_type": "to_music"})
	case hasPhrase(t, "dance together", "dance with me", "let s dance together"):
		return res("dance_together", "dance", map[string]any{"dance_type": "together"})
	case hasPhrase(t, "party", "disco", "party time", "dance with lights", "dance with the lights"):
		return res("dance_with_lights", "dance", map[string]any{"dance_type": "with_lights"})
	case hasPhrase(t, "dance", "dancing"):
		return res("dance", "dance", nil)
	}
	return nluResult{}, false
}

// ---- movement, presence, exploration ----

func ruleMove(orig, t string, tok []string) (nluResult, bool) {
	short := len(tok) <= 5
	switch {
	case hasPhrase(t, "go home", "go to your charger", "go to your dock", "go to the charger", "go to your home station", "return home", "back to your charger"):
		return res("go_home", "go_home", nil)
	case hasPhrase(t, "get off the charger", "go off charger", "go off the charger", "leave the charger", "get off your charger", "get off the dock"):
		return res("go_off_charger", "go_off_charger", nil)
	case hasPhrase(t, "turn around", "spin around", "turn back"):
		return res("turn_around", "basic_move", map[string]any{"type": "turn_around"})
	case short && hasPhrase(t, "turn left", "go left", "move left", "step left"):
		return res("left", "basic_move", map[string]any{"type": "left"})
	case short && hasPhrase(t, "turn right", "go right", "move right", "step right"):
		return res("right", "basic_move", map[string]any{"type": "right"})
	case short && hasPhrase(t, "forward", "go forward", "move forward", "walk forward", "come here", "come forward"):
		return res("forward", "basic_move", map[string]any{"type": "forward"})
	case short && hasPhrase(t, "backward", "backwards", "go back", "move back", "back up", "reverse"):
		return res("backward", "basic_move", map[string]any{"type": "backward"})
	}
	return nluResult{}, false
}

func ruleAboutEmo(orig, t string, tok []string) (nluResult, bool) {
	typ := ""
	switch {
	case hasPhrase(t, "how old are you", "your age", "what is your age", "what s your age", "how old are u", "how old r you"):
		typ = "age"
	case hasPhrase(t, "lucky number", "your lucky number"):
		typ = "lucky_number"
	case hasPhrase(t, "what s your name", "what is your name", "who are you", "your name", "may i know your name", "can i know your name"):
		typ = "about_name"
	case hasPhrase(t, "favorite color", "favourite colour", "favorite colour", "favourite color", "your color", "your colour", "color do you like", "colour do you like", "color you like best", "colour you like best"):
		typ = "color"
	case hasPhrase(t, "star sign", "zodiac", "your sign", "what sign are you"):
		typ = "sign"
	case hasPhrase(t, "when is your birthday", "your birthday", "when s your birthday", "your birthdate", "what s your birthdate", "when were you born"):
		typ = "birthday"
	case hasPhrase(t, "happy birthday"):
		typ = "happy_birthday"
	case hasPhrase(t, "hope you get better soon", "get well soon", "i will take care of you", "i ll take care of you", "i hope you feel better",
		"are you feeling better", "do you need comfort", "cheer up"):
		typ = "about_comfort"
	case hasPhrase(t, "are you ok", "are you okay", "how are you", "how do you feel", "how are you feeling", "how are you doing",
		"are you sick", "how is your health", "are you healthy", "are you feeling ok"):
		typ = "about_health"
	}
	if typ == "" {
		return nluResult{}, false
	}
	intent := typ
	if !strings.HasPrefix(typ, "about_") {
		intent = "about_" + typ
	}
	return res(intent, "about_emo", map[string]any{"type": typ})
}

func ruleSimple(orig, t string, tok []string) (nluResult, bool) {
	if len(tok) > 7 || asksAbout(tok) {
		return nluResult{}, false
	}
	switch {
	case len(tok) <= 5 && hasPhrase(t, "shut down", "shutdown", "power off", "power down", "turn off", "switch off", "turn yourself off"):
		return res("power_off", "power_off", nil)
	case t == "hi" || t == "hello" || t == "hey" || t == "hi emo" || t == "hello emo" || t == "hey emo" || t == "hi there" ||
		t == "greetings" || t == "howdy":
		return res("greeting", "play_animation", map[string]any{"animation_name": "Hi"})
	case hasPhrase(t, "explore", "look around", "go exploring", "go explore", "start exploring") && !hasPhrase(t, "stop", "do not", "don t"):
		return res("explore", "explore", nil)
	case t == "stop" || t == "quiet" || hasPhrase(t, "be quiet", "shut up", "shush", "shh", "silence", "enough", "stay still", "stand still",
		"keep still", "be still", "freeze emo", "hold still", "stop exploring", "stop moving"):
		return res("be_quiet", "stay_still", nil)
	case hasPhrase(t, "go to sleep", "go to bed", "time to sleep", "sleep now", "good night emo"):
		return res("sleep", "tree_sleep_and_wake_up", nil)
	case hasPhrase(t, "look at me", "see me", "can you see me", "find me", "where am i"):
		return res("look_at_me", "tree_search_and_interact", nil)
	case hasPhrase(t, "play by yourself", "go play", "play on your own", "entertain yourself", "amuse yourself", "play around"):
		return res("play_by_yourself", "play_around", map[string]any{"play_type": "normal"})
	case hasPhrase(t, "listen to", "listen up", "listen"):
		return res("listen_to_voice", "listen", nil)
	}
	return nluResult{}, false
}
