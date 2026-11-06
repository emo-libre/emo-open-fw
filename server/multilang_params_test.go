package main

import (
	"reflect"
	"testing"
)

// Parameterised commands in each language, with values different from the fixtures, checked against the English meaning.
// {native, english}: both must parse to the same command and parameters.
var paramCases = map[string][][2]string{
	"de": {
		{"Setze einen Timer für 25 Minuten", "set a timer for 25 minutes"}, {"Setze einen Timer für fünf Minuten", "set a timer for five minutes"},
		{"Timer für eine halbe Stunde", "set a timer for half an hour"}, {"Timer für 2 Stunden", "set a timer for 2 hours"},
		{"Stelle einen Wecker auf 6 Uhr 30 morgens", "set an alarm for 6:30 am"}, {"Wecker um 22 Uhr jeden Montag", "set an alarm at 10 pm every monday"},
		{"Erinnere mich um 9 Uhr an die Pille", "set an alarm called pill at 9 am"}, {"Schalte den zweiten Alarm aus", "turn off second alarm"},
		{"Entferne den dritten Alarm", "remove third alarm"}, {"Ändere deine Augenfarbe auf blau", "set eye color to blue"}, {"Grüne Lichter", "green lights"},
		{"Was sagt der Frosch", "what does the frog say"}, {"Ich wähle 9", "i choose 9"}, {"Mein Geburtstag ist am 24. Dezember", "my birthday is december 24th"},
		{"Ich heiße Anna", "my name is anna"},
	},
	"fr": {
		{"Crée un minuteur de 25 minutes", "set a timer for 25 minutes"}, {"Mets un minuteur de cinq minutes", "set a timer for five minutes"},
		{"Minuteur d'une heure", "set a timer for 1 hour"}, {"Mets une alarme à 6h30 du matin", "set an alarm for 6:30 am"},
		{"Mets une alarme à 22 heures tous les lundis", "set an alarm at 10 pm every monday"}, {"Désactive la deuxième alarme", "turn off second alarm"},
		{"Supprime la troisième alarme", "remove third alarm"}, {"Change la couleur de tes yeux en bleu", "set eye color to blue"}, {"Lumière verte", "green lights"},
		{"Que dit la grenouille", "what does the frog say"}, {"Je choisis 9", "i choose 9"}, {"Mon anniversaire est le 24 décembre", "my birthday is december 24th"},
		{"Je m'appelle Marie", "my name is marie"}, {"Rappelle-moi de manger à 9 heures", "set an alarm called eat at 9 am"},
	},
	"es": {
		{"Pon un temporizador de 25 minutos", "set a timer for 25 minutes"}, {"Pon un temporizador de cinco minutos", "set a timer for five minutes"},
		{"Pon una alarma a las 6 y media de la mañana", "set an alarm for 6:30 am"}, {"Configura una alarma a las 10 de la noche todos los lunes", "set an alarm at 10 pm every monday"},
		{"Apaga la segunda alarma", "turn off second alarm"}, {"Elimina la tercera alarma", "remove third alarm"}, {"Cambia el color de los ojos a azul", "set eye color to blue"},
		{"Luz verde", "green lights"}, {"Qué dice la rana", "what does the frog say"}, {"Elijo 9", "i choose 9"}, {"Mi cumpleaños es el 24 de diciembre", "my birthday is december 24th"},
		{"Me llamo Lucía", "my name is lucia"}, {"Pon una alarma de pastilla a las 9 de la mañana", "set an alarm called pill at 9 am"},
	},
	"it": {
		{"Imposta un timer di 25 minuti", "set a timer for 25 minutes"}, {"Imposta un timer di cinque minuti", "set a timer for five minutes"},
		{"Imposta una sveglia alle 6:30 di mattina", "set an alarm for 6:30 am"}, {"Imposta una sveglia alle 22:00 ogni lunedì", "set an alarm at 10 pm every monday"},
		{"Disattiva la seconda sveglia", "turn off second alarm"}, {"Togli la terza sveglia", "remove third alarm"}, {"Imposta gli occhi di colore blu", "set eye color to blue"},
		{"Luce verde", "green lights"}, {"Come fa la rana", "what does the frog say"}, {"Io scelgo 9", "i choose 9"}, {"Il mio compleanno è il 24 dicembre", "my birthday is december 24th"},
		{"Mi chiamo Giulia", "my name is giulia"},
	},
	"pt": {
		{"Coloque um cronômetro para vinte e cinco minutos", "set a timer for 25 minutes"}, {"Coloque um cronômetro para cinco minutos", "set a timer for five minutes"},
		{"Defina um alarme para às seis horas e trinta da manhã", "set an alarm for 6:30 am"}, {"Defina um alarme às 22 horas toda segunda-feira", "set an alarm at 10 pm every monday"},
		{"Desligue o segundo alarme", "turn off second alarm"}, {"Remova o terceiro alarme", "remove third alarm"}, {"Defina a cor dos olhos para azul", "set eye color to blue"},
		{"Luz verde", "green lights"}, {"O que o sapo diz", "what does the frog say"}, {"Eu escolho 9", "i choose 9"}, {"Meu aniversário é 24 de dezembro", "my birthday is december 24th"},
		{"Meu nome é Ana", "my name is ana"}, {"Me lembre de comer às nove horas da manhã", "set an alarm called eat at 9 am"},
	},
	"ru": {
		{"Поставь таймер на 25 минут", "set a timer for 25 minutes"}, {"Поставь таймер на пять минут", "set a timer for five minutes"},
		{"Поставь таймер на один час", "set a timer for 1 hour"}, {"Поставь будильник на 6:30 утра", "set an alarm for 6:30 am"},
		{"Поставь будильник на 22:00 на понедельник", "set an alarm at 10 pm every monday"}, {"Выключи второй будильник", "turn off second alarm"},
		{"Удали третий будильник", "remove third alarm"}, {"Поменяй цвет глаз на синий", "set eye color to blue"}, {"Включи зелёный свет", "green lights"},
		{"Что делает лягушка", "what does the frog say"}, {"Я выбираю девять", "i choose 9"}, {"У меня день рождения двадцать четвертого декабря", "my birthday is december 24th"},
		{"Меня зовут Анна", "my name is anna"}, {"Какая погода в Москве", "what's the weather in moscow"},
	},
	"ja": {
		{"タイマー25分", "set a timer for 25 minutes"}, {"タイマーを五分", "set a timer for five minutes"}, {"タイマー二時間", "set a timer for 2 hours"},
		{"朝6時半にアラームをセット", "set an alarm for 6:30 am"}, {"毎週月曜日の午後10時にアラームをセット", "set an alarm at 10 pm every monday"},
		{"二番目のアラームを切って", "turn off second alarm"}, {"三番目のアラームを削除", "remove third alarm"}, {"目の色を青に変えて", "set eye color to blue"},
		{"ライトを緑にして", "green lights"}, {"蛙は何て鳴く", "what does the frog say"}, {"九番", "i choose 9"}, {"私の誕生日は12月24日です", "my birthday is december 24th"},
		{"私の名前はアンナです", "my name is anna"},
	},
	"ko": {
		{"이십오분 후에 타이머 설정", "set a timer for 25 minutes"}, {"5분 후에 타이머 설정", "set a timer for five minutes"}, {"두 시간 후에 타이머 설정", "set a timer for 2 hours"},
		{"오전 여섯시 삼십분에 알람 설정", "set an alarm for 6:30 am"}, {"월요일 오후 10시 알람 설정", "set an alarm at 10 pm every monday"},
		{"두번째 알람 꺼줘", "turn off second alarm"}, {"세번째 알람 삭제", "remove third alarm"}, {"눈을 파란색으로 설정", "set eye color to blue"},
		{"초록색 조명", "green lights"}, {"개구리는 어떻게 울어?", "what does the frog say"}, {"나는 구번", "i choose 9"}, {"내 생일은 십이월 이십사일이야", "my birthday is december 24th"},
		{"내 이름은 안나야", "my name is anna"}, {"오전 열두시 알람 설정", "set an alarm for 12 am"},
	},
	"zh": {
		{"设置一个二十五分钟的倒计时", "set a timer for 25 minutes"}, {"设置一个五分钟的倒计时", "set a timer for five minutes"}, {"设置一个两小时的倒计时", "set a timer for 2 hours"},
		{"设置一个早上六点半的闹钟", "set an alarm for 6:30 am"}, {"设置一个每周一晚上十点的闹钟", "set an alarm at 10 pm every monday"},
		{"关闭第二个闹钟", "turn off second alarm"}, {"删除第三个闹钟", "remove third alarm"}, {"眼睛变成蓝色", "set eye color to blue"},
		{"打开绿色的灯", "green lights"}, {"青蛙怎么叫", "what does the frog say"}, {"我选九", "i choose 9"}, {"我的生日是十二月二十四号", "my birthday is december 24th"},
		{"我叫安娜", "my name is anna"}, {"设置一个每天早上八点的闹钟", "set an alarm at 8 am every day"},
	},
}

func TestParameterisedCommandsInEveryLanguage(t *testing.T) {
	s := testServer(t)
	for lang, cases := range paramCases {
		for _, c := range cases {
			en, ok1 := s.parse(c[1], "en")
			got, ok2 := s.parse(c[0], lang)
			if !ok1 {
				t.Errorf("english %q not understood (bad test case)", c[1])
				continue
			}
			if !ok2 {
				t.Errorf("%s: %q not understood, want %s %v", lang, c[0], en.Intent, en.Paras)
				continue
			}
			want, have := norm(en), norm(got)
			if lang == "zh" || lang == "ja" || lang == "ko" || lang == "ru" || true { // names differ only by case/script: compare the rest
				if want["i"] == "my_name_is" {
					delete(want["p"].(map[string]any), "name")
					delete(have["p"].(map[string]any), "name")
					if got.Paras["name"] == "" {
						t.Errorf("%s: %q gave no name", lang, c[0])
					}
				}
			}
			if !reflect.DeepEqual(want, have) {
				t.Errorf("%s: %q\n   want %v\n   got  %v", lang, c[0], want, have)
			}
		}
	}
}

func TestCitiesInEveryLanguage(t *testing.T) {
	s := testServer(t)
	for lang, say := range map[string]map[string]string{
		"de": {"Wie ist das Wetter in Berlin heute": "Berlin", "Wetter in München": "München"},
		"fr": {"Quelle est la météo à Paris aujourd'hui": "Paris", "Météo à Lyon": "Lyon"},
		"es": {"Cuál es el tiempo en Madrid hoy": "Madrid", "El clima en Sevilla": "Sevilla"},
		"it": {"Com'è il meteo a Milano oggi": "Milano", "Meteo a Roma": "Roma"},
		"pt": {"Como está o clima em Lisboa hoje": "Lisboa", "Tempo em São Paulo": "São Paulo"},
		"ru": {"Какая погода в Москве": "Москва", "Какая погода в Казани сегодня": "Казань", "Погода в Берлине": "Берлин"},
		"ja": {"東京の天気は？": "東京", "大阪の天気を教えて": "大阪", "ニューヨークの天気は": "ニューヨーク"},
		"ko": {"서울 날씨 알려 줘": "서울", "부산의 날씨": "부산", "오늘 서울 날씨 어때": "서울"},
		"zh": {"北京的天气": "北京", "请问上海天气怎么样": "上海", "伦敦的天气": "伦敦"},
	} {
		for say, city := range say {
			r, ok := s.parse(say, lang)
			if !ok || !r.NeedWeather || r.City != city {
				t.Errorf("%s: %q -> ok=%v weather=%v city=%q, want %q", lang, say, ok, r.NeedWeather, r.City, city)
			}
		}
	}
	// no city: the robot's own
	for lang, say := range map[string]string{"de": "Wie ist das Wetter", "fr": "Quel temps fait-il", "es": "Cuál es el tiempo", "it": "Com'è il meteo",
		"pt": "Como está o tempo", "ru": "Какая погода", "ja": "天気は？", "ko": "날씨 알려줘", "zh": "今天天气怎么样"} {
		if r, ok := s.parse(say, lang); !ok || !r.NeedWeather || r.City != "" {
			t.Errorf("%s: %q -> %+v ok=%v, want weather without a city", lang, say, r, ok)
		}
	}
}

func TestNamesKeepTheirScript(t *testing.T) {
	s := testServer(t)
	for lang, c := range map[string][2]string{"ja": {"私の名前は田中です", "田中"}, "zh": {"我叫李小明", "李小明"}, "ko": {"내 이름은 김민준이야", "김민준"},
		"ru": {"Меня зовут Иван", "Иван"}, "de": {"Ich heiße Jürgen", "Jürgen"}, "fr": {"Je m'appelle Hélène", "Hélène"}} {
		r, ok := s.parse(c[0], lang)
		if !ok || r.Paras["name"] != c[1] {
			t.Errorf("%s: %q -> %v ok=%v, want name %q", lang, c[0], r.Paras, ok, c[1])
		}
	}
}

func TestCJKNumerals(t *testing.T) {
	s := testServer(t)
	for lang, m := range map[string]map[string]int{
		"zh": {"十": 10, "十五": 15, "二十": 20, "二十五": 25, "九十九": 99, "一百": 100, "一百零五": 105, "两": 2, "三十": 30},
		"ja": {"十": 10, "十二": 12, "二十三": 23, "百": 100, "百五": 105},
		"ko": {"십": 10, "십이": 12, "이십오": 25, "구": 9, "일곱": 7, "열두": 12, "스물다섯": 25, "서른": 30, "한": 1, "백": 100},
	} {
		lx := s.packs[lang].lex
		for in, want := range m {
			tok := splitChars(in, true)
			if n, used := lx.number(tok, 0); n != want || used != len(tok) {
				t.Errorf("%s number(%q) = %d (used %d of %d), want %d", lang, in, n, used, len(tok), want)
			}
		}
	}
}
