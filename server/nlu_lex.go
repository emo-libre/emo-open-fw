package main

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// A lexicon describes how one language writes the parts of a command that carry parameters: numbers, durations, clock times,
// weekdays, colours, ordinals, cities, names, dates. One generic engine then understands timers, alarms, light and eye colours,
// weather-for-a-city, "my name is…", birthdays and "I choose 5" in any language that has a lexicon. English has a built-in one;
// Vietnamese ships embedded; more can be added in a pack file under the key "_lex" (see server/README.md).

type lexicon struct {
	Lang string `json:"-"`

	Fold        bool           `json:"fold"`         // compare without diacritics (robust to speech-to-text variation)
	Numbers     map[string]int `json:"numbers"`      // word -> value for 0..90 (and teens where the language has them)
	NumberStyle string         `json:"number_style"` // "" = word or "tens unit"; "vi" = Vietnamese compounds

	SecUnits   []string `json:"sec_units"`
	MinUnits   []string `json:"min_units"`
	HourUnits  []string `json:"hour_units"`
	OneWords   []string `json:"one_words"`   // articles read as 1 before a unit ("a minute", "an hour")
	HalfWords  []string `json:"half_words"`  // "half" / Vietnamese "rưỡi", "nửa"
	ClockWords []string `json:"clock_words"` // word that follows the hour in a clock time ("giờ"); empty = not used

	AmWords    []string `json:"am_words"`
	PmWords    []string `json:"pm_words"`
	NoonWords  []string `json:"noon_words"`  // around midday
	NightWords []string `json:"night_words"` // late evening / small hours

	TimerWords  []string `json:"timer_words"`
	TimerCancel []string `json:"timer_cancel_words"`
	AlarmWords  []string `json:"alarm_words"`
	RemindWords []string `json:"remind_words"`
	RemoveWords []string `json:"remove_words"`
	CheckWords  []string `json:"check_words"`
	OnWords     []string `json:"on_words"`
	OffWords    []string `json:"off_words"`
	AllWords    []string `json:"all_words"`
	StopWords   []string `json:"stop_words"` // "stop the alarm": silence a ringing alarm, never delete it

	EveryDay   []string          `json:"every_day"`
	WeekdayPhr []string          `json:"weekday_phrases"` // Monday to Friday
	WeekendPhr []string          `json:"weekend_phrases"`
	Weekdays   map[string]string `json:"weekdays"` // phrase -> SUN..SAT
	Tags       map[string]string `json:"tags"`     // phrase -> alarm tag (gym, eat, sleep, pill)
	Ordinals   map[string]int    `json:"ordinals"` // phrase -> 1..

	LightWords   []string         `json:"light_words"`
	EyeWords     []string         `json:"eye_words"`
	DefaultWords []string         `json:"default_words"` // "default" colour for the eyes
	Colours      map[string][]int `json:"colours"`       // phrase -> [h, s, l]

	WeatherWords []string `json:"weather_words"`
	InWords      []string `json:"in_words"`      // "in", "for"
	CityTrail    []string `json:"city_trailing"` // words that follow the city and are not part of it ("today", "thế nào")

	NamePrefixes     []string       `json:"name_prefixes"`
	BirthdayPrefixes []string       `json:"birthday_prefixes"`
	DayWord          string         `json:"day_word"`   // "ngày"
	MonthWord        string         `json:"month_word"` // "tháng"
	Months           map[string]int `json:"months"`
	ChoosePrefixes   []string       `json:"choose_prefixes"` // "i choose", "ô số"

	Animals    map[string]string `json:"animals"`     // phrase -> animal key (fox, cat, ...)
	AnimalCues []string          `json:"animal_cues"` // words that show an animal is being asked about, not just mentioned

	// Added for languages written without spaces (zh, ja, ko: the pack sets "_chars") and for languages with other word orders.
	Chars          bool              `json:"-"`               // every non-ASCII character is its own token
	AtWords        []string          `json:"at_words"`        // "at" before a clock time ("a las", "um", "в", "alle")
	NumberAnd      []string          `json:"number_and"`      // joins tens and unit ("et", "y", "e")
	DayWords       []string          `json:"day_words"`       // words that mark a day of the month ("号", "日")
	MonthWords     []string          `json:"month_words"`     // words that mark a month number ("月")
	DateSuffix     bool              `json:"date_suffix"`     // the number comes before the day/month word (3月5日)
	ChooseCounters []string          `json:"choose_counters"` // counter after a chosen number ("番", "格")
	ChooseBare     bool              `json:"choose_bare"`     // "5番" on its own is a choice
	NameSuffix     []string          `json:"name_suffix"`     // closing words after a name ("です", "입니다")
	CityBefore     []string          `json:"city_before"`     // weather words that follow the city ("の天気", "날씨")
	CityLead       []string          `json:"city_lead"`       // words to drop before the city ("请问")
	CityParticle   []string          `json:"city_particles"`  // particle between city and weather word ("的", "の")
	CityForms      map[string]string `json:"city_forms"`      // inflected form -> base form ("москве" -> "Москва")

	norm func(string) string
}

// ---- normalisation ----

var foldTable = map[rune]rune{}

func init() {
	groups := map[rune]string{
		'a': "àáạảãâầấậẩẫăằắặẳẵäåā", 'e': "èéẹẻẽêềếệểễëē", 'i': "ìíịỉĩïī", 'o': "òóọỏõôồốộổỗơờớợởỡöøō",
		'u': "ùúụủũưừứựửữüū", 'y': "ỳýỵỷỹÿ", 'd': "đ", 'c': "ç", 'n': "ñ", 's': "ß", 'е': "ё", 'и': "й",
	}
	for base, set := range groups {
		for _, r := range set {
			foldTable[r] = base
		}
	}
}

// foldDiacritics lower-cases and removes accents (Vietnamese and common Latin letters).
func foldDiacritics(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := foldTable[r]; ok {
			r = f
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (lx *lexicon) normalize(s string) string {
	if lx.norm != nil {
		return lx.norm(s)
	}
	return normalizeLoose(s)
}

// fullWidth turns full-width ASCII (１２３ＡＢＣ) into plain ASCII.
func fullWidth(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0xFF01 && r <= 0xFF5E {
			return r - 0xFEE0
		}
		if r == 0x3000 {
			return ' '
		}
		return r
	}, s)
}

// splitChars cuts text into tokens for languages written without spaces: a run of ASCII letters/digits is one token, every other
// letter is its own token. Punctuation separates tokens. lower=false keeps the original case (for names and cities).
func splitChars(s string, lower bool) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range fullWidth(s) {
		if lower {
			r = unicode.ToLower(r)
		}
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			cur = append(cur, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flush()
			out = append(out, string(r))
		default:
			flush()
		}
	}
	flush()
	return out
}

// charNormalize is the normalisation for space-less languages: tokens joined by single spaces.
func charNormalize(s string) string { return strings.Join(splitChars(s, true), " ") }

// joinTokens glues tokens back into text: spaces only between two ASCII tokens (so "Linh Nguyen" and "小明" both come out right).
func joinTokens(tok []string) string {
	var b strings.Builder
	for i, t := range tok {
		if i > 0 && t != "" && tok[i-1] != "" && t[0] < 128 && tok[i-1][len(tok[i-1])-1] < 128 {
			b.WriteByte(' ')
		}
		b.WriteString(t)
	}
	return b.String()
}

// phraseAt returns how many tokens the longest phrase of list that starts at tok[k] has (0 = none).
func phraseAt(list []string, tok []string, k int) int {
	best := 0
	for _, p := range list {
		pt := strings.Fields(p)
		if len(pt) <= best || k < 0 || k+len(pt) > len(tok) {
			continue
		}
		ok := true
		for x, w := range pt {
			if tok[k+x] != w {
				ok = false
				break
			}
		}
		if ok {
			best = len(pt)
		}
	}
	return best
}

// phraseEndsAt reports whether some phrase of list ends exactly at tok[k].
func phraseEndsAt(list []string, tok []string, k int) bool {
	for _, p := range list {
		n := len(strings.Fields(p))
		if n > 0 && k-n+1 >= 0 && phraseAt([]string{p}, tok, k-n+1) == n {
			return true
		}
	}
	return false
}

// compile normalises every phrase in the lexicon the same way utterances will be normalised.
func (lx *lexicon) compile() *lexicon {
	if lx.norm == nil {
		switch {
		case lx.Chars && lx.Fold:
			lx.norm = func(s string) string { return charNormalize(foldDiacritics(s)) }
		case lx.Chars:
			lx.norm = charNormalize
		case lx.Fold:
			lx.norm = func(s string) string { return normalizeLoose(foldDiacritics(s)) }
		default:
			lx.norm = normalizeLoose
		}
	}
	list := func(p *[]string) {
		for i, s := range *p {
			(*p)[i] = lx.norm(s)
		}
	}
	for _, p := range []*[]string{&lx.SecUnits, &lx.MinUnits, &lx.HourUnits, &lx.OneWords, &lx.HalfWords, &lx.ClockWords, &lx.AmWords, &lx.PmWords,
		&lx.NoonWords, &lx.NightWords, &lx.TimerWords, &lx.TimerCancel, &lx.AlarmWords, &lx.RemindWords, &lx.RemoveWords,
		&lx.CheckWords, &lx.OnWords, &lx.OffWords, &lx.AllWords, &lx.StopWords, &lx.EveryDay, &lx.WeekdayPhr, &lx.WeekendPhr, &lx.LightWords,
		&lx.EyeWords, &lx.DefaultWords, &lx.AnimalCues, &lx.WeatherWords, &lx.InWords, &lx.CityTrail, &lx.NamePrefixes, &lx.BirthdayPrefixes, &lx.ChoosePrefixes,
		&lx.AtWords, &lx.NumberAnd, &lx.DayWords, &lx.MonthWords, &lx.ChooseCounters, &lx.NameSuffix, &lx.CityBefore, &lx.CityLead, &lx.CityParticle} {
		list(p)
	}
	lx.DayWord, lx.MonthWord = lx.norm(lx.DayWord), lx.norm(lx.MonthWord)
	if lx.DayWord != "" {
		lx.DayWords = append(lx.DayWords, lx.DayWord)
	}
	if lx.MonthWord != "" {
		lx.MonthWords = append(lx.MonthWords, lx.MonthWord)
	}
	forms := map[string]string{}
	for k, v := range lx.CityForms {
		forms[lx.norm(k)] = v
	}
	lx.CityForms = forms
	remapI := func(m map[string]int) map[string]int {
		o := map[string]int{}
		for k, v := range m {
			o[lx.norm(k)] = v
		}
		return o
	}
	remapS := func(m map[string]string) map[string]string {
		o := map[string]string{}
		for k, v := range m {
			o[lx.norm(k)] = v
		}
		return o
	}
	lx.Numbers, lx.Ordinals, lx.Months = remapI(lx.Numbers), remapI(lx.Ordinals), remapI(lx.Months)
	lx.Weekdays, lx.Tags, lx.Animals = remapS(lx.Weekdays), remapS(lx.Tags), remapS(lx.Animals)
	cols := map[string][]int{}
	for k, v := range lx.Colours {
		cols[lx.norm(k)] = v
	}
	lx.Colours = cols
	return lx
}

// ---- matching helpers ----

func in(list []string, w string) bool {
	for _, x := range list {
		if x == w {
			return true
		}
	}
	return false
}

func (lx *lexicon) has(t string, phrases []string) bool {
	if len(phrases) == 0 {
		return false
	}
	return hasPhrase(t, phrases...)
}

// longestKey returns the value whose phrase occurs in t, preferring the longest phrase.
func longestKey[V any](t string, m map[string]V) (V, bool) {
	var zero V
	best, bestLen := zero, 0
	found := false
	for k, v := range m {
		if k != "" && hasPhrase(t, k) && len(k) > bestLen {
			best, bestLen, found = v, len(k), true
		}
	}
	return best, found
}

// ---- numbers ----

func (lx *lexicon) number(tok []string, i int) (int, int) {
	if i >= len(tok) || i < 0 {
		return 0, 0
	}
	if n, err := strconv.Atoi(tok[i]); err == nil && n >= 0 && n < 100000 {
		return n, 1
	}
	if n, ok := digitsWithSuffix(tok[i]); ok && lx.Lang == "en" {
		return n, 1
	}
	switch lx.NumberStyle {
	case "vi":
		return lx.numberVI(tok, i)
	case "cjk":
		return lx.numberCJK(tok, i)
	case "ko":
		if n, used := lx.numberKONative(tok, i); used > 0 {
			return n, used
		}
		return lx.numberCJK(tok, i)
	}
	for k := 3; k >= 2; k-- { // multi-word numbers ("vingt et un", "quatre vingt")
		if i+k <= len(tok) {
			if v, ok := lx.Numbers[strings.Join(tok[i:i+k], " ")]; ok {
				return v, k
			}
		}
	}
	v, ok := lx.Numbers[tok[i]]
	if !ok {
		return 0, 0
	}
	if v >= 20 && v%10 == 0 && i+1 < len(tok) {
		j := i + 1
		if len(lx.NumberAnd) > 0 && in(lx.NumberAnd, tok[j]) && j+1 < len(tok) {
			if w, ok := lx.Numbers[tok[j+1]]; ok && w >= 1 && w <= 9 {
				return v + w, 3
			}
		}
		if w, ok := lx.Numbers[tok[j]]; ok && w >= 1 && w <= 9 {
			return v + w, 2
		}
	}
	return v, 1
}

// numberCJK reads Chinese/Japanese/Korean-Sino numerals written with single characters: 十 10, 二十五 25, 一百零五 105.
// Multipliers (10, 100, 1000) are the entries of the lexicon's numbers whose value is a power of ten.
func (lx *lexicon) numberCJK(tok []string, i int) (int, int) {
	total, cur, used := 0, 0, 0
	lastDigit := false
	for j := i; j < len(tok); j++ {
		v, ok := lx.Numbers[tok[j]]
		if !ok {
			break
		}
		if v == 10 || v == 100 || v == 1000 {
			if cur == 0 {
				cur = 1
			}
			total += cur * v
			cur, lastDigit = 0, false
		} else if v >= 0 && v <= 9 {
			if lastDigit {
				if cur != 0 { // two bare digits in a row are two numbers (오일 = 5 + 日), except after a zero (一百零五)
					break
				}
				cur = v
			} else {
				cur = v
			}
			lastDigit = true
		} else {
			break
		}
		used++
	}
	if used == 0 {
		return 0, 0
	}
	return total + cur, used
}

// Korean native numerals (하나 둘 셋 ... 열 스물 서른 ...), including the short forms used before counters (한 두 세 네).
var koTens = map[string]int{"열": 10, "스물": 20, "스무": 20, "서른": 30, "마흔": 40, "쉰": 50}
var koUnits = map[string]int{"하나": 1, "한": 1, "둘": 2, "두": 2, "셋": 3, "세": 3, "넷": 4, "네": 4, "다섯": 5, "여섯": 6, "일곱": 7, "여덟": 8, "아홉": 9}

func (lx *lexicon) numberKONative(tok []string, i int) (int, int) {
	match := func(m map[string]int, at int) (int, int) { // longest of 1..2 characters
		for k := 2; k >= 1; k-- {
			if at+k <= len(tok) {
				if v, ok := m[strings.Join(tok[at:at+k], "")]; ok {
					return v, k
				}
			}
		}
		return 0, 0
	}
	tens, tu := match(koTens, i)
	if tu > 0 {
		if u, uu := match(koUnits, i+tu); uu > 0 {
			return tens + u, tu + uu
		}
		return tens, tu
	}
	return match(koUnits, i)
}

// Vietnamese numerals: một hai ba bốn năm sáu bảy tám chín mười; "mười lăm" 15, "hai mươi mốt" 21, "một trăm lẻ năm" 105.
func (lx *lexicon) numberVI(tok []string, i int) (int, int) {
	base := func(w string) (int, bool) { v, ok := lx.Numbers[w]; return v, ok && v >= 0 && v <= 9 }
	unit := func(w string) (int, bool) { // a digit word, including the forms used after "mười/mươi"
		switch w {
		case "mốt", "mot":
			return 1, true
		case "tư", "tu":
			return 4, true
		case "lăm", "lam":
			return 5, true
		}
		return base(w)
	}
	isTen := func(w string) bool { return w == "mười" || w == "muoi" }
	isTens := func(w string) bool { return w == "mươi" || w == "muoi" }
	isHundred := func(w string) bool { return w == "trăm" || w == "tram" }
	below100 := func(j int) (int, int) {
		if j >= len(tok) {
			return 0, 0
		}
		w := tok[j]
		if isTen(w) {
			v, used := 10, 1
			if j+1 < len(tok) {
				if u, ok := unit(tok[j+1]); ok && u >= 1 {
					v, used = v+u, 2
				}
			}
			return v, used
		}
		if u, ok := base(w); ok {
			if u >= 2 && j+1 < len(tok) && isTens(tok[j+1]) {
				v, used := u*10, 2
				if j+2 < len(tok) {
					if u2, ok := unit(tok[j+2]); ok && u2 >= 1 {
						v, used = v+u2, 3
					}
				}
				return v, used
			}
			return u, 1
		}
		return 0, 0
	}
	if u, ok := base(tok[i]); ok && i+1 < len(tok) && isHundred(tok[i+1]) {
		v, used := u*100, 2
		j := i + 2
		if j < len(tok) && (tok[j] == "lẻ" || tok[j] == "le" || tok[j] == "linh") {
			if j+1 < len(tok) {
				if u2, ok := unit(tok[j+1]); ok {
					v, used = v+u2, used+2
				}
			}
		} else if r, ru := below100(j); ru > 0 {
			v, used = v+r, used+ru
		}
		return v, used
	}
	return below100(i)
}

// ---- durations and clock times ----

func (lx *lexicon) duration(tok []string) int {
	total := 0
	for i := 0; i < len(tok); i++ {
		if h := phraseAt(lx.HalfWords, tok, i); h > 0 { // "nửa tiếng", "half an hour"
			j := i + h
			if j < len(tok) && in(lx.OneWords, tok[j]) {
				j++
			}
			if u := phraseAt(lx.HourUnits, tok, j); u > 0 {
				total += 1800
				i = j + u - 1
				continue
			}
		}
		n, used := lx.number(tok, i)
		if used == 0 && in(lx.OneWords, tok[i]) {
			n, used = 1, 1
		}
		if used == 0 || i+used >= len(tok) {
			continue
		}
		k := i + used
		if lx.Chars && k < len(tok) && in([]string{"个", "個", "개"}, tok[k]) { // Chinese/Japanese measure word between number and unit
			k++
		}
		switch {
		case phraseAt(lx.SecUnits, tok, k) > 0:
			total += n
			k += phraseAt(lx.SecUnits, tok, k)
		case phraseAt(lx.MinUnits, tok, k) > 0:
			total += n * 60
			k += phraseAt(lx.MinUnits, tok, k)
		case phraseAt(lx.HourUnits, tok, k) > 0:
			total += n * 3600
			k += phraseAt(lx.HourUnits, tok, k)
			if h := phraseAt(lx.HalfWords, tok, k); h > 0 { // "một tiếng rưỡi", "一个半小时"
				total += 1800
				k += h
			}
		default:
			continue
		}
		i = k - 1
	}
	return total
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// splitHour turns "9h30" / "9h" into "9", "h", "30" so French-style clock times read like the others.
func splitHour(tok []string) []string {
	var out []string
	for _, w := range tok {
		if i := strings.IndexByte(w, 'h'); i > 0 && isDigits(w[:i]) && (i == len(w)-1 || isDigits(w[i+1:])) {
			out = append(out, w[:i], "h")
			if i < len(w)-1 {
				out = append(out, w[i+1:])
			}
			continue
		}
		out = append(out, w)
	}
	return out
}

// periodAt finds a day-part word (morning, evening, ...) right after the time (at k) or right before the hour (ending at i-1).
func (lx *lexicon) periodAt(tok []string, k, i int) string {
	for _, c := range []struct {
		name string
		list []string
	}{{"pm", lx.PmWords}, {"am", lx.AmWords}, {"noon", lx.NoonWords}, {"night", lx.NightWords}} {
		if len(c.list) == 0 {
			continue
		}
		if (!lx.Chars && phraseAt(c.list, tok, k) > 0) || phraseEndsAt(c.list, tok, i-1) || (i >= 2 && in(lx.AtWords, tok[i-1]) && phraseEndsAt(c.list, tok, i-2)) {
			return c.name
		}
	}
	return ""
}

// clock reads "7 giờ sáng", "chín giờ tối", "bảy giờ rưỡi", "6 giờ 30 chiều", "a las 7 de la mañana", "晚上八点", "um 21 Uhr", "alle 8:00".
// An hour counts as a clock time if an hour word follows it, an "at" word precedes it, a day-part word is next to it, or a
// two-digit minute follows ("9 00"). ok=false if none.
func (lx *lexicon) clock(tok []string) (string, bool) {
	tok = splitHour(tok)
	for i := range tok {
		h, used := lx.number(tok, i)
		if used == 0 || h > 24 {
			continue
		}
		j := i + used
		hw := phraseAt(lx.ClockWords, tok, j)
		atBefore := i > 0 && phraseEndsAt(lx.AtWords, tok, i-1)
		k := j + hw
		m, colon := 0, false
		if h2 := phraseAt(lx.HalfWords, tok, k); h2 > 0 && (hw > 0 || atBefore) {
			m, k = 30, k+h2
		} else {
			kk := k
			if hw > 0 && kk < len(tok) && in(lx.NumberAnd, tok[kk]) { // "seis horas e trinta"
				kk++
			}
			if mm, u := lx.number(tok, kk); u > 0 && mm <= 59 && (hw > 0 || (u == 1 && len(tok[kk]) == 2 && isDigits(tok[kk]))) {
				m, k, colon = mm, kk+u, hw == 0
				if u2 := phraseAt(lx.MinUnits, tok, k); u2 > 0 {
					k += u2
				}
			}
		}
		period := lx.periodAt(tok, k, i)
		if hw == 0 && !atBefore && !colon && period == "" {
			continue
		}
		switch period {
		case "pm":
			if h < 12 {
				h += 12
			}
		case "am":
			if h == 12 {
				h = 0
			}
		case "noon":
			if h >= 1 && h <= 4 {
				h += 12
			}
		case "night":
			if h >= 6 && h < 12 {
				h += 12
			} else if h == 12 {
				h = 0
			}
		}
		if h > 23 || m > 59 {
			continue
		}
		return pad2(h) + ":" + pad2(m), true
	}
	return "", false
}

func (lx *lexicon) recurrence(t string) string {
	switch {
	case lx.has(t, lx.EveryDay):
		return "ED"
	case lx.has(t, lx.WeekdayPhr):
		return "WDAY"
	case lx.has(t, lx.WeekendPhr):
		return "WKND"
	}
	if c, ok := longestKey(t, lx.Weekdays); ok {
		return c
	}
	return "ONCE"
}

func (lx *lexicon) ordinal(tok []string, t string) int {
	if n, ok := longestKey(t, lx.Ordinals); ok {
		return n
	}
	for i := range tok {
		if n, used := lx.number(tok, i); used > 0 && n >= 1 && n <= 20 {
			return n
		}
	}
	return 0
}

// ---- original-case text after a prefix (names, cities) ----

// fieldsOrig splits the original text into tokens that line up with the normalised ones (spaces, or characters for space-less languages).
func (lx *lexicon) fieldsOrig(orig string) []string {
	if lx.Chars {
		return splitChars(orig, false)
	}
	return strings.Fields(orig)
}

// afterPrefix returns the words that follow the first matching prefix, with their original spelling. A whitespace-separated word
// may normalise to several tokens (the apostrophe in "m'appelle"), so matching runs over the normalised tokens and the result is
// the original words after the one that ends the prefix.
func (lx *lexicon) afterPrefix(orig string, prefixes []string) ([]string, bool) {
	fields := lx.fieldsOrig(orig)
	var flat []string // normalised tokens
	var owner []int   // index of the original word each token came from
	for i, f := range fields {
		for _, w := range strings.Fields(lx.normalize(f)) {
			flat = append(flat, w)
			owner = append(owner, i)
		}
	}
	for _, p := range prefixes {
		pt := strings.Fields(p)
		if len(pt) == 0 {
			continue
		}
		for s := 0; s+len(pt) <= len(flat); s++ {
			match := true
			for k, w := range pt {
				if flat[s+k] != w {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			last := owner[s+len(pt)-1]
			var out []string
			for _, f := range fields[last+1:] {
				f = strings.TrimFunc(f, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
				if f != "" {
					out = append(out, f)
				}
			}
			return out, true
		}
	}
	return nil, false
}

// trimTail removes closing words (name suffixes, city trailers) from the end of the original-case words. Matching runs over
// normalised tokens, so a word like "aujourd'hui" (two tokens) is found.
func (lx *lexicon) trimTail(rest []string, list []string) []string {
	for changed := true; changed && len(rest) > 0; {
		changed = false
		var flat []string
		var owner []int
		for i, f := range rest {
			for _, w := range strings.Fields(lx.normalize(f)) {
				flat = append(flat, w)
				owner = append(owner, i)
			}
		}
		for _, ph := range list {
			pt := strings.Fields(ph)
			if len(pt) == 0 || len(pt) > len(flat) {
				continue
			}
			start := len(flat) - len(pt)
			ok := true
			for k := range pt {
				if flat[start+k] != pt[k] {
					ok = false
					break
				}
			}
			if ok && (start == 0 || owner[start-1] != owner[start]) {
				rest, changed = rest[:owner[start]], true
				break
			}
		}
	}
	return rest
}

func titleCase(w string) string {
	r := []rune(strings.ToLower(w))
	if len(r) == 0 {
		return w
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// ---- generic rules ----

// strictRules need explicit structure (a timer word and a duration, an alarm word and a time, a name prefix, ...) and run before
// the phrase pack; looseRules (light, animals) match on single words, so the phrase pack gets to claim longer phrases first.
func (lx *lexicon) strictRules() []ruleFn {
	return []ruleFn{lx.ruleChoose, lx.ruleName, lx.ruleTimer, lx.ruleAlarm, lx.ruleEyes, lx.ruleWeatherCity}
}

func (lx *lexicon) looseRules() []ruleFn { return []ruleFn{lx.ruleLight, lx.ruleAnimal} }

func (lx *lexicon) ruleChoose(orig, t string, tok []string) (nluResult, bool) {
	counter := func(rest []string) []string { // drop a trailing counter word ("番", "번", "格")
		if len(rest) > 1 && in(lx.ChooseCounters, rest[len(rest)-1]) {
			return rest[:len(rest)-1]
		}
		return rest
	}
	accept := func(rest []string) bool {
		rest = counter(rest)
		n, used := lx.number(rest, 0)
		return used > 0 && used == len(rest) && n >= 1 && n <= 10
	}
	if lx.ChooseBare && accept(tok) {
		return res("user_choose_number", "", nil)
	}
	for _, p := range lx.ChoosePrefixes {
		if i := strings.Index(" "+t+" ", " "+p+" "); i >= 0 {
			rest := strings.Fields(strings.TrimSpace((" " + t + " ")[i+len(p)+2:]))
			if accept(rest) {
				return res("user_choose_number", "", nil)
			}
		}
	}
	return nluResult{}, false
}

// numberBefore reads a number that ends right before tok[i] (3月: the 3 before 月).
func (lx *lexicon) numberBefore(tok []string, i int, used map[int]bool) (int, int) {
	for s := 3; s >= 1; s-- {
		j := i - s
		if j < 0 {
			continue
		}
		if n, u := lx.number(tok, j); u == s {
			free := true
			for x := j; x < i; x++ {
				if used[x] {
					free = false
				}
			}
			if free {
				return n, s
			}
		}
	}
	return 0, 0
}

// birthDate reads month and day from the words after "my birthday is": month/day marker words ("tháng 12 ngày 25", "3月5日"),
// or a month name with a day number ("24 de marzo", "5 mars", "March 5th").
func (lx *lexicon) birthDate(tok []string) (mo, day int) {
	used := map[int]bool{}
	mark := func(from, to int) {
		for x := from; x < to; x++ {
			used[x] = true
		}
	}
	find := func(words []string) (val int) {
		for i := 0; i < len(tok); i++ {
			w := phraseAt(words, tok, i)
			if w == 0 || used[i] {
				continue
			}
			if lx.DateSuffix {
				if n, s := lx.numberBefore(tok, i, used); s > 0 {
					mark(i-s, i+w)
					val = n
					continue
				}
			}
			if n, u := lx.number(tok, i+w); u > 0 {
				mark(i, i+w+u)
				val = n
			}
		}
		return val
	}
	mo = find(lx.MonthWords)
	day = find(lx.DayWords)
	if mo == 0 { // month names
		for i, w := range tok {
			m, ok := lx.Months[w]
			if !ok {
				continue
			}
			mo = m
			used[i] = true
			break
		}
		if mo > 0 && day == 0 {
			for i := range tok {
				if used[i] {
					continue
				}
				if o, ok := lx.Ordinals[tok[i]]; ok && o >= 1 && o <= 31 {
					day = o
					break
				}
				if n, u := lx.number(tok, i); u > 0 && n >= 1 && n <= 31 {
					day = n
					break
				}
			}
		}
	}
	return mo, day
}

func (lx *lexicon) ruleName(orig, t string, tok []string) (nluResult, bool) {
	if len(lx.BirthdayPrefixes) > 0 && lx.has(t, lx.BirthdayPrefixes) {
		mo, day := lx.birthDate(tok)
		if mo >= 1 && mo <= 12 && day >= 1 && day <= 31 {
			return res("my_birthday_is", "tree_search_and_update_name", map[string]any{"birth_date": strconv.Itoa(mo) + "." + strconv.Itoa(day)})
		}
		return nluResult{}, false
	}
	if rest, ok := lx.afterPrefix(orig, lx.NamePrefixes); ok {
		rest = lx.trimTail(rest, lx.NameSuffix)
		max := 3
		if lx.Chars {
			max = 8
		}
		if len(rest) >= 1 && len(rest) <= max {
			if !lx.Chars {
				for i := range rest {
					rest[i] = titleCase(rest[i])
				}
				return res("my_name_is", "tree_search_and_update_name", map[string]any{"name": strings.Join(rest, " ")})
			}
			for i, w := range rest {
				if w[0] < 128 {
					rest[i] = titleCase(w)
				}
			}
			return res("my_name_is", "tree_search_and_update_name", map[string]any{"name": joinTokens(rest)})
		}
	}
	return nluResult{}, false
}

func (lx *lexicon) ruleTimer(orig, t string, tok []string) (nluResult, bool) {
	if !lx.has(t, lx.TimerWords) {
		return nluResult{}, false
	}
	if lx.has(t, lx.TimerCancel) {
		return util("timer_cancel", "timer", map[string]any{"timer": map[string]any{"operation": "cancel"}})
	}
	if d := lx.duration(tok); d > 0 && d <= 24*3600 {
		return util("timer_set", "timer", map[string]any{"timer": map[string]any{"operation": "set", "duration": d}})
	}
	return nluResult{}, false
}

func (lx *lexicon) alarmTag(t string) string {
	if tag, ok := longestKey(t, lx.Tags); ok {
		return tag
	}
	return "alarm"
}

func (lx *lexicon) ruleAlarm(orig, t string, tok []string) (nluResult, bool) {
	isAlarm := lx.has(t, lx.AlarmWords)
	if !isAlarm && !lx.has(t, lx.RemindWords) {
		return nluResult{}, false
	}
	ord := lx.ordinal(tok, t)
	idx := ""
	if ord > 0 && !lx.has(t, lx.AllWords) {
		idx = strconv.Itoa(ord)
	}
	at, hasClock := "", false
	if lx.ClockWords != nil || lx.AtWords != nil {
		at, hasClock = lx.clock(tok)
	} else {
		at, hasClock = clock(tok) // English-style clock for lexicons without hour words
	}
	switch {
	case isAlarm && lx.has(t, lx.StopWords) && !lx.has(t, lx.RemoveWords):
		return res("be_quiet", "stay_still", nil)
	case isAlarm && lx.has(t, lx.RemoveWords):
		return util("alarm_remove", "alarm", map[string]any{"alarm": map[string]any{"operation": "remove", "index": idx}})
	case hasClock:
		return util("alarm_set", "alarm", map[string]any{"alarm": map[string]any{
			"operation": "set", "time": at, "recurrence": lx.recurrence(t), "tag": lx.alarmTag(t)}})
	case isAlarm && (lx.has(t, lx.OnWords) || lx.has(t, lx.OffWords)) && (ord > 0 || lx.has(t, lx.AllWords)):
		on := 1
		if lx.has(t, lx.OffWords) {
			on = 0
		}
		return util("alarm_update", "alarm", map[string]any{"alarm": map[string]any{"operation": "update", "index": idx, "schedule_on": on}})
	case isAlarm && lx.has(t, lx.CheckWords):
		return util("alarm_check", "alarm", map[string]any{"alarm": map[string]any{"operation": "check"}})
	}
	return nluResult{}, false
}

func (lx *lexicon) colourOf(t string) ([3]int, bool) {
	if c, ok := longestKey(t, lx.Colours); ok && len(c) == 3 {
		return [3]int{c[0], c[1], c[2]}, true
	}
	return [3]int{}, false
}

func (lx *lexicon) ruleEyes(orig, t string, tok []string) (nluResult, bool) {
	if !lx.has(t, lx.EyeWords) {
		return nluResult{}, false
	}
	c, ok := lx.colourOf(t)
	if !ok && lx.has(t, lx.DefaultWords) {
		c, ok = [3]int{0, 0, 0}, true // "default": the robot's own colour [V]
	}
	if !ok {
		return nluResult{}, false
	}
	return util("eye_color", "customize", map[string]any{"customize": hsl(c)})
}

func (lx *lexicon) ruleLight(orig, t string, tok []string) (nluResult, bool) {
	isLight := lx.has(t, lx.LightWords)
	if !isLight && !(len(tok) <= 2) {
		return nluResult{}, false
	}
	off, on := lx.has(t, lx.OffWords), lx.has(t, lx.OnWords)
	switch {
	case isLight && off:
		return util("turn_off_light", "light", map[string]any{"light": map[string]any{"operation": "off", "room": ""}})
	}
	if c, ok := lx.colourOf(t); ok && (isLight || len(tok) <= 2) {
		return util("turn_on_light", "light", map[string]any{"light": map[string]any{"operation": "set_hsl", "hsl": hsl(c), "room": ""}})
	}
	if isLight && on {
		return util("turn_on_light", "light", map[string]any{"light": map[string]any{"operation": "set_hsl", "hsl": hsl([3]int{0, 0, 100}), "room": ""}})
	}
	return nluResult{}, false
}

// cityResult builds the weather command for a city, mapping inflected forms to the base name.
func (lx *lexicon) cityResult(rest []string) (nluResult, bool) {
	if len(rest) == 0 {
		return nluResult{}, false
	}
	city := joinTokens(rest)
	if !lx.Chars {
		city = strings.Join(rest, " ")
	}
	if base, ok := lx.CityForms[lx.normalize(city)]; ok {
		city = base
	}
	if len([]rune(city)) > 60 {
		return nluResult{}, false
	}
	return nluResult{Intent: "weather", Behavior: "utilities", Paras: map[string]any{"utility_type": "weather"}, NeedWeather: true, City: city}, true
}

func (lx *lexicon) ruleWeatherCity(orig, t string, tok []string) (nluResult, bool) {
	if !lx.has(t, lx.WeatherWords) {
		return nluResult{}, false
	}
	if len(lx.CityBefore) > 0 { // the city comes before the weather word: "伦敦的天气", "東京の天気", "서울 날씨"
		fields := lx.fieldsOrig(orig)
		norm := make([]string, len(fields))
		for i, f := range fields {
			norm[i] = lx.normalize(f)
		}
		p := -1
		for i := range norm {
			if phraseAt(lx.CityBefore, norm, i) > 0 {
				p = i
				break
			}
		}
		if p <= 0 || p > 10 {
			return nluResult{}, false
		}
		rest := fields[:p]
		rest = lx.trimTail(rest, lx.CityParticle)
		for changed := true; changed && len(rest) > 0; { // leading filler ("请问", "教えて")
			changed = false
			if n := phraseAt(lx.CityLead, lx.normAll(rest), 0); n > 0 {
				rest, changed = rest[n:], true
			}
		}
		katakana := true
		for _, w := range rest {
			if r := []rune(w); len(r) != 1 || r[0] < 0x30A0 || r[0] > 0x30FF {
				katakana = false
			}
		}
		maxCity := 5 // a city name is short; a longer stretch before the weather word is a sentence, not a place
		if katakana {
			maxCity = 8 // ニューヨーク
		}
		after := len(norm) - p - phraseAt(lx.CityBefore, norm, p)
		if len(rest) == 0 || len(rest) > maxCity || after > 8 || in(lx.CityTrail, lx.normalize(joinTokens(rest))) || phraseAt(lx.CityTrail, lx.normAll(rest), 0) == len(rest) {
			return nluResult{}, false
		}
		return lx.cityResult(rest)
	}
	if len(lx.InWords) == 0 || len(tok) > 8 {
		return nluResult{}, false
	}
	rest, ok := lx.afterPrefix(orig, lx.InWords)
	if !ok || len(rest) == 0 || len(rest) > 4 {
		return nluResult{}, false
	}
	rest = lx.trimTail(rest, lx.CityTrail) // drop trailing words that are not part of the city
	return lx.cityResult(rest)
}

func (lx *lexicon) normAll(tok []string) []string {
	out := make([]string, len(tok))
	for i, w := range tok {
		out[i] = lx.normalize(w)
	}
	return out
}

func (lx *lexicon) ruleAnimal(orig, t string, tok []string) (nluResult, bool) {
	if len(lx.Animals) == 0 || len(tok) > 8 && !(lx.Chars && len(tok) <= 16) {
		return nluResult{}, false
	}
	if a, ok := longestKey(t, lx.Animals); ok && (len(tok) <= 2 || lx.has(t, lx.AnimalCues)) {
		return res("animal", "play_animation", map[string]any{"animation_name": animalAnim(a)})
	}
	return nluResult{}, false
}

// sortedKeys is a small helper for deterministic output in tests and docs.
func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
