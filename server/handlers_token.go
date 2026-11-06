package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const tokenLifetime = 3600 // seconds; the robot refreshes before expiry

var deviceID = regexp.MustCompile(`^[0-9a-f]{12}$`)

// loadOrCreateKey returns the HMAC key stored in dir/jwt.key, creating it (mode 0600) on first use.
func loadOrCreateKey(dir string) ([]byte, error) {
	p := filepath.Join(dir, "jwt.key")
	if b, err := os.ReadFile(p); err == nil && len(b) >= 32 {
		return b, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, os.WriteFile(p, k, 0o600)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// signJWT builds an HS256 token. The robot does not verify it; it stores and replays it.
func signJWT(key []byte, claims map[string]any) (string, error) {
	h, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "HS256"})
	c, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := b64(h) + "." + b64(c)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(signing))
	return signing + "." + b64(m.Sum(nil)), nil
}

// verifyJWT checks signature and expiry and returns the claims.
func verifyJWT(key []byte, tok string, now time.Time) (map[string]any, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(parts[0] + "." + parts[1]))
	want := b64(m.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, errors.New("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, err
	}
	if exp, ok := claims["exp"].(float64); !ok || int64(exp) < now.Unix() {
		return nil, errors.New("expired")
	}
	return claims, nil
}

type tokenReply struct {
	AccessToken string `json:"access_token"`
	ExpireIn    int    `json:"expire_in"`
	Type        string `json:"type"`
}

// tokenHandler answers GET /token/<device id>[?version=&version_name=]. The id is 12 lower-case hex characters
// (the robot's Bluetooth MAC without separators). Claims follow emo-libre/Documentation (token.md).
func (s *server) tokenHandler() http.HandlerFunc {
	key := s.key
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !deviceID.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"errcode": 400, "errmsg": "bad device id"})
			return
		}
		if !s.allowedDevice(id) {
			writeJSON(w, http.StatusForbidden, map[string]any{"code": 403, "errmessage": "device not allowed"})
			return
		}
		s.stats.seen(id)
		now := time.Now()
		claims := map[string]any{
			"sub": id,
			"iat": now.Unix(),
			"nbf": now.Unix(),
			"exp": now.Add(tokenLifetime * time.Second).Unix(),
		}
		if v := r.URL.Query().Get("version"); v != "" && len(v) <= 8 {
			claims["version"] = v
		}
		if n := r.URL.Query().Get("version_name"); n != "" && len(n) <= 16 {
			claims["name"] = n
		}
		tok, err := signJWT(key, claims)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"errcode": 500, "errmsg": "token error"})
			return
		}
		writeJSON(w, http.StatusOK, tokenReply{AccessToken: tok, ExpireIn: tokenLifetime, Type: "Bearer"})
	}
}
