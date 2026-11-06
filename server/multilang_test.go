package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// Each testdata/<lang>_pairs.txt lists the vendor's English commands next to the same command in another language. Most
// non-English lines come from the vendor's published per-language command lists; lines those lists lack were translated for this
// project (see each file's header). Every pair must mean exactly the same.

var pairLangs = []string{"de", "fr", "es", "it", "pt", "ru", "ja", "ko", "zh"}

// Languages whose text carries accents or similar marks that a sloppy transcript may drop.
var foldLangs = map[string]bool{"de": true, "fr": true, "es": true, "it": true, "pt": true, "ru": true}

func TestLanguagePairsMeanTheSameAsEnglish(t *testing.T) {
	s := testServer(t)
	for _, lang := range pairLangs {
		lang := lang
		t.Run(lang, func(t *testing.T) {
			pairs := loadPairsFrom(t, "testdata/"+lang+"_pairs.txt")
			if len(pairs) < 168 {
				t.Fatalf("only %d pairs", len(pairs))
			}
			bad := 0
			for _, p := range pairs {
				if unknownEverywhere[p.en] {
					if r, ok := s.parse(p.vi, lang); ok {
						t.Errorf("%q unexpectedly understood as %s", p.vi, r.Intent)
					}
					continue
				}
				en, ok1 := s.parse(p.en, "en")
				got, ok2 := s.parse(p.vi, lang)
				if !ok1 || !ok2 {
					t.Errorf("%q / %q: english ok=%v %s ok=%v", p.en, p.vi, ok1, lang, ok2)
					bad++
				} else if !reflect.DeepEqual(norm(en), norm(got)) {
					t.Errorf("%q => %q\n   english: %v\n   %s:      %v", p.en, p.vi, norm(en), lang, norm(got))
					bad++
				}
				if bad > 25 {
					t.Fatal("too many failures")
				}
			}
		})
	}
}

func TestLanguagePairsWithoutAccents(t *testing.T) {
	s := testServer(t)
	for lang := range foldLangs {
		for _, p := range loadPairsFrom(t, "testdata/"+lang+"_pairs.txt") {
			if unknownEverywhere[p.en] {
				continue
			}
			plain := foldDiacritics(p.vi)
			en, _ := s.parse(p.en, "en")
			got, ok := s.parse(plain, lang)
			if !ok || !reflect.DeepEqual(norm(en), norm(got)) {
				t.Errorf("%s accents stripped: %q -> ok=%v %v, want %v", lang, plain, ok, norm(got), norm(en))
			}
		}
	}
}

// Ordinary sentences must stay conversation in every language.
func TestLanguagesDoNotHijackConversation(t *testing.T) {
	s := testServer(t)
	say := map[string][]string{
		"de": {"Heute ist das Wetter wirklich schön, findest du nicht", "Erzähl mir eine Geschichte über einen Drachen", "Mein Hund heißt Max und ist sehr alt", "Warum ist der Himmel blau", "Ich habe gestern einen Film über Tanzen gesehen", "Meine Schwester arbeitet in einem Fotostudio", "Die Uhr in der Küche geht falsch"},
		"fr": {"Aujourd'hui il fait vraiment beau, tu ne trouves pas", "Raconte-moi une histoire de dragon", "Mon chien s'appelle Max et il est très vieux", "Pourquoi le ciel est bleu", "J'ai vu un film sur la danse hier soir", "Ma soeur travaille dans un studio photo", "La lumière du soleil est très forte aujourd'hui"},
		"es": {"Hoy hace un día precioso, no crees", "Cuéntame una historia sobre un dragón", "Mi perro se llama Max y es muy viejo", "Por qué el cielo es azul", "Ayer vi una película sobre el baile", "Mi hermana trabaja en un estudio de fotografía", "La luz del sol es muy fuerte hoy"},
		"it": {"Oggi è una giornata bellissima, non trovi", "Raccontami una storia su un drago", "Il mio cane si chiama Max ed è molto vecchio", "Perché il cielo è blu", "Ieri ho visto un film sul ballo", "Mia sorella lavora in uno studio fotografico", "La luce del sole è molto forte oggi"},
		"pt": {"Hoje está um dia lindo, não achas", "Conta-me uma história sobre um dragão", "O meu cão chama-se Max e é muito velho", "Porque é que o céu é azul", "Ontem vi um filme sobre dança", "A minha irmã trabalha num estúdio de fotografia", "A luz do sol está muito forte hoje"},
		"ru": {"Сегодня такая прекрасная погода, ты не находишь", "Расскажи мне сказку про дракона", "Мою собаку зовут Макс, и она очень старая", "Почему небо голубое", "Вчера я смотрел фильм про танцы", "Моя сестра работает в фотостудии", "Солнечный свет сегодня очень яркий"},
		"ja": {"今日はとても良い天気だと思わない", "ドラゴンについての物語を聞かせてください", "私の犬はとても年寄りです", "なぜ空は青いのですか", "昨日ダンスの映画を見ました", "姉は写真スタジオで働いています", "今日の日差しはとても強いですね"},
		"ko": {"오늘은 정말 날씨가 좋다고 생각하지 않니", "용에 대한 이야기를 들려줘", "우리 집 강아지는 아주 나이가 많아", "하늘은 왜 파란색이야", "어제 춤에 관한 영화를 봤어", "우리 누나는 사진관에서 일해", "오늘은 햇빛이 아주 강하네"},
		"zh": {"今天真是个好天气你觉得呢", "给我讲一个关于龙的故事", "我的小狗很老了而且很可爱", "为什么天空是蓝色的", "我昨天看了一部关于跳舞的电影", "我姐姐在照相馆工作", "今天的阳光很强烈", "我家的小猫昨天生病了"},
	}
	for lang, list := range say {
		for _, u := range list {
			if r, ok := s.parse(u, lang); ok {
				t.Errorf("%s: %q was hijacked: %s %v", lang, u, r.Intent, r.Paras)
			}
		}
	}
}

func TestPairFilesAreComplete(t *testing.T) {
	for _, lang := range pairLangs {
		b, err := os.ReadFile("testdata/" + lang + "_pairs.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "NOT been reviewed") && !strings.Contains(string(b), "not been reviewed") {
			t.Errorf("%s_pairs.txt must say which lines are unreviewed translations", lang)
		}
	}
}

// A phrase may belong to only one command in a language, or the winner would be an accident of ordering.
func TestPacksHaveNoAmbiguousPhrases(t *testing.T) {
	s := testServer(t)
	for lang, pl := range s.packs {
		seen := map[string]string{}
		for _, p := range pl.phrases {
			ph := strings.Join(p.tokens, " ")
			if prev, ok := seen[ph]; ok && prev != p.key {
				t.Errorf("%s: phrase %q is under both %s and %s", lang, ph, prev, p.key)
			}
			seen[ph] = p.key
		}
	}
}
