# Presets

Starting points (the admin page's Setup tab applies them with one click). Or copy one to `config.json`, then set `public_host` and `audio_base_url` to this machine's LAN address.
API keys are read from environment variables whose NAMES appear in the file; never put a key in the file.

| File | What it uses | Needs |
|---|---|---|
| `local.json` | everything on your own machine: whisper.cpp for speech-to-text (the server starts and watches it; set its `model` path), Ollama for chat and (with `llava`) photo description, espeak-ng + lame for speech (`"type": "espeak"`, a voice per language), Stable Diffusion WebUI for drawing | those programs running. Weather uses the keyless Open-Meteo API, the only internet use. |
| `free-online.json` | Groq (speech-to-text and chat), Google Cloud Text-to-Speech, OpenWeatherMap, Pollinations (drawing), RSS news | `GROQ_API_KEY`, `GOOGLE_API_KEY`, `OPENWEATHER_API_KEY` |
| `anthropic.json` | Anthropic for chat and photo description (paid), Groq for speech-to-text, espeak-ng for speech, WeatherAPI.com | `ANTHROPIC_API_KEY`, `GROQ_API_KEY`, `WEATHERAPI_KEY` |

"Free" means the provider offered a free tier or free allowance when this was written. Terms, limits and model names change: check the
provider's page before relying on it, and edit the `model` fields to whatever is current. Anthropic's API has no free tier.

Other free or local options that need only configuration, no code:
- Gemini: `openai` type with base URL `https://generativelanguage.googleapis.com/v1beta/openai/`
- OpenRouter free models: `https://openrouter.ai/api/v1`
- any local llama.cpp, vLLM or LocalAI server: `openai` type
- whisper.cpp: `command` type, or its server started with an OpenAI-compatible inference path
- Piper or Kokoro: `command` type (text on stdin, audio to `{out}`, plus the same `convert_args` to make MP3)

Every preset is checked by a test that it loads, validates and builds its backends. That does not prove any service is reachable.
