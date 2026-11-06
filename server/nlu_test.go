package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// want: intent, behavior, and a subset of paras that must match (JSON-compared, so numbers are numbers).
type nluCase struct {
	say, intent, behavior string
	paras                 string // JSON object; every key must be present and equal
}

var nluCases = []nluCase{
	// greetings, simple commands, presence
	{"Hi", "greeting", "play_animation", `{"animation_name":"Hi"}`},
	{"hello EMO!", "greeting", "play_animation", `{"animation_name":"Hi"}`},
	{"good morning", "good_morning", "play_animation", `{"animation_name":"Good_morning"}`},
	{"shut down", "power_off", "power_off", `{}`},
	{"Shutdown", "power_off", "power_off", `{}`},
	{"power down", "power_off", "power_off", `{}`},
	{"explore", "explore", "explore", `{}`},
	{"go and look around", "explore", "explore", `{}`},
	{"stop", "be_quiet", "stay_still", `{}`},
	{"be quiet", "be_quiet", "stay_still", `{}`},
	{"look at me", "look_at_me", "tree_search_and_interact", `{}`},
	{"play by yourself", "play_by_yourself", "play_around", `{"play_type":"normal"}`},
	{"listen to", "listen_to_voice", "listen", `{}`},
	// dancing
	{"let's dance", "dance", "dance", `{}`},
	{"party", "dance_with_lights", "dance", `{"dance_type":"with_lights"}`},
	{"dance to the music", "listen_to_music", "dance", `{"dance_type":"to_music"}`},
	{"listen to the music", "listen_to_music", "dance", `{"dance_type":"to_music"}`},
	// movement
	{"forward", "forward", "basic_move", `{"type":"forward"}`},
	{"turn right", "right", "basic_move", `{"type":"right"}`},
	{"turn left", "left", "basic_move", `{"type":"left"}`},
	{"turn around", "turn_around", "basic_move", `{"type":"turn_around"}`},
	{"go back", "backward", "basic_move", `{"type":"backward"}`},
	{"go home", "go_home", "go_home", `{}`},
	{"get off the charger", "go_off_charger", "go_off_charger", `{}`},
	// about me
	{"how old are you", "about_age", "about_emo", `{"type":"age"}`},
	{"what's your lucky number", "about_lucky_number", "about_emo", `{"type":"lucky_number"}`},
	{"what's your name", "about_name", "about_emo", `{"type":"about_name"}`},
	{"when is your birthday", "about_birthday", "about_emo", `{"type":"birthday"}`},
	{"are you ok", "about_health", "about_emo", `{"type":"about_health"}`},
	// names and birthdays
	{"I'm Jack.", "my_name_is", "tree_search_and_update_name", `{"name":"Jack"}`},
	{"that's Poppy", "my_name_is", "tree_search_and_update_name", `{"name":"Poppy"}`},
	{"my name is rush", "my_name_is", "tree_search_and_update_name", `{"name":"Rush"}`},
	{"call me alex", "my_name_is", "tree_search_and_update_name", `{"name":"Alex"}`},
	{"what's my name", "who_am_i", "tree_search_and_update_name", `{}`},
	{"my birthday is March 3", "my_birthday_is", "tree_search_and_update_name", `{"birth_date":"3.3"}`},
	{"I was born on 14 July", "my_birthday_is", "tree_search_and_update_name", `{"birth_date":"7.14"}`},
	{"my birthday is 12.25", "my_birthday_is", "tree_search_and_update_name", `{"birth_date":"12.25"}`},
	// small-talk answers carry no behaviour
	{"yes", "smalltalk_yes", "", `{}`},
	{"yeah", "smalltalk_yes", "", `{}`},
	{"no", "smalltalk_no", "", `{}`},
	{"nope", "smalltalk_no", "", `{}`},
	{"5", "user_choose_number", "", `{}`},
	{"seven", "user_choose_number", "", `{}`},
	{"I play at 3", "user_choose_number", "", `{}`},
	// utilities
	{"what time is it", "time", "utilities", `{"utility_type":"time","time":{}}`},
	{"the time", "time", "utilities", `{"utility_type":"time"}`},
	{"date", "date", "utilities", `{"utility_type":"date"}`},
	{"what day is it", "day", "utilities", `{"utility_type":"day"}`},
	{"battery", "battery", "utilities", `{"utility_type":"battery"}`},
	{"how much battery do you have", "battery", "utilities", `{"utility_type":"battery"}`},
	{"check for updates", "check_update", "utilities", `{"utility_type":"update","update":{"operation":"check"}}`},
	{"install firmware", "update_firmware", "utilities", `{"update":{"operation":"update"}}`},
	{"set a timer for 10 seconds", "timer_set", "utilities", `{"utility_type":"timer","timer":{"operation":"set","duration":10}}`},
	{"timer for five minutes", "timer_set", "utilities", `{"timer":{"duration":300}}`},
	{"set a timer for 1 hour 30 minutes", "timer_set", "utilities", `{"timer":{"duration":5400}}`},
	{"timer for half an hour", "timer_set", "utilities", `{"timer":{"duration":1800}}`},
	{"set a timer for an hour", "timer_set", "utilities", `{"timer":{"duration":3600}}`},
	{"set an alarm at 7 a.m. every Tuesday", "alarm_set", "utilities", `{"utility_type":"alarm","alarm":{"operation":"set","time":"07:00","recurrence":"TUE","tag":"alarm"}}`},
	{"remind me to eat at 8 p.m. every Friday", "alarm_set", "utilities", `{"alarm":{"operation":"set","time":"20:00","recurrence":"FRI","tag":"eat"}}`},
	{"wake me up at 6:30 am on weekdays", "alarm_set", "utilities", `{"alarm":{"time":"06:30","recurrence":"WDAY"}}`},
	{"set an alarm for seven thirty pm every day", "alarm_set", "utilities", `{"alarm":{"time":"19:30","recurrence":"ED"}}`},
	{"alarm at half past six", "alarm_set", "utilities", `{"alarm":{"time":"06:30","recurrence":"ONCE"}}`},
	{"set an alarm at noon", "alarm_set", "utilities", `{"alarm":{"time":"12:00"}}`},
	{"remind me to take my pills at 9 pm", "alarm_set", "utilities", `{"alarm":{"time":"21:00","tag":"pill"}}`},
	{"check my alarms", "alarm_check", "utilities", `{"alarm":{"operation":"check"}}`},
	{"stop the alarm", "be_quiet", "stay_still", `{}`},
	{"remove alarm", "alarm_remove", "utilities", `{"alarm":{"operation":"remove","index":""}}`},
	{"delete alarm 2", "alarm_remove", "utilities", `{"alarm":{"operation":"remove","index":"2"}}`},
	{"turn off the light", "turn_off_light", "utilities", `{"utility_type":"light","light":{"operation":"off","room":""}}`},
	{"yellow", "turn_on_light", "utilities", `{"light":{"operation":"set_hsl","hsl":{"0":50,"1":100,"2":100},"room":""}}`},
	{"pink light", "turn_on_light", "utilities", `{"light":{"hsl":{"0":330,"1":100,"2":100}}}`},
	{"make the light cold", "turn_on_light", "utilities", `{"light":{"hsl":{"0":190,"1":10,"2":100}}}`},
	{"turn on the light", "turn_on_light", "utilities", `{"light":{"operation":"set_hsl","hsl":{"0":0,"1":0,"2":100}}}`},
	{"volume up", "volume_to", "utilities", `{"utility_type":"volume","volume":{"operation":"up"}}`},
	{"louder", "volume_to", "utilities", `{"volume":{"operation":"up"}}`},
	{"volume down", "volume_to", "utilities", `{"volume":{"operation":"down"}}`},
	{"set the volume to maximum", "volume_to", "utilities", `{"volume":{"operation":"max"}}`},
	{"volume low", "volume_to", "utilities", `{"volume":{"operation":"min"}}`},
	{"medium volume", "volume_to", "utilities", `{"volume":{"operation":"mid"}}`},
	{"mute", "volume_to", "utilities", `{"volume":{"operation":"mute"}}`},
	{"take a photo", "take_photo", "utilities", `{"utility_type":"take_photo"}`},
	{"what do you see", "take_photo_recog", "utilities", `{"utility_type":"take_photo_recog"}`},
	{"show me the photos", "show_photo", "utilities", `{"utility_type":"show_photo"}`},
	{"what's the weather", "weather", "utilities", `{"utility_type":"weather"}`},
	// games and music
	{"let's play tic tac toe", "featured_game_ttt", "featured_game", `{"game_name":"ttt"}`},
	{"hands up", "featured_game_shot", "featured_game", `{"game_name":"shot"}`},
	{"fix bugs", "featured_game_fix_bugs", "featured_game", `{"game_name":"fix_bugs"}`},
	{"let's play angry emo", "featured_game_angry_emo", "featured_game", `{"game_name":"angry_emo"}`},
	{"rock paper scissors", "featured_game_rps", "featured_game", `{"game_name":"rps"}`},
	{"flip a coin", "featured_game_flip_a_coin", "featured_game", `{"game_name":"flip_a_coin"}`},
	{"roll the dice", "featured_game_roll_a_dice", "featured_game", `{"game_name":"roll_a_dice"}`},
	{"fireworks", "featured_game_fireworks", "featured_game", `{"game_name":"fireworks"}`},
	{"let's chat", "chatgpt", "featured_game", `{"game_name":"chatgpt"}`},
	{"talk to me", "chatgpt", "featured_game", `{"game_name":"chatgpt"}`},
	{"play the piano", "play_music", "play_music", `{"type":"instrument","action":"piano"}`},
	{"play the drums", "play_music", "play_music", `{"type":"instrument","action":"drum"}`},
	{"play some music", "play_music", "play_music", `{"type":"instrument","action":"random"}`},
	{"sing a song", "play_music", "play_music", `{"type":"sing"}`},
	{"connect bluetooth", "play_music", "play_music", `{"type":"bluetooth","action":"enter"}`},
	// animals
	{"fox", "animal", "play_animation", `{"animation_name":"Fox"}`},
	{"what does the cow say", "animal", "play_animation", `{"animation_name":"Cattle"}`},
	{"be a snake", "animal", "play_animation", `{"animation_name":"Snake"}`},
	{"make a dog sound", "animal", "play_animation", `{"animation_name":"Dog"}`},
}

func TestNLUCatalogue(t *testing.T) {
	for _, c := range nluCases {
		got, ok := testServer(t).parse(c.say, "en")
		if !ok {
			t.Errorf("%q: not recognised", c.say)
			continue
		}
		if got.Intent != c.intent || got.Behavior != c.behavior {
			t.Errorf("%q -> %s/%s, want %s/%s", c.say, got.Intent, got.Behavior, c.intent, c.behavior)
			continue
		}
		var want map[string]any
		if err := json.Unmarshal([]byte(c.paras), &want); err != nil {
			t.Fatalf("bad case params %q: %v", c.paras, err)
		}
		// round-trip through JSON so int/float differences do not matter
		raw, _ := json.Marshal(got.Paras)
		var have map[string]any
		json.Unmarshal(raw, &have)
		if !subsetEqual(want, have) {
			t.Errorf("%q paras = %s, want ⊇ %s", c.say, raw, c.paras)
		}
	}
}

func subsetEqual(want, have any) bool {
	wm, ok := want.(map[string]any)
	if !ok {
		return reflect.DeepEqual(want, have)
	}
	hm, ok := have.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range wm {
		if !subsetEqual(v, hm[k]) {
			return false
		}
	}
	return true
}

// Ordinary conversation must NOT be hijacked by the command rules.
func TestNLULeavesConversationAlone(t *testing.T) {
	for _, say := range []string{
		"tell me about the history of dance", "what is the capital of france", "how do I shut down my computer properly",
		"can you tell me a story about a dragon", "that's nice", "I am human", "I'm cold", "my cat is very cute",
		"why is the sky blue", "the battle of hastings was in ten sixty six", "what is the best way to learn the piano",
		"tell me about camping in the mountains last summer", "who won the match yesterday", "right", "left", "it is raining a lot",
		"abundance of cheese", "i am hungry", "i am a little hungry today", "please explain how timers work in an oven",
		"do you like magic tricks and zombie movies", "I was born in a small town", "how old is the universe",
		// short questions about a topic: conversation, not a command to do the thing
		"what is dance", "tell me about camping", "why do robots dance", "how do you play the piano", "what is a zombie",
		"who invented fireworks", "explain exercise", "what is a coin flip",
		// long sentences that merely contain a trigger phrase
		"I think we should all just be quiet when the baby sleeps tonight",
		"my brother plays the piano in a band and I like to play along sometimes",
		"after the party last weekend everyone was too tired to dance anymore at all",
	} {
		if r, ok := testServer(t).parse(say, "en"); ok {
			t.Errorf("%q was hijacked: %s/%s %v", say, r.Intent, r.Behavior, r.Paras)
		}
	}
	if _, ok := parseIntent(""); ok {
		t.Error("empty text matched")
	}
	if _, ok := parseIntent("timer"); ok { // a timer with no duration is not a timer
		t.Error("timer without duration matched")
	}
	if _, ok := parseIntent("set an alarm"); ok { // an alarm with no time is not an alarm
		t.Error("alarm without time matched")
	}
}

func TestNLUWeatherNeedsBackend(t *testing.T) {
	r, ok := parseIntent("how's the weather")
	if !ok || !r.NeedWeather || r.Behavior != "utilities" {
		t.Fatalf("%v %v", r, ok)
	}
}

func TestClockParsing(t *testing.T) {
	for in, want := range map[string]string{
		"at 7 a m": "07:00", "at 7 p m": "19:00", "at 12 a m": "00:00", "at 12 p m": "12:00", "at 6 30 a m": "06:30",
		"at seven thirty p m": "19:30", "at 19 45": "19:45", "at quarter past 7": "07:15", "quarter to 8": "07:45",
		"at half past eleven p m": "23:30", "at midnight": "00:00", "at noon": "12:00", "at 9": "09:00",
	} {
		got, ok := clock(words(in))
		if !ok || got != want {
			t.Errorf("clock(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"hello there", "in a bit", "at 25", "sometime"} {
		if g, ok := clock(words(in)); ok {
			t.Errorf("clock(%q) = %q, want none", in, g)
		}
	}
}

func TestDurationParsing(t *testing.T) {
	for in, want := range map[string]int{
		"10 seconds": 10, "5 minutes": 300, "an hour": 3600, "half an hour": 1800, "1 hour 30 minutes": 5400,
		"two minutes thirty seconds": 150, "twenty five seconds": 25, "ninety seconds": 90, "a minute": 60, "": 0, "five apples": 0,
	} {
		if got := englishLex.duration(words(in)); got != want {
			t.Errorf("duration(%q) = %d want %d", in, got, want)
		}
	}
}

// "ChatGPT mode" as the owner's robot actually transcribed it (capture of 2026-11-05), plus clean forms; and lookalikes that must stay conversation.
func TestChatModeAsHeard(t *testing.T) {
	s := testServer(t)
	for _, say := range []string{
		"ChatGPT mode.", "chat GPT mode", "Chat mode.", "chat GPP mode.", "Jack Cheepity Mode.", "chat qbt mode.", "GPD mode.",
		"Enter chat mode", "Hey EMO, start ChatGPT mode.", "switch to chat g p t mode", "talk mode", "let's go into chat mode",
	} {
		if r, ok := s.parse(say, "en"); !ok || r.Intent != "chatgpt" || r.Paras["game_name"] != "chatgpt" {
			t.Errorf("%q: got %q (ok=%v), want chat mode", say, r.Intent, ok)
		}
	}
	for _, say := range []string{
		"what mode are you in", "check mode", "jack mode", "the chat was fun", "my phone is in silent mode",
		"chat about the weather mode tomorrow please now", "mode",
	} {
		if r, ok := s.parse(say, "en"); ok && r.Intent == "chatgpt" {
			t.Errorf("%q: wrongly started chat mode", say)
		}
	}
}

// The speech-to-text hint carries the command names (English) and the owner's own phrases for the robot's language, capped.
func TestSTTHint(t *testing.T) {
	s := testServer(t)
	cs, err := parseCustom([]byte(`{"commands":[{"name":"party","phrases":["Disco Inferno","party mode"],"say":"Woo"},{"name":"hallo","lang":"de","phrases":["Tanzparty"],"say":"Ja"}]}`), false)
	if err != nil {
		t.Fatal(err)
	}
	s.custom = &customSet{cmds: cs}
	en := s.sttHint("en")
	for _, want := range []string{"Tic Tac Toe", "ChatGPT mode", "Disco Inferno", "party mode"} {
		if !strings.Contains(en, want) {
			t.Errorf("en hint lacks %q: %s", want, en)
		}
	}
	if strings.Contains(en, "Tanzparty") || len(en) > maxHintChars {
		t.Errorf("en hint: %d chars, %s", len(en), en)
	}
	de := s.sttHint("de")
	if strings.Contains(de, "Tic Tac Toe") || !strings.Contains(de, "Tanzparty") || !strings.Contains(de, "Disco Inferno") {
		t.Errorf("de hint: %s", de)
	}
	var many []string
	for i := 0; i < 200; i++ {
		many = append(many, fmt.Sprintf(`"phrase number %d"`, i))
	}
	cs, _ = parseCustom([]byte(`{"commands":[{"name":"lots","phrases":[`+strings.Join(many[:60], ",")+`],"say":"x"}]}`), false)
	s.custom = &customSet{cmds: cs}
	if h := s.sttHint("en"); len(h) > maxHintChars || !strings.HasSuffix(h, ".") {
		t.Errorf("hint not capped: %d", len(h))
	}
}

// Tic tac toe as speech-to-text wrote it on the owner's robot ("Let's play Tic Tac 2.") and other common spellings.
func TestTicTacToeAsHeard(t *testing.T) {
	s := testServer(t)
	for _, say := range []string{"Let's play Tic Tac 2.", "let's play tic tac two", "Let's play tic-tac-toe.", "tick tack toe", "Rock Paper Scissors,"} {
		if r, ok := s.parse(say, "en"); !ok || r.Behavior != "featured_game" {
			t.Errorf("%q: got %q (ok=%v), want a featured game", say, r.Intent, ok)
		}
	}
}
