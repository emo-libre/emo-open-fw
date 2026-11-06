# Robot ↔ server API: time, token, weather, speech, conversation (clean layer)

What the *stock* firmware sends and what it needs back, so a server of your own can stand in for the vendor's.
Sources: public `emo-libre/Documentation` (Server pages, commit `007a02066241`, tagged **[pub]**) and the owner's private
analysis of the response handler (tagged **[priv]**, single-sourced). **[V]** = not yet confirmed against the owner's own robot —
use the server's capture mode (`server/README.md`) to confirm.

## Transport and headers
- HTTPS, TLS 1.2, **certificate not checked**; every request carries `Authorization: Bearer <jwt>` and a `Secret:` header
  (22 characters, changes each second; derivation not used or needed here). `/time` and `/token` carry no bearer. [pub]
- The server's `queryId` / `resultCode` values need not come from any particular issuer. [pub]
- Generated speech is downloaded by the robot over **plain HTTP** from the URL the server returns. [pub] Whether a non-default
  port in that URL is accepted: [V].

## Request kinds the response handler distinguishes [priv]
| kind | request | reply must contain |
|---|---|---|
| 1, 0xF | `POST /emo/voice/detectintent`, `POST /emo/ai/imgrecog` | an intent reply (below). **Kind 1 is dropped as stale unless the top-level `index` equals the request's `index`.** The reply is also ignored unless the robot is in its listening state. |
| 2 | `GET /emo/speech/tts?q=<text>&l=<lang>` | `errmessage == "ok"` and `url`; otherwise the robot shows an error state. [pub shape] |
| 3 | `GET /emo/weather/forecast?city=&later=&lon=&lat=` | `{"weather":{"main":<s>,"temp_c":<number>}}` [pub] |
| 4, 5 | `GET /emo/dialogue/general?l=`, `/emo/dialogue/weather?l=&city=` | `errmessage == "ok"` and a `dialogue` array; for each element the robot speaks one line itself and sends a second line to its partner robot. The two per-element key names are **[U]** (not in any source we hold); not implemented, the server answers 501. Capture a request and inspect a real reply to learn them. |
| 0xC, 0xD | `POST /emo/ai/draw/imgtoimg?steps=&cfg_scale=&denoising_strength=&sampler_index=&prompt=` (body: the photo, JPEG, chunked) and `POST /emo/ai/draw/txttoimg?steps=&prompt=` | `errmessage == "ok"` and the picture as base64 in `data`. The parameters are Stable Diffusion WebUI's. Image format the robot accepts: **[V]** (we default to PNG; JPEG and size limits are configurable). |
| 0x2C (utilities `news`) | intent reply | `utilities`/`news`: `{count, urls:[{id, url}]}`, each url an MP3 read out in turn. Up to 20 entries fit the robot's buffer; we send at most 10. [pub/priv] |
| 0x10 | `GET /emo/chat/start?lang=&name=&tz=` | `responsetag == "chatstart"`, `errcode == 0`, `url` (an MP3 greeting). `name` is omitted when the robot does not know it. |
| 0x11 | `POST /emo/voice/voiceprompt?languagecode=&type=&index=` | `responsetag == "voiceprompt"`; if `errmessage == "ok"` then `url` (an MP3 line), else no url. `type` is the robot's own refusal/event name (e.g. the "can't do that now" family); non-English units ask the server to speak it. We send both `errmsg` and `errmessage` as `"ok"`. [V for the exact field set] |
| 6 | `GET /emo/ota/version` | see `ota_wire.md` |

## Weather
- `main` is one of `sunny`, `cloudy`, `rain`, `snow`, `dust`, `fog`; the robot maps it to a localized phrase. [priv]
- `temp_c` is **Celsius**. The robot converts for display according to its own temperature-unit setting, so the server never
  converts. A temperature class (cold < 15, mild 15–30, hot > 30) and a rare "weather greeting" (rain or snow, or ≤ 0 °C or ≥ 30 °C)
  are derived on the robot. [priv]
- `later` = hours ahead, 0 or 12. `lon`/`lat` are sent as `0.00000` and unused. [pub]
- "No data": `{"errcode":0,"errmsg":"api error"}`. [priv]

## Voice upload (`detectintent`) [pub]
`POST`, `Content-Type: application/octet-stream`, usually `Transfer-Encoding: chunked`. Query: `locale`, `timezone`, `languagecode`
(`en-c` on 3.x), `lon`, `lat`, `alwaysReply`, `index`, `source`, and `role=chatgpt` while in a conversation. Body:
`0x22 | PCM s16le mono 16000 Hz | 0x22`; conversation turns are capped at 48,128 samples (≈ 3 s).

**How long the robot waits for the reply [priv, I; ≥ 8 s observed].** While the upload is in flight the robot shows a "thinking" face and
waits in its listening state. That wait restarts while the robot sees activity (recording) and ends after about **10 s** without
any; a reply that arrives after it ends is ignored (the listening-state rule in the table above), and the robot goes back to idle.
The upload itself streams in real time and stops after at most 12 s of audio. The transport has no read timeout of its own: the
long-lived connection simply waits. **Server budget: answer within about 10 s of the end of the upload**, and less is safer —
everything (speech-to-text, the model, and any speech that is synthesised before the reply is sent) counts. Single-sourced and
inferred from a first-pass analysis.
*Observed on the owner's robot (2026-11-05, 8 model replies):* every reply that was ready within **8.0 s of the end of the
upload** was accepted (the robot fetched its speech at once), including one that arrived **12.0 s after the upload began**, so
the window is counted from the end of the upload, not from wake-up. The exact cut-off (≈ 10 s?) is still unmeasured: set
`debug_reply_delay_ms` to find it.

## Intent reply
```
{"queryId":"<uuid>","queryResult":{"resultCode":"<uuid>","queryText":"<heard>","intent":{"name":"<n>","confidence":1},
  "rec_behavior":"<b>","behavior_paras":{...}},"languageCode":"en","index":<echo>}
```
Behaviours used by this server (names/params per `Documentation/Behaviors`; **[V]** on a real robot):
| rec_behavior | behavior_paras | meaning |
|---|---|---|
| `speak` | `url` (MP3), `txt`, `listen` (1 = keep listening), optional `pre_animation`, `post_animation`, `sentiment` | say something |
| `play_animation` | `animation_name` | play a named animation; `chatgpt_end` leaves conversation mode; `dont_understand` is the "didn't get that" reaction |
| `power_off`, `explore`, `dance` | `{}` | simple commands |
| `featured_game` | `game_name: "chatgpt"` | enter conversation mode (the robot then calls `chat/start`) |
While in conversation mode, a `play_animation` reply with `animation_name: chatgpt_end` (or a different featured game) makes the robot
leave it; that is how the server ends a chat. [priv]

## Push keys
Any reply may carry `switch`, `type`+`screen_info`, `version-num`, `notice`, `weather`. The robot reads each only when it is waiting
for it, so unrelated replies must not add these keys (our `weather` reply has exactly one top-level key). [priv]

## To pin on the owner's robot (capture mode)
Voice-prompt reply fields and `type` values · whether kind 0xF requires `index` · dialogue (kinds 4/5) and drawing (0xC/0xD) formats ·
the plain-HTTP speech request (headers, ranges, port) · `listen` semantics · any other path the robot or its dock requests
(`/home/...`, `/app/...`) · whether MP3 sample rate/bitrate matters to the player.

## Intent catalogue the local server produces (names/params per `Documentation/Intents` and `Behaviors`; [V] unless noted)
| intent | rec_behavior | behavior_paras |
|---|---|---|
| greeting | play_animation | `animation_name: Hi` [pub] |
| animal | play_animation | `animation_name: Fox/Cattle/Snake/Cat/Dog` [pub] |
| power_off / explore / be_quiet(stay_still) / look_at_me(tree_search_and_interact) | power_off / explore / stay_still / tree_search_and_interact | `{}` [pub] |
| play_by_yourself | play_around | `play_type: normal` [pub] |
| dance / dance_with_lights / listen_to_music | dance | `{}` / `dance_type: with_lights` / `dance_type: to_music` [pub] |
| forward, left, right, turn_around (+ backward [priv]) | basic_move | `type` [pub] |
| go_home / go_off_charger | go_home / go_off_charger | `{}` [priv] |
| about_age, about_lucky_number, about_name, about_health… | about_emo | `type: age / lucky_number / about_name / color / sign / birthday / happy_birthday / about_health / about_comfort` [pub/priv] |
| my_name_is / who_am_i / my_birthday_is | tree_search_and_update_name | `name` [pub] / `{}` / `birth_date: "M.D"` (month first; the robot reads `%d.%d`) [priv] |
| smalltalk_yes / smalltalk_no / user_choose_number | *(empty)* | `{}` — the robot decides from its own question state; for number answers it also inspects `queryText` [pub/priv] |
| time, date, day, battery | utilities | `utility_type` = `time` (with `time: {}`) / `date` / `day` / `battery` [pub/priv] |
| check_update / update_firmware | utilities | `utility_type: update`, `update.operation: check` / `update` [pub] |
| timer_set | utilities | `utility_type: timer`, `timer: {operation: set, duration: <seconds>}` [pub] |
| alarm_set / alarm_check / alarm_remove | utilities | `utility_type: alarm`, `alarm: {operation: set, time: "HH:MM", recurrence: ONCE\|SUN…SAT\|WDAY\|WKND\|ED, tag}`; `{operation: check}`; `{operation: remove, index: ""\|"n"}` [pub/priv] |
| turn_on_light / turn_off_light | utilities | `utility_type: light`, `light: {operation: set_hsl, hsl: {"0":h 0-330,"1":s,"2":l}, room: ""}` / `{operation: off, room: ""}` [pub] |
| volume_to | utilities | `utility_type: volume`, `volume: {operation: up\|down\|max\|mid\|min\|mute}` [pub: max/mid/min/mute; priv: up/down] |
| weather | utilities | `utility_type: weather`, `weather: {main, temp}` (note `temp`, two decimals, not `temp_c`) [pub] |
| take_photo / take_photo_recog / show_photo | utilities | `utility_type` = the same word [priv] |
| featured_game_* | featured_game | `game_name`: angry_emo, rps, shot, flip_a_coin, ttt, lucky_fruit, roll_a_dice, fix_bugs, paint, fireworks, zombie, sticker, battle, exercise, camping, magic, pose, drawing, show_something, snake, chatgpt [pub/priv] |
| play_music | play_music | `type: instrument, action: piano\|drum\|trumpet\|random\|dance_together`; `type: sing`; `type: bluetooth, action: enter` [priv] |
Known cloud quirks we deliberately do not copy: `alarm_check` answering "shut the alarm", brightness commands answering `dont_understand`,
bare words (e.g. "Rush") being taken as names — the server has no conversation state, so only explicit forms are accepted.
