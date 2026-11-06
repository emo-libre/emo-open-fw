package main

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The English side of these pairs is the vendor's published voice-command list (used as a coverage checklist); the Vietnamese side
// was contributed by voduylinh. Every English command must be understood, and the Vietnamese version must mean exactly the same.

type pair struct{ en, vi string }

func loadPairs(t *testing.T) []pair { return loadPairsFrom(t, "testdata/vi_pairs.txt") }

func loadPairsFrom(t *testing.T, file string) []pair {
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []pair
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " => ", 2)
		if len(parts) != 2 {
			t.Fatalf("bad fixture line %q", line)
		}
		strip := strings.NewReplacer("{", "", "}", "")
		out = append(out, pair{strip.Replace(parts[0]), strip.Replace(parts[1])})
	}
	return out
}

func norm(r nluResult) map[string]any {
	b, _ := json.Marshal(map[string]any{"i": r.Intent, "b": r.Behavior, "p": r.Paras, "w": r.NeedWeather, "n": r.NeedNews})
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

// The two robot-to-robot questions have no known cloud reply, so no language answers them.
var unknownEverywhere = map[string]bool{"Do you know him?": true, "Do you know it?": true}

func testServer(t *testing.T) *server {
	t.Helper()
	ps, err := loadPacks("")
	if err != nil {
		t.Fatal(err)
	}
	return &server{packs: ps}
}

func TestEveryEnglishCommandIsUnderstood(t *testing.T) {
	s := testServer(t)
	pairs := loadPairs(t)
	if len(pairs) < 160 {
		t.Fatalf("only %d pairs", len(pairs))
	}
	for _, p := range pairs {
		r, ok := s.parse(p.en, "en")
		if unknownEverywhere[p.en] {
			if ok {
				t.Errorf("%q unexpectedly understood: %s", p.en, r.Intent)
			}
			continue
		}
		if !ok {
			t.Errorf("English %q not understood", p.en)
		}
	}
}

func TestVietnameseMeansTheSameAsEnglish(t *testing.T) {
	s := testServer(t)
	for _, p := range loadPairs(t) {
		if unknownEverywhere[p.en] {
			if r, ok := s.parse(p.vi, "vi"); ok {
				t.Errorf("%q unexpectedly understood as %s", p.vi, r.Intent)
			}
			continue
		}
		en, ok1 := s.parse(p.en, "en")
		vi, ok2 := s.parse(p.vi, "vi")
		if !ok1 || !ok2 {
			t.Errorf("%q / %q: english ok=%v vietnamese ok=%v", p.en, p.vi, ok1, ok2)
			continue
		}
		if !reflect.DeepEqual(norm(en), norm(vi)) {
			t.Errorf("%q => %q\n   english:    %v\n   vietnamese: %v", p.en, p.vi, norm(en), norm(vi))
		}
	}
}

// Without diacritics (what a sloppy transcript or keyboard gives) the Vietnamese still works.
func TestVietnameseWithoutAccents(t *testing.T) {
	s := testServer(t)
	for _, p := range loadPairs(t) {
		if unknownEverywhere[p.en] {
			continue
		}
		plain := foldDiacritics(p.vi)
		en, _ := s.parse(p.en, "en")
		vi, ok := s.parse(plain, "vi")
		if !ok || !reflect.DeepEqual(norm(en), norm(vi)) {
			t.Errorf("accents stripped: %q -> ok=%v %v, want %v", plain, ok, norm(vi), norm(en))
		}
	}
}

func TestVietnameseNumbersAndTimes(t *testing.T) {
	lx := testServer(t).packs["vi"].lex
	for in, want := range map[string]int{
		"không": 0, "một": 1, "năm": 5, "mười": 10, "mười một": 11, "mười lăm": 15, "hai mươi": 20, "hai mươi mốt": 21,
		"hai mươi lăm": 25, "ba mươi tư": 34, "chín mươi chín": 99, "một trăm": 100, "một trăm lẻ năm": 105, "hai trăm năm mươi": 250,
		"10": 10, "45": 45,
	} {
		tok := strings.Fields(lx.normalize(in))
		if n, used := lx.number(tok, 0); n != want || used != len(tok) {
			t.Errorf("number(%q) = %d (used %d of %d), want %d", in, n, used, len(tok), want)
		}
	}
	for in, want := range map[string]int{
		"mười giây": 10, "năm phút": 300, "một giờ": 3600, "hai tiếng": 7200, "một tiếng rưỡi": 5400, "nửa tiếng": 1800,
		"một phút ba mươi giây": 90, "hai mươi lăm giây": 25, "": 0, "chó mèo": 0,
	} {
		if got := lx.duration(strings.Fields(lx.normalize(in))); got != want {
			t.Errorf("duration(%q) = %d want %d", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"bảy giờ sáng": "07:00", "chín giờ tối": "21:00", "sáu giờ tối": "18:00", "bảy giờ rưỡi": "07:30", "mười hai giờ trưa": "12:00",
		"một giờ trưa": "13:00", "sáu giờ ba mươi chiều": "18:30", "mười hai giờ đêm": "00:00", "mười giờ đêm": "22:00", "3 giờ chiều": "15:00",
		"hai giờ sáng": "02:00",
	} {
		got, ok := lx.clock(strings.Fields(lx.normalize(in)))
		if !ok || got != want {
			t.Errorf("clock(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := lx.clock(strings.Fields(lx.normalize("ngày mai"))); ok {
		t.Error("clock found in 'ngày mai'")
	}
}

func TestVietnameseCityNameAndBirthday(t *testing.T) {
	s := testServer(t)
	r, ok := s.parse("Thời tiết ở Hà Nội thế nào?", "vi")
	if !ok || !r.NeedWeather || r.City != "Hà Nội" {
		t.Fatalf("%+v %v", r, ok)
	}
	if r, _ := s.parse("thời tiết tại Đà Nẵng hôm nay", "vi"); r.City != "Đà Nẵng" {
		t.Fatalf("%+v", r)
	}
	if r, _ := s.parse("Thời tiết hôm nay thế nào?", "vi"); !r.NeedWeather || r.City != "" {
		t.Fatalf("no city should mean the robot's own: %+v", r)
	}
	if r, _ := s.parse("Mình tên là Nguyễn Văn An", "vi"); r.Paras["name"] != "Nguyễn Văn An" {
		t.Fatalf("%+v", r)
	}
	if r, ok := s.parse("Mình tên là một hai ba bốn năm sáu", "vi"); ok && r.Intent == "my_name_is" {
		t.Fatalf("a long utterance is not a name: %+v", r)
	}
	if r, _ := s.parse("sinh nhật mình là ngày hai mươi lăm tháng mười hai", "vi"); r.Paras["birth_date"] != "12.25" {
		t.Fatalf("%+v", r)
	}
	// English too
	e := englishLex
	if r, _ := e.ruleWeatherCity("What's the weather in New York today?", normalizeText("What's the weather in New York today?"),
		words(normalizeText("What's the weather in New York today?"))); r.City != "New York" {
		t.Fatalf("%+v", r)
	}
}

func TestVietnameseAlarmsInDetail(t *testing.T) {
	s := testServer(t)
	for say, want := range map[string]string{
		"đặt báo thức lúc sáu giờ rưỡi sáng mỗi ngày": `{"alarm":{"operation":"set","recurrence":"ED","tag":"alarm","time":"06:30"},"utility_type":"alarm"}`,
		"đặt báo thức lúc mười một giờ đêm thứ hai":   `{"alarm":{"operation":"set","recurrence":"MON","tag":"alarm","time":"23:00"},"utility_type":"alarm"}`,
		"tạo báo thức uống thuốc lúc tám giờ sáng":    `{"alarm":{"operation":"set","recurrence":"ONCE","tag":"pill","time":"08:00"},"utility_type":"alarm"}`,
		"xóa báo thức thứ ba":                         `{"alarm":{"index":"3","operation":"remove"},"utility_type":"alarm"}`,
		"tắt báo thức thứ hai":                        `{"alarm":{"index":"2","operation":"update","schedule_on":0},"utility_type":"alarm"}`,
		"bật báo thức đầu tiên":                       `{"alarm":{"index":"1","operation":"update","schedule_on":1},"utility_type":"alarm"}`,
		"hẹn giờ năm phút":                            `{"timer":{"duration":300,"operation":"set"},"utility_type":"timer"}`,
		"hẹn giờ một tiếng rưỡi":                      `{"timer":{"duration":5400,"operation":"set"},"utility_type":"timer"}`,
		"hủy hẹn giờ":                                 `{"timer":{"operation":"cancel"},"utility_type":"timer"}`,
		"Đổi màu mắt thành màu xanh lá":               `{"customize":{"0":120,"1":100,"2":100},"utility_type":"eye_color"}`,
	} {
		r, ok := s.parse(say, "vi")
		if !ok {
			t.Errorf("%q not understood", say)
			continue
		}
		b, _ := json.Marshal(r.Paras)
		var got, exp map[string]any
		json.Unmarshal(b, &got)
		json.Unmarshal([]byte(want), &exp)
		if r.Intent == "eye_color" {
			exp["utility_type"] = "customize"
		}
		if !subsetEqual(exp, got) {
			t.Errorf("%q -> %s, want ⊇ %s", say, b, want)
		}
	}
}

func TestVietnameseDoesNotHijackConversation(t *testing.T) {
	s := testServer(t)
	for _, say := range []string{
		"Hôm nay trời đẹp quá em nhỉ", "Kể cho mình nghe một câu chuyện về con rồng", "Em có thích ăn kem không", "Con chó nhà mình rất dễ thương",
		"Mình muốn học nấu ăn món phở", "Tại sao bầu trời lại màu xanh", "Hôm qua mình đi chơi công viên với bạn", "Nhạc này hay quá",
	} {
		if r, ok := s.parse(say, "vi"); ok {
			t.Errorf("%q was hijacked: %s %v", say, r.Intent, r.Paras)
		}
	}
}

// New English behaviour beyond the pairs.
func TestEnglishExtras(t *testing.T) {
	s := testServer(t)
	for say, want := range map[string]string{
		"Set eye color to purple": "eye_color", "set eye colour to default": "eye_color", "what does the frog say": "animal",
		"act like a tiger": "animal", "what is a duck like": "animal", "turn on the second alarm": "alarm_update", "remove the third alarm": "alarm_remove",
		"set a timer for twenty five minutes": "timer_set", "stop the timer": "timer_cancel", "what's the weather in Paris": "weather",
		"good night": "good_night", "see you later": "goodbye", "draw me as an anime girl": "featured_game_aidraw",
	} {
		r, ok := s.parse(say, "en")
		if !ok || r.Intent != want {
			t.Errorf("%q -> %q ok=%v, want %s", say, r.Intent, ok, want)
		}
	}
	if r, _ := s.parse("What's the weather in Paris?", "en"); r.City != "Paris" {
		t.Errorf("city %q", r.City)
	}
	if r, _ := s.parse("Set eye color to default", "en"); r.Paras["customize"] == nil {
		t.Errorf("%v", r.Paras)
	}
	if r, _ := s.parse("what does the frog say", "en"); r.Paras["animation_name"] != "Fog" {
		t.Errorf("%v", r.Paras)
	}
	// every one of the 16 animals
	for _, a := range animalKeys {
		r, ok := s.parse("what does the "+a+" say", "en")
		if !ok || r.Paras["animation_name"] != animalAnim(a) {
			t.Errorf("animal %s -> %v ok=%v", a, r.Paras, ok)
		}
	}
	// sentences that merely contain these words stay conversation
	for _, say := range []string{"my dog is called max and he is very old", "tell me about the weather in the middle ages",
		"I am very tired today because I woke up early", "the matrix is a good film", "do you think christmas is too commercial these days"} {
		if r, ok := s.parse(say, "en"); ok {
			t.Errorf("%q hijacked: %s", say, r.Intent)
		}
	}
}
