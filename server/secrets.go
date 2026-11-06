package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// API keys entered on the admin page. They live in data_dir/secrets.json (mode 0600), are exported to the process environment under
// the names the config uses (api_key_env), and are never sent back to any page: the page only learns which names are set.
// An environment variable of the same name that is already set wins, so a systemd unit or shell can still override the file.

var secretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,39}$`)

type secretStore struct {
	mu   sync.Mutex
	path string
	vals map[string]string
}

func loadSecrets(dataDir string) (*secretStore, error) {
	st := &secretStore{path: filepath.Join(dataDir, "secrets.json"), vals: map[string]string{}}
	b, err := os.ReadFile(st.path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &st.vals); err != nil {
		return nil, errors.New("secrets: " + st.path + " is not valid JSON")
	}
	for k, v := range st.vals {
		if secretName.MatchString(k) && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	return st, nil
}

func (st *secretStore) save() error {
	b, _ := json.MarshalIndent(st.vals, "", " ")
	return writeFileAtomic(st.path, b, 0o600)
}

func (st *secretStore) set(name, value string) error {
	if !secretName.MatchString(name) {
		return errors.New("a key name is 3-40 capital letters, digits or _, starting with a letter (for example GROQ_API_KEY)")
	}
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("a key is 1-512 characters on one line")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.vals[name] = value
	if err := st.save(); err != nil {
		return err
	}
	os.Setenv(name, value)
	return nil
}

func (st *secretStore) del(name string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if v, ok := st.vals[name]; ok {
		delete(st.vals, name)
		if os.Getenv(name) == v {
			os.Unsetenv(name)
		}
		return st.save()
	}
	return nil
}

// names lists the keys that are set, from the file or from the environment, never the values.
func (st *secretStore) names(wanted []string) map[string]string {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := map[string]string{}
	for k := range st.vals {
		out[k] = "saved here"
	}
	for _, k := range wanted {
		if _, ok := out[k]; !ok && os.Getenv(k) != "" {
			out[k] = "set in the environment"
		}
	}
	return out
}

func sortedNames(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// writeFileAtomic writes next to the target and renames, so a crash never leaves half a file.
func writeFileAtomic(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ---- admin token ----

// resolveAdminToken returns the admin token: the environment variable named by admin_token_env if it is set, otherwise (with
// admin_auto_token) the token kept in data_dir/admin_token, made on first use. created reports a freshly made token.
func resolveAdminToken(cfg Config) (tok []byte, created bool, err error) {
	if t := adminToken(cfg.AdminTokenEnv); len(t) > 0 {
		return t, false, nil
	}
	if !cfg.AdminAutoToken {
		return nil, false, nil
	}
	p := filepath.Join(cfg.DataDir, "admin_token")
	if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) >= 24 {
		return []byte(strings.TrimSpace(string(b))), false, nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, false, err
	}
	t := hex.EncodeToString(raw)
	if err := writeFileAtomic(p, []byte(t+"\n"), 0o600); err != nil {
		return nil, false, err
	}
	return []byte(t), true, nil
}

// ---- first run and this machine's address ----

// lanIPs lists this machine's private IPv4 addresses (the ones a robot on the same network could reach).
func lanIPs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ip := ipn.IP.To4(); ip != nil && ip.IsPrivate() {
				out = append(out, ip.String())
			}
		}
	}
	sort.Strings(out)
	return out
}

// starterConfig is what a first run writes: everything that needs no outside program, and the admin page ready to use.
func starterConfig() map[string]any {
	host := "192.168.1.10"
	if ips := lanIPs(); len(ips) > 0 {
		host = ips[0]
	}
	return map[string]any{
		"listen":           ":8443",
		"listen_audio":     ":8080",
		"public_host":      host + ":8443",
		"audio_base_url":   "http://" + host + ":8080",
		"listen_admin":     ":8090", // any device on the home network (a phone, a laptop) can open the page; the token is still required
		"listen_admin_tls": ":8444",
		"admin_auto_token": true,
		"admin_allow_edit": true,
		"free_chat":        true,
		"activity_log":     true,
	}
}

func createStarterConfig(path string) error {
	b, _ := json.MarshalIndent(starterConfig(), "", "  ")
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}
