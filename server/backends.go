package main

import (
	"fmt"

	"emoserver/backend"
)

// defaultLLMMaxTokens caps a model reply when the config sets no max_tokens: EMO speaks one or two short sentences,
// and a hard cap keeps a rambling model from producing a minute of speech.
const defaultLLMMaxTokens = 60

// CapConfig selects and configures one capability (speech-to-text, language model, text-to-speech, vision).
type CapConfig struct {
	Type     string                `json:"type"` // none | mock | openai | whispercpp | espeak | piper | anthropic | google | elevenlabs | command
	OpenAI   backend.OpenAIConfig  `json:"openai"`
	Command  backend.CommandConfig `json:"command"`
	MockText string                `json:"mock_text"` // type "mock": fixed STT transcript or LLM reply

	WhisperCpp backend.WhisperCppConfig `json:"whispercpp"` // stt: the server runs and watches whisper.cpp for you
	Espeak     backend.EspeakConfig     `json:"espeak"`     // tts: espeak-ng + lame, voice chosen by language
	Piper      backend.PiperConfig      `json:"piper"`      // tts: a Piper neural voice

	Anthropic  backend.AnthropicConfig  `json:"anthropic"`
	Google     backend.GoogleConfig     `json:"google"`
	ElevenLabs backend.ElevenLabsConfig `json:"elevenlabs"`
}

type WeatherConfig struct {
	Type      string                  `json:"type"` // none | static | open-meteo
	Main      string                  `json:"static_main"`
	TempC     float64                 `json:"static_temp_c"`
	OpenMeteo backend.OpenMeteoConfig `json:"open_meteo"`

	OpenWeatherMap backend.OpenWeatherMapConfig `json:"openweathermap"`
	WeatherAPI     backend.WeatherAPIConfig     `json:"weatherapi"`
}

type ImageConfig struct {
	Type    string                `json:"type"` // none | mock | sdwebui | openai | command
	SDWebUI backend.SDWebUIConfig `json:"sdwebui"`
	OpenAI  backend.OpenAIConfig  `json:"openai"`
	Command backend.CommandConfig `json:"command"`

	Pollinations backend.PollinationsConfig `json:"pollinations"`
}

type NewsConfig struct {
	Type  string            `json:"type"` // none | rss | mock
	RSS   backend.RSSConfig `json:"rss"`
	Lines []string          `json:"mock_lines"`
	Count int               `json:"count"` // headlines per request, default 3, max 10
}

type BackendsConfig struct {
	Image ImageConfig `json:"image"`
	News  NewsConfig  `json:"news"`
	STT   CapConfig   `json:"stt"`
	LLM   CapConfig   `json:"llm"`
	TTS   CapConfig   `json:"tts"`
	// TTSFallback is a fast voice (espeak, piper) used when the main voice cannot finish before the robot stops waiting.
	TTSFallback CapConfig     `json:"tts_fallback"`
	Vision      CapConfig     `json:"vision"`
	Weather     WeatherConfig `json:"weather"`
}

// buildTTS makes one voice; type "" or "none" gives nil.
func buildTTS(cc CapConfig) (backend.TTS, error) {
	switch cc.Type {
	case "", "none":
		return nil, nil
	case "mock":
		return backend.MockTTS{}, nil
	case "openai":
		return backend.NewOpenAITTS(cc.OpenAI)
	case "command":
		return backend.NewCommandTTS(cc.Command)
	case "google":
		return backend.NewGoogleTTS(cc.Google)
	case "elevenlabs":
		return backend.NewElevenLabs(cc.ElevenLabs)
	case "espeak":
		return backend.NewEspeakTTS(cc.Espeak)
	case "piper":
		return backend.NewPiperTTS(cc.Piper)
	}
	return nil, fmt.Errorf("unknown type %q", cc.Type)
}

func buildBackends(c BackendsConfig) (backend.Set, error) {
	var set backend.Set
	var err error

	switch c.STT.Type {
	case "", "none":
	case "mock":
		set.STT = backend.MockSTT{Text: c.STT.MockText}
	case "openai":
		set.STT, err = backend.NewOpenAISTT(c.STT.OpenAI)
	case "command":
		set.STT = backend.NewCommandSTT(c.STT.Command)
	case "google":
		set.STT, err = backend.NewGoogleSTT(c.STT.Google)
	case "whispercpp":
		set.STT, err = backend.NewWhisperCpp(c.STT.WhisperCpp)
	default:
		err = fmt.Errorf("stt: unknown type %q", c.STT.Type)
	}
	if err != nil {
		return set, fmt.Errorf("stt: %w", err)
	}

	llm := func(cc CapConfig, what string) (backend.LLM, error) {
		switch cc.Type {
		case "", "none":
			return nil, nil
		case "mock":
			return backend.MockLLM{Reply: cc.MockText}, nil
		case "openai":
			if cc.OpenAI.MaxTokens == 0 {
				cc.OpenAI.MaxTokens = defaultLLMMaxTokens
			}
			return backend.NewOpenAILLM(cc.OpenAI)
		case "anthropic":
			return backend.NewAnthropic(cc.Anthropic)
		}
		return nil, fmt.Errorf("%s: unknown type %q", what, cc.Type)
	}
	if set.LLM, err = llm(c.LLM, "llm"); err != nil {
		return set, err
	}
	if set.Vision, err = llm(c.Vision, "vision"); err != nil {
		return set, err
	}

	if set.TTS, err = buildTTS(c.TTS); err != nil {
		return set, fmt.Errorf("tts: %w", err)
	}
	if set.TTSFallback, err = buildTTS(c.TTSFallback); err != nil {
		return set, fmt.Errorf("tts_fallback: %w", err)
	}

	switch c.Weather.Type {
	case "", "none":
	case "static":
		set.Weather = backend.StaticWeather{F: backend.Forecast{Main: c.Weather.Main, TempC: c.Weather.TempC}}
	case "open-meteo":
		set.Weather, err = backend.NewOpenMeteo(c.Weather.OpenMeteo)
	case "openweathermap":
		set.Weather, err = backend.NewOpenWeatherMap(c.Weather.OpenWeatherMap)
	case "weatherapi":
		set.Weather, err = backend.NewWeatherAPI(c.Weather.WeatherAPI)
	default:
		err = fmt.Errorf("weather: unknown type %q", c.Weather.Type)
	}
	if err != nil {
		return set, fmt.Errorf("weather: %w", err)
	}
	switch c.Image.Type {
	case "", "none":
	case "mock":
		set.Image = backend.MockImage{}
	case "sdwebui":
		set.Image, err = backend.NewSDWebUI(c.Image.SDWebUI)
	case "openai":
		set.Image, err = backend.NewOpenAIImages(c.Image.OpenAI)
	case "command":
		set.Image = backend.NewCommandImage(c.Image.Command)
	case "pollinations":
		set.Image, err = backend.NewPollinations(c.Image.Pollinations)
	default:
		err = fmt.Errorf("image: unknown type %q", c.Image.Type)
	}
	if err != nil {
		return set, fmt.Errorf("image: %w", err)
	}

	switch c.News.Type {
	case "", "none":
	case "mock":
		set.News = backend.MockNews{Lines: c.News.Lines}
	case "rss":
		set.News, err = backend.NewRSS(c.News.RSS)
	default:
		err = fmt.Errorf("news: unknown type %q", c.News.Type)
	}
	if err != nil {
		return set, fmt.Errorf("news: %w", err)
	}
	return set, nil
}
