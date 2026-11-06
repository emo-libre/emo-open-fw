package main

import "strings"

// The built-in English lexicon, and the parts of the English command set that are not simple phrase lists.

var englishLex = newEnglishLex()

func newEnglishLex() *lexicon {
	nums := map[string]int{}
	for w, v := range numWords {
		if w != "a" && w != "an" {
			nums[w] = v
		}
	}
	cols := map[string][]int{}
	for _, c := range colours {
		for _, w := range c.words {
			cols[w] = []int{c.hsl[0], c.hsl[1], c.hsl[2]}
		}
	}
	ord := map[string]int{"first": 1, "1st": 1, "second": 2, "2nd": 2, "third": 3, "3rd": 3, "fourth": 4, "4th": 4, "fifth": 5,
		"5th": 5, "sixth": 6, "6th": 6, "seventh": 7, "7th": 7, "eighth": 8, "8th": 8, "ninth": 9, "9th": 9, "tenth": 10, "10th": 10}
	animals := map[string]string{}
	for _, a := range animalKeys {
		animals[a] = a
	}
	animals["cattle"], animals["rooster"], animals["hen"], animals["kitten"], animals["puppy"] = "cow", "chicken", "chicken", "cat", "dog"
	lx := &lexicon{
		Lang: "en", Numbers: nums,
		SecUnits: []string{"second", "seconds", "sec", "secs"}, MinUnits: []string{"minute", "minutes", "min", "mins"},
		HourUnits: []string{"hour", "hours", "hr", "hrs"}, HalfWords: []string{"half"}, OneWords: []string{"a", "an"},
		TimerWords: []string{"timer", "countdown"}, TimerCancel: []string{"cancel", "stop", "delete", "clear", "remove", "turn off"},
		AlarmWords: []string{"alarm", "alarms", "wake me up", "wake me"}, RemindWords: []string{"remind me", "reminder"},
		RemoveWords: []string{"remove", "delete", "cancel", "clear"}, CheckWords: []string{"check", "what", "when", "which", "do i have", "list", "show"},
		OnWords: []string{"on", "turn on", "switch on", "enable"}, OffWords: []string{"off", "turn off", "switch off", "disable"},
		AllWords: []string{"all", "every"}, StopWords: []string{"stop", "silence", "quiet", "shut up"},
		EveryDay:   []string{"every day", "everyday", "daily", "every morning", "every night", "every evening"},
		WeekdayPhr: []string{"weekdays", "every weekday", "on weekdays"}, WeekendPhr: []string{"weekend", "weekends", "every weekend"},
		Weekdays: map[string]string{"sunday": "SUN", "monday": "MON", "tuesday": "TUE", "wednesday": "WED", "thursday": "THU",
			"friday": "FRI", "saturday": "SAT", "sundays": "SUN", "mondays": "MON", "tuesdays": "TUE", "wednesdays": "WED",
			"thursdays": "THU", "fridays": "FRI", "saturdays": "SAT"},
		Tags: map[string]string{"pill": "pill", "pills": "pill", "medicine": "pill", "medication": "pill", "vitamin": "pill", "vitamins": "pill",
			"eat": "eat", "dinner": "eat", "lunch": "eat", "breakfast": "eat", "meal": "eat", "supper": "eat",
			"gym": "gym", "workout": "gym", "exercise": "gym", "training": "gym", "sleep": "sleep", "bedtime": "sleep", "bed": "sleep"},
		Ordinals: ord, LightWords: []string{"light", "lights", "lamp", "lamps", "lighting"},
		EyeWords:     []string{"eye color", "eye colour", "eyes color", "eyes colour", "color of your eyes", "colour of your eyes", "your eyes", "eyes"},
		DefaultWords: []string{"default", "normal", "original"}, Colours: cols,
		WeatherWords: []string{"weather", "forecast"}, InWords: []string{"in", "for"}, CityTrail: []string{"today", "tomorrow", "now", "tonight", "please", "right now", "like", "this week"}, Animals: animals,
	}
	lx.norm = normalizeText
	return lx.compile()
}

// ---- animals ----

var animalKeys = []string{"pig", "sheep", "snake", "tiger", "wolf", "cat", "cow", "chicken", "cicada", "cuckoo", "dog", "duck",
	"elephant", "frog", "horse", "fox"}

// animalAnim is the animation name the robot is asked to play. "Cattle" for the cow is known from the documented intent
// replies; the other names are the documented file names with a capital letter [V]. (The documentation spells the frog's
// file name "fog"; we follow it.)
func animalAnim(key string) string {
	switch key {
	case "cow":
		return "Cattle"
	case "frog":
		return "Fog"
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

func ruleAnimals(orig, t string, tok []string) (nluResult, bool) {
	if len(tok) > 8 {
		return nluResult{}, false
	}
	a, ok := longestKey(t, englishLex.Animals)
	if !ok {
		return nluResult{}, false
	}
	if len(tok) <= 2 || hasPhrase(t, "sound", "be a", "act like", "like a", "what does the", "what does a", "what is a", "what s a", "do the", "make the", "imitate", "impression", "make a") {
		return res("animal", "play_animation", map[string]any{"animation_name": animalAnim(a)})
	}
	return nluResult{}, false
}

// ---- AI painting: "draw me as an anime boy", "anime woman" ----

func ruleAIDraw(orig, t string, tok []string) (nluResult, bool) {
	if len(tok) > 8 || !hasPhrase(t, "anime") {
		return nluResult{}, false
	}
	style := ""
	switch {
	case hasPhrase(t, "boy", "man", "male", "guy"):
		style = "anime_boy"
	case hasPhrase(t, "girl", "woman", "female", "lady"):
		style = "anime_girl"
	default:
		return nluResult{}, false
	}
	return res("featured_game_aidraw", "featured_game", map[string]any{"game_name": "aidraw", "draw_style": style})
}
