package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// ---- spoken prompts (our own wording; override or extend with prompts_file) ----

// The keys are the robot's own event names, which it passes as the "type" of a voice-prompt request when it
// cannot say the line itself (non-English units). Values are keyed by base language code.
var defaultPrompts = map[string]map[string]string{
	"en": {
		"chat_start":                   "Hi{name}! What shall we talk about?",
		"not_execute":                  "I can't do that right now.",
		"not_execute2":                 "I'm busy at the moment. Ask me again in a bit.",
		"not_execute3":                 "Not now, I'm in the middle of something.",
		"not_execute4":                 "Let's try that a little later.",
		"speak_notconnect_homestation": "I need my home station for that, and I can't find it.",
		"default":                      "Sorry, I can't do that right now.",
		"dont_understand":              "Sorry, I didn't catch that.",
		"weather_unavailable":          "I can't check the weather right now.",
		"news_unavailable":             "I can't get the news right now.",
	},
	// The lines below were written for this project and have NOT been reviewed by native speakers: please correct them
	// (override any line with prompts_file).
	"de": {
		"chat_start":                   "Hallo{name}! Worüber wollen wir reden?",
		"not_execute":                  "Das kann ich gerade nicht.",
		"not_execute2":                 "Ich bin gerade beschäftigt. Frag mich gleich noch einmal.",
		"not_execute3":                 "Jetzt nicht, ich bin mitten in etwas.",
		"not_execute4":                 "Lass uns das ein bisschen später versuchen.",
		"speak_notconnect_homestation": "Dafür brauche ich meine Ladestation, aber ich finde sie nicht.",
		"default":                      "Entschuldige, das kann ich gerade nicht.",
		"dont_understand":              "Entschuldige, das habe ich nicht verstanden.",
		"weather_unavailable":          "Ich kann das Wetter gerade nicht abrufen.",
		"news_unavailable":             "Ich kann gerade keine Nachrichten abrufen.",
	},
	"fr": {
		"chat_start":                   "Salut{name} ! De quoi voulez-vous parler ?",
		"not_execute":                  "Je ne peux pas faire ça maintenant.",
		"not_execute2":                 "Je suis occupé pour le moment. Redemande-moi dans un instant.",
		"not_execute3":                 "Pas maintenant, je suis en plein milieu de quelque chose.",
		"not_execute4":                 "Essayons un peu plus tard.",
		"speak_notconnect_homestation": "J'ai besoin de ma station d'accueil pour ça, et je ne la trouve pas.",
		"default":                      "Désolé, je ne peux pas faire ça maintenant.",
		"dont_understand":              "Désolé, je n'ai pas compris.",
		"weather_unavailable":          "Je ne peux pas consulter la météo pour l'instant.",
		"news_unavailable":             "Je ne peux pas récupérer les actualités pour l'instant.",
	},
	"es": {
		"chat_start":                   "¡Hola{name}! ¿De qué quieres hablar?",
		"not_execute":                  "No puedo hacer eso ahora mismo.",
		"not_execute2":                 "Estoy ocupado ahora. Pregúntame de nuevo en un rato.",
		"not_execute3":                 "Ahora no, estoy en medio de algo.",
		"not_execute4":                 "Intentémoslo un poco más tarde.",
		"speak_notconnect_homestation": "Necesito mi base de carga para eso y no la encuentro.",
		"default":                      "Lo siento, no puedo hacer eso ahora mismo.",
		"dont_understand":              "Lo siento, no te he entendido.",
		"weather_unavailable":          "Ahora mismo no puedo consultar el tiempo.",
		"news_unavailable":             "Ahora mismo no puedo obtener las noticias.",
	},
	"it": {
		"chat_start":                   "Ciao{name}! Di cosa vuoi parlare?",
		"not_execute":                  "Non posso farlo adesso.",
		"not_execute2":                 "Sono occupato in questo momento. Chiedimelo di nuovo tra poco.",
		"not_execute3":                 "Adesso no, sono nel mezzo di qualcosa.",
		"not_execute4":                 "Proviamo un po' più tardi.",
		"speak_notconnect_homestation": "Per questo mi serve la mia base di ricarica, ma non la trovo.",
		"default":                      "Mi dispiace, non posso farlo adesso.",
		"dont_understand":              "Mi dispiace, non ho capito.",
		"weather_unavailable":          "Al momento non riesco a controllare il meteo.",
		"news_unavailable":             "Al momento non riesco a ottenere le notizie.",
	},
	"vi": {
		"chat_start":                   "Chào{name}! Mình nói chuyện về gì nhỉ?",
		"not_execute":                  "Mình chưa làm được việc đó lúc này.",
		"not_execute2":                 "Mình đang hơi bận, lát nữa hỏi lại nhé.",
		"not_execute3":                 "Chưa được đâu, mình đang làm dở việc khác.",
		"not_execute4":                 "Mình thử lại sau một chút nhé.",
		"speak_notconnect_homestation": "Mình cần đế sạc để làm việc này nhưng không tìm thấy nó.",
		"default":                      "Xin lỗi, mình chưa làm được việc đó lúc này.",
		"dont_understand":              "Xin lỗi, mình chưa nghe rõ.",
		"weather_unavailable":          "Mình chưa xem được thời tiết lúc này.",
		"news_unavailable":             "Mình chưa lấy được tin tức lúc này.",
	},
	"pt": {
		"chat_start":                   "Olá{name}! Sobre o que queres falar?",
		"not_execute":                  "Não posso fazer isso agora.",
		"not_execute2":                 "Estou ocupado neste momento. Pergunta-me outra vez daqui a pouco.",
		"not_execute3":                 "Agora não, estou no meio de uma coisa.",
		"not_execute4":                 "Vamos tentar um pouco mais tarde.",
		"speak_notconnect_homestation": "Preciso da minha base de carregamento para isso e não a encontro.",
		"default":                      "Desculpa, não posso fazer isso agora.",
		"dont_understand":              "Desculpa, não percebi.",
		"weather_unavailable":          "Não consigo ver o tempo neste momento.",
		"news_unavailable":             "Não consigo obter as notícias neste momento.",
	},
	"ru": {
		"chat_start":                   "Привет{name}! О чём поговорим?",
		"not_execute":                  "Сейчас я не могу этого сделать.",
		"not_execute2":                 "Я сейчас занят. Спроси меня чуть позже.",
		"not_execute3":                 "Не сейчас, я занят одним делом.",
		"not_execute4":                 "Давай попробуем чуть позже.",
		"speak_notconnect_homestation": "Для этого мне нужна моя домашняя станция, но я не могу её найти.",
		"default":                      "Извини, сейчас я не могу этого сделать.",
		"dont_understand":              "Извини, я не расслышал.",
		"weather_unavailable":          "Сейчас я не могу узнать погоду.",
		"news_unavailable":             "Сейчас я не могу получить новости.",
	},
	"ja": {
		"chat_start":                   "こんにちは{name}。何をお話ししましょうか。",
		"not_execute":                  "今はそれができません。",
		"not_execute2":                 "今ちょっと忙しいので、あとでもう一度聞いてね。",
		"not_execute3":                 "今は手が離せません。",
		"not_execute4":                 "少しあとでやってみましょう。",
		"speak_notconnect_homestation": "それにはホームステーションが必要ですが、見つかりません。",
		"default":                      "ごめんなさい、今はそれができません。",
		"dont_understand":              "ごめんなさい、聞き取れませんでした。",
		"weather_unavailable":          "今は天気を調べられません。",
		"news_unavailable":             "今はニュースを取得できません。",
	},
	"ko": {
		"chat_start":                   "안녕{name}! 무슨 얘기를 할까?",
		"not_execute":                  "지금은 그걸 할 수 없어.",
		"not_execute2":                 "지금은 좀 바빠. 조금 있다가 다시 말해 줘.",
		"not_execute3":                 "지금은 안 돼, 하던 일이 있어.",
		"not_execute4":                 "조금 이따가 다시 해 보자.",
		"speak_notconnect_homestation": "그러려면 홈스테이션이 필요한데, 찾을 수가 없어.",
		"default":                      "미안, 지금은 그걸 할 수 없어.",
		"dont_understand":              "미안, 잘 못 들었어.",
		"weather_unavailable":          "지금은 날씨를 확인할 수 없어.",
		"news_unavailable":             "지금은 뉴스를 가져올 수 없어.",
	},
	"zh": {
		"chat_start":                   "你好{name}！我们聊点什么呢？",
		"not_execute":                  "我现在做不到这件事。",
		"not_execute2":                 "我现在有点忙，等一会儿再问我吧。",
		"not_execute3":                 "现在不行，我正在忙别的事。",
		"not_execute4":                 "我们稍后再试试吧。",
		"speak_notconnect_homestation": "这件事需要我的充电座，可是我找不到它。",
		"default":                      "抱歉，我现在做不到这件事。",
		"dont_understand":              "抱歉，我没听清楚。",
		"weather_unavailable":          "我现在查不了天气。",
		"news_unavailable":             "我现在获取不了新闻。",
	},
}

type prompts map[string]map[string]string

func loadPrompts(path string) (prompts, error) {
	p := prompts{}
	for lang, m := range defaultPrompts {
		p[lang] = map[string]string{}
		for k, v := range m {
			p[lang][k] = v
		}
	}
	if path == "" {
		return p, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var extra map[string]map[string]string
	if err := json.Unmarshal(b, &extra); err != nil {
		return nil, err
	}
	for lang, m := range extra {
		if p[lang] == nil {
			p[lang] = map[string]string{}
		}
		for k, v := range m {
			p[lang][k] = v
		}
	}
	return p, nil
}

// text returns the prompt for (lang, key), falling back to English and then to the generic line.
func (p prompts) text(lang, key string) string {
	for _, l := range []string{lang, "en"} {
		if v, ok := p[l][key]; ok {
			return v
		}
	}
	if v, ok := p[lang]["default"]; ok {
		return v
	}
	return p["en"]["default"]
}

// baseLang turns "en-c", "en_US", "EN" into "en".
func baseLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	for i, c := range code {
		if c == '-' || c == '_' {
			return code[:i]
		}
	}
	if code == "" {
		return "en"
	}
	return code
}

// ---- intent rules ----

var nonLetter = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeText(s string) string {
	return strings.TrimSpace(nonLetter.ReplaceAllString(strings.ToLower(s), " "))
}

var goodbye = regexp.MustCompile(`\b(bye|goodbye|good bye|stop chatting|end chat|end the chat|that s all|exit)\b`)

func isGoodbye(text string) bool { return goodbye.MatchString(normalizeText(text)) }

// tidyReply keeps spoken replies short and plain: no markdown or emoji, bounded length.
// Reply clean-up patterns: markdown bold keeps its words, a *single-starred action* is dropped whole (it is not speech),
// and a free-standing emoticon is dropped. Emoticons must stand alone so "3:30" or "(about 8)" survive.
var (
	mdBold           = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	stageAction      = regexp.MustCompile(`\*[^*\n]{1,80}\*`)
	emoticon         = regexp.MustCompile(`(^|\s)(?:[:;=][-^']?[)(\][dDpPoO3/|]+|<3+|[xX][dD]+|\^_?\^)([\s.,!?]|$)`)
	spaceBeforePunct = regexp.MustCompile(`\s+([.,!?])`)
	curlyQuotes      = strings.NewReplacer("‘", "'", "’", "'", "“", `"`, "”", `"`)
)

func tidyReply(s string) string {
	s = mdBold.ReplaceAllString(s, "$1")
	s = stageAction.ReplaceAllString(s, " ")
	for i := 0; i < 2; i++ { // twice: adjacent emoticons share the space between them
		s = emoticon.ReplaceAllString(s, "$1$2")
	}
	s = curlyQuotes.Replace(s)
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '*' || r == '#' || r == '`' || r == '_':
			return -1
		case r >= 0x1F000 || (r >= 0x2600 && r <= 0x27BF):
			return -1
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	s = spaceBeforePunct.ReplaceAllString(s, "$1") // "funny ^_^." -> "funny ."  -> "funny."
	const max = 300
	r := []rune(s)
	if len(r) > max {
		cut := max
		for i := max - 1; i >= 40; i-- {
			if r[i] == '.' || r[i] == '!' || r[i] == '?' {
				cut = i + 1
				break
			}
		}
		s = strings.TrimSpace(string(r[:cut]))
	}
	return s
}
