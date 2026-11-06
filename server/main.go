// Command emoserver is the owner's local replacement for the robot's update/time/token server.
// It makes no outbound connections of any kind.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// denyTransport makes any accidental outbound HTTP call fail loudly.
type denyTransport struct{}

func (denyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("emoserver: outbound connections are disabled")
}

func main() {
	confFile := flag.String("c", "", "config file (JSON); defaults are used if omitted")
	flag.Parse()

	http.DefaultTransport = denyTransport{}
	log.SetFlags(log.LstdFlags)

	path := *confFile
	if path == "" {
		if _, err := os.Stat("config.json"); err == nil {
			path = "config.json"
		} else if len(flag.Args()) == 0 {
			path = "config.json" // first run: make one
		}
	}
	if path != "" {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := createStarterConfig(path); err != nil {
				log.Fatalf("could not write a first config to %s: %v", path, err)
			}
			log.Printf("FIRST RUN: wrote a starter config to %s. Everything is off except the admin page; finish setup there.", path)
		}
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	key, err := loadOrCreateKey(cfg.DataDir)
	if err != nil {
		log.Fatalf("token key: %v", err)
	}
	s, err := newServer(cfg, key)
	if err != nil {
		log.Fatalf("setup: %v", err)
	}
	s.configPath = path
	h, err := capturing(cfg.Capture, s.routes())
	if err != nil {
		log.Fatalf("capture: %v", err)
	}
	if cfg.Capture.Dir != "" {
		log.Printf("CAPTURE ON: recording requests to %s (bodies=%v). Authorization/Secret values are not recorded.", cfg.Capture.Dir, cfg.Capture.Bodies)
	}

	if s.dns, err = startDNS(cfg); err != nil {
		log.Fatalf("%v", err)
	}
	logDNS(s.dns)
	if !cfg.NoPrewarm {
		go s.warmModel(context.Background())
		go s.prewarm(context.Background())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var servers []*http.Server
	errc := make(chan error, 3)
	start := func(srv *http.Server, tlsOn bool) {
		servers = append(servers, srv)
		go func() {
			if tlsOn {
				errc <- srv.ListenAndServeTLS("", "")
			} else {
				errc <- srv.ListenAndServe()
			}
		}()
	}
	if s.newAdminToken {
		log.Printf("ADMIN TOKEN (shown once; it is kept in %s): %s", filepath.Join(cfg.DataDir, "admin_token"), s.adminTok)
	}
	if cfg.ListenAdmin != "" || cfg.ListenAdminTLS != "" {
		log.Printf("OPEN THE ADMIN PAGE from this computer or any device on your network:")
		for _, u := range s.adminURLs() {
			log.Printf("    %s", u)
		}
		if cfg.ListenAdmin != "" {
			if host, _, err := net.SplitHostPort(cfg.ListenAdmin); err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
				log.Printf("note: the http:// address is not encrypted - fine on a home network you trust; use the https:// one (self-signed, accept the warning) otherwise, or set listen_admin to 127.0.0.1:8090 to keep the page on this computer only.")
			}
			start(&http.Server{Addr: cfg.ListenAdmin, Handler: s.adminRoutes(), ReadHeaderTimeout: 10 * time.Second}, false)
		}
		if cfg.ListenAdminTLS != "" {
			cert, err := ensureCert(cfg.DataDir, cfg.PublicHost)
			if err != nil {
				log.Fatalf("certificate: %v", err)
			}
			start(&http.Server{Addr: cfg.ListenAdminTLS, Handler: s.adminRoutes(), TLSConfig: tlsConfig(cert, false), ReadHeaderTimeout: 10 * time.Second}, true)
		}
	} else if len(s.adminTok) > 0 {
		log.Printf("OPEN THE ADMIN PAGE: https://<this machine>%s/admin/ui (self-signed certificate)", cfg.Listen)
	}
	if cfg.ListenHTTP != "" {
		log.Printf("plain HTTP on %s (development only)", cfg.ListenHTTP)
		start(&http.Server{Addr: cfg.ListenHTTP, Handler: h}, false)
	}
	if cfg.ListenAudio != "" && s.be.TTS != nil {
		log.Printf("speech downloads (plain HTTP) on %s, advertised as %s", cfg.ListenAudio, cfg.AudioBaseURL)
		start(&http.Server{Addr: cfg.ListenAudio, Handler: s.audioRoutes()}, false)
	}
	if cfg.Listen != "" {
		cert, err := ensureCert(cfg.DataDir, cfg.PublicHost)
		if err != nil {
			log.Fatalf("certificate: %v", err)
		}
		log.Printf("HTTPS on %s, serving %q as version %d", cfg.Listen, cfg.ESP32File, cfg.VersionNum)
		start(&http.Server{Addr: cfg.Listen, Handler: h, TLSConfig: tlsConfig(cert, cfg.LegacyCiphers)}, true)
	}
	select {
	case err := <-errc:
		log.Fatal(err)
	case <-ctx.Done():
		log.Print("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, srv := range servers {
			_ = srv.Shutdown(sctx)
		}
		s.closeAll()
	}
}
