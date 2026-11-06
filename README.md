# EMO Open Server

Make your EMO desk robot work **without the vendor's cloud**. A small server runs on your own computer, and EMO listens,
thinks and speaks through it: no account, and nothing about you leaves your home.

| Part | State |
|---|---|
| `server/`: the local server EMO talks to | **Works today** with a real EMO on its normal (stock) firmware |
| `components/`, `main/`: open ESP32 firmware | **Not ready.** Never run on a robot yet. **Do not flash it.** |

> **Read this first: disclaimer**
> - This is an independent community project. It is **not made, endorsed or supported by Living AI**, the maker of EMO.
>   "EMO" is used only to name the robot this software works with; all trademarks belong to their owners.
> - It exists for **interoperability**: so a robot you own can talk to a server you run. The interface was documented from
>   the public [emo-libre documentation](https://github.com/emo-libre/Documentation) and by observing our own robots, only as
>   far as needed to make them work together (as allowed, for example, by EU Directive 2009/24/EC Art. 6 and US 17 U.S.C. § 1201(f)).
> - It contains **no vendor firmware, code, keys, sounds, animations or images**.
> - **Use at your own risk.** It comes with no warranty (see LICENSE). Changing how your robot connects may affect your
>   warranty or service from the maker. This is not legal advice.

---

## What you need

- An EMO robot on the same Wi-Fi network as your computer.
- A computer running **Linux** (Ubuntu/Debian or Arch). It stays on while you use EMO.
- About 15 minutes.

Every grey box below is a command. **Copy it, paste it into a terminal, press Enter.** Do the boxes in order.

---

## Step 1: Install the basics

**Ubuntu / Debian:**
```bash
sudo apt update
sudo apt install -y git golang-go espeak-ng lame cmake build-essential
```

**Arch:**
```bash
sudo pacman -S --needed git go espeak-ng lame cmake base-devel
```

Check that Go is new enough (it must say **go1.22** or higher):
```bash
go version
```
If it is older, install a newer Go from https://go.dev/dl/ and continue.

## Step 2: Download and build the server

```bash
cd ~
git clone https://github.com/emo-libre/emo-open-fw.git
cd ~/emo-open-fw/server
go build -o emoserver .
```

## Step 3: Start it

```bash
cd ~/emo-open-fw/server
./emoserver
```

The first time, it prints an **admin token** and an address. Open **http://127.0.0.1:8090/admin/ui** in your browser and
paste the token. Lost it? This shows it again:
```bash
cat ~/emo-open-fw/server/data/admin_token
```
Leave this terminal open: closing it stops the server. Use a **second terminal** for the next steps.

## Step 4: Give EMO ears, a brain and a voice

**The brain** (a chat model that runs on your computer, about 2 GB):
```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama pull llama3.2:3b
```

**The ears** (speech-to-text, about 150 MB):
```bash
cd ~
git clone https://github.com/ggml-org/whisper.cpp
cd ~/whisper.cpp
cmake -B build
cmake --build build -j --config Release
sh ./models/download-ggml-model.sh base.en
```

**The voice:** espeak-ng is already installed from Step 1. (A nicer voice, Piper, is explained in the admin page's **Help** tab.)

Now tell the server to use them. In the admin page, open **Setup**:
1. Under *Start from a ready-made set*, press **Use this** on **Everything on my own computer**.
2. Under *Listening*, set **Model file** to `~/whisper.cpp/models/ggml-base.en.bin` and **whisper-server program** to
   `~/whisper.cpp/build/bin/whisper-server` (type your real home folder instead of `~`, for example `/home/alex/...`).
3. Under *Chatting*, set **Model** to `llama3.2:3b`.
4. Press **Save settings**, then **Restart the server now**.

The **Home** tab now shows Listening, Thinking and Voice. Press **Check the engines now** to test them.

## Step 5: Point EMO at your computer

EMO looks for its server by name (`api.living.ai`) on port 443. Two small changes send it to your computer instead.

**1. The port.** Send port 443 to the server (run once; repeat after every reboot):
```bash
sudo nft add table ip emo
sudo nft add chain ip emo pre '{ type nat hook prerouting priority -100; }'
sudo nft add rule ip emo pre tcp dport 443 redirect to :8443
```

**2. The name.** Find your computer's address:
```bash
hostname -I
```
Use the first number (for example `192.168.1.23`). Then pick **one** of these:
- **Easiest, in your router:** add a DNS override (also called *local DNS* or *DNS rewrite*; Pi-hole, AdGuard Home and most
  routers have it) that points `api.living.ai` to that address.
- **Or let the server do it:** in the admin page open **Expert**, tick **Turn the DNS helper on**, put `api.living.ai` in
  *Name the robot asks for* and your router's address in the next box, **Save**, **Restart**. Then allow the server to use
  the DNS port:
  ```bash
  sudo setcap cap_net_bind_service=+ep "$(readlink -f ~/emo-open-fw/server/emoserver)"
  ```
  Finally, set your router's DHCP **DNS server** to your computer's address, so EMO asks your computer.

Turn EMO off and on again. After a minute the **Home** tab says **EMO is connected**.

## Step 6: Talk to EMO

Try:
- "What's the weather?"
- "Let's play tic tac toe"
- "How far away is the moon?"
- "ChatGPT mode", then chat, then "goodbye"

The **Activity** tab shows what EMO heard and how long each answer took.

---

## If something goes wrong

| Problem | Fix |
|---|---|
| The page asks for a token | `cat ~/emo-open-fw/server/data/admin_token` |
| Home says *EMO has not contacted this server yet* | Redo Step 5, then restart EMO. Your computer and EMO must be on the same network. |
| EMO shows its "don't understand" face | Open **Activity**: if *nothing heard*, speak closer; if the bar passes the dashed line, answers are too slow (use the smaller model, or Piper for the voice). |
| `address already in use` when starting | Another copy is running: `pkill emoserver`, then start again. |
| Engines show *Problem* | Open **Try it** and check each engine; the message says what is missing. |

More detail, every setting and the full API: [server/README.md](server/README.md).

## What is in this repository

| Folder | What it is |
|---|---|
| `server/` | The local server (Go, standard library only) and its admin page |
| `components/`, `main/` | Open ESP32 firmware, work in progress, not yet run on hardware |
| `spec-clean/` | The specification everything is written from (wire formats, timings, behaviour) |
| `tools/emotool/` | Helper tool: partition checks, capture summaries, a scan that keeps vendor material out |
| `tests/` | Host tests (`cmake -S tests/host -B build/host && cmake --build build/host && ctest --test-dir build/host`) |

## Licence

**GNU GPL v3.0 or later** (see [LICENSE](LICENSE)). You may use, change and share it, including commercially, as long as
what you share stays under the GPL with its source. Third-party parts keep their own licences; they are listed in [NOTICES](NOTICES).

## Thanks

- The **emo-libre** community for the public documentation and the original proxy (BSD-3-Clause).
- **voduylinh** for the Vietnamese voice commands.
- **ROBOTEKA**, credited by the robot's maker for the Russian command list that the Russian pack's coverage was checked against.
