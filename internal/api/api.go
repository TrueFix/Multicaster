package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/truefix/multicaster/internal/auth"
	"github.com/truefix/multicaster/internal/config"
	"github.com/truefix/multicaster/internal/engine"
	"github.com/truefix/multicaster/internal/systeminfo"
	"github.com/truefix/multicaster/internal/updates"
)

type Server struct {
	eng  *engine.Engine
	auth *auth.Manager
}

func New(eng *engine.Engine) *Server {
	return &Server{eng: eng, auth: auth.New(eng)}
}

func (s *Server) Router(static http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(3))
	r.Use(middleware.RequestSize(1024 * 1024))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Cache-Control", "no-store")
				if r.Method != "GET" && r.Method != "HEAD" {
					if origin := r.Header.Get("Origin"); origin != "" {
						u, e := url.Parse(origin)
						if e != nil || !strings.EqualFold(u.Host, r.Host) {
							http.Error(w, "Request must come from this dashboard", 403)
							return
						}
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	})

	r.Get("/api/v1/health", s.health)
	r.Get("/api/v1/auth/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"setup_required": s.auth.NeedsSetup(), "authenticated": s.auth.Authorized(r)})
	})
	r.Post("/api/v1/auth/login", s.login)
	r.Post("/api/v1/auth/setup", s.setup)
	r.Post("/api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		s.auth.Logout(r)
		auth.ClearCookie(w)
		w.WriteHeader(204)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.requireAdmin)
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, r)
			})
		})
		r.Get("/session", s.session)
		r.Get("/system", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, systeminfo.Snapshot(s.eng)) })
		r.Post("/auth/reset", func(w http.ResponseWriter, r *http.Request) {
			code, err := s.auth.Reset()
			if err != nil {
				s.authError(w, err)
				return
			}
			auth.ClearCookie(w)
			writeJSON(w, 200, map[string]string{"setup_token": code})
		})
		r.Get("/connection", func(w http.ResponseWriter, r *http.Request) {
			c := s.eng.Config()
			writeJSON(w, 200, map[string]any{"app": c.IngestApp(), "stream_key": c.Auth.StreamKey, "rtmp_listen": c.Server.RTMPListen})
		})
		r.Get("/updates", func(w http.ResponseWriter, r *http.Request) {
			settings := s.eng.Config().Settings
			info, err := updates.Check(r.Context(), settings.UpdateRepository, settings.UpdateManifestURL, r.URL.Query().Get("check") == "true")
			if err != nil {
				http.Error(w, err.Error(), 502)
				return
			}
			writeJSON(w, 200, info)
		})
		r.Get("/settings", s.getSettings)
		r.Get("/storage", func(w http.ResponseWriter, r *http.Request) {
			c := s.eng.Config().Storage
			has := c.SecretKey != ""
			c.SecretKey = ""
			writeJSON(w, 200, map[string]any{"config": c, "has_secret": has, "runtime": s.eng.Storage.Snapshot()})
		})
		r.Put("/storage", func(w http.ResponseWriter, r *http.Request) {
			var c config.Storage
			if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
				http.Error(w, "invalid JSON", 400)
				return
			}
			if err := s.eng.UpdateStorage(c); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			writeJSON(w, 200, map[string]bool{"saved": true})
		})
		r.Patch("/settings", s.patchSettings)
		r.Get("/destinations", s.listDestinations)
		r.Post("/destinations", s.createDestination)
		r.Patch("/destinations/{id}", s.patchDestination)
		r.Delete("/destinations/{id}", func(w http.ResponseWriter, r *http.Request) {
			if err := s.eng.DeleteDestination(chi.URLParam(r, "id")); err != nil {
				http.Error(w, err.Error(), 404)
				return
			}
			w.WriteHeader(204)
		})
	})

	if static != nil {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/app/", http.StatusFound)
		})
		r.Handle("/app/*", http.StripPrefix("/app", static))
	}

	return r
}

func (s *Server) hlsPath() string {
	if !s.eng.Config().Settings.WebStreamEnabled {
		return ""
	}
	return "/hls/index.m3u8"
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) session(w http.ResponseWriter, _ *http.Request) {
	snap := s.eng.Snapshot(s.hlsPath())
	snap.HLSStatus, snap.HLSError = s.eng.HLS.Status()
	status := s.eng.Storage.Snapshot()
	snap.CDNStatus = status.Status
	snap.CDNError = status.Message
	snap.CDNURL = status.PlaybackURL
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	cfg := s.eng.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"restream_enabled":    cfg.Settings.RestreamEnabled,
		"web_stream_enabled":  cfg.Settings.WebStreamEnabled,
		"performance_mode":    cfg.Settings.PerformanceMode,
		"ingest_app":          cfg.IngestApp(),
		"update_repository":   cfg.Settings.UpdateRepository,
		"update_manifest_url": cfg.Settings.UpdateManifestURL,
		"preview_enabled":     cfg.Settings.PreviewEnabled,
	})
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var body config.SettingsPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.UpdateManifestURL != nil {
		if err := updates.ValidateURL(*body.UpdateManifestURL); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	if body.UpdateRepository != nil {
		if !updates.ValidRepository(*body.UpdateRepository) {
			http.Error(w, "repository must be owner/name", 400)
			return
		}
	}
	cur, err := s.eng.PatchSettings(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cur)
}

func (s *Server) listDestinations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Config().Destinations)
}

func (s *Server) createDestination(w http.ResponseWriter, r *http.Request) {
	var d config.Destination
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if d.Name == "" || d.URL == "" {
		http.Error(w, "name and url required", http.StatusBadRequest)
		return
	}
	created, err := s.eng.AddDestination(d)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) patchDestination(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Enabled *bool   `json:"enabled"`
		Name    *string `json:"name"`
		URL     *string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.eng.PatchDestination(id, body.Enabled, body.Name, body.URL); err != nil {
		if err.Error() == "destination not found" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, d := range s.eng.Config().Destinations {
		if d.ID == id {
			writeJSON(w, http.StatusOK, d)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth.Authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (s *Server) authError(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, auth.ErrLimited) {
		status = 429
		w.Header().Set("Retry-After", "1")
	}
	if errors.Is(err, auth.ErrInvalid) {
		status = 401
	}
	http.Error(w, err.Error(), status)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
		http.Error(w, "Invalid login request", 400)
		return
	}
	token, err := s.auth.Login(body.Password)
	if err != nil {
		s.authError(w, err)
		return
	}
	auth.SetCookie(w, r, token)
	writeJSON(w, 200, map[string]bool{"authenticated": true})
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
		Code     string `json:"setup_token"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
		http.Error(w, "Invalid password setup request", 400)
		return
	}
	token, err := s.auth.Setup(body.Code, body.Password)
	if err != nil {
		s.authError(w, err)
		return
	}
	auth.SetCookie(w, r, token)
	writeJSON(w, 200, map[string]bool{"authenticated": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
