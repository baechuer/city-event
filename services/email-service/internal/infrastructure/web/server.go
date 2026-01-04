package web

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/rs/zerolog"

	"github.com/baechuer/real-time-ressys/services/email-service/internal/infrastructure/web/middleware"
)

type Server struct {
	addr     string
	authBase string
	lg       zerolog.Logger
	srv      *http.Server
	client   *http.Client

	rl *middleware.RedisRateLimiter
}

type RateLimitConfig struct {
	Enabled bool

	IPLimit     int
	IPWindow    time.Duration
	TokenLimit  int
	TokenWindow time.Duration
}

type Config struct {
	Addr     string // ":8090"
	AuthBase string // "http://localhost:8080"

	RedisPool *redis.Pool

	RateLimit RateLimitConfig
}

func NewServer(cfg Config, lg zerolog.Logger) *Server {
	mux := http.NewServeMux()

	s := &Server{
		addr:     cfg.Addr,
		authBase: strings.TrimRight(cfg.AuthBase, "/"),
		lg:       lg.With().Str("component", "email_web").Logger(),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// rate limiter (optional)
	if cfg.RateLimit.Enabled && cfg.RedisPool != nil {
		s.rl = middleware.NewRedisRateLimiter(cfg.RedisPool, middleware.RedisRateLimitConfig{
			Enabled:     cfg.RateLimit.Enabled,
			IPLimit:     cfg.RateLimit.IPLimit,
			IPWindow:    cfg.RateLimit.IPWindow,
			TokenLimit:  cfg.RateLimit.TokenLimit,
			TokenWindow: cfg.RateLimit.TokenWindow,
			KeyPrefix:   "rl:email_web",
		}, s.lg)
		s.lg.Info().
			Bool("enabled", cfg.RateLimit.Enabled).
			Int("ip_limit", cfg.RateLimit.IPLimit).
			Dur("ip_window", cfg.RateLimit.IPWindow).
			Int("token_limit", cfg.RateLimit.TokenLimit).
			Dur("token_window", cfg.RateLimit.TokenWindow).
			Msg("rate limiting configured (redis)")
	} else {
		s.lg.Info().Msg("rate limiting disabled (redis)")
	}

	// health checks
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// pages
	mux.HandleFunc("/verify", s.handleVerifyPage)
	mux.HandleFunc("/reset", s.handleResetPage)

	// APIs with RL wrappers
	if s.rl != nil {
		mux.HandleFunc("/api/verify", s.rl.WrapJSONTokenEndpoint("api_verify", s.handleAPIVerify))
		mux.HandleFunc("/api/reset/validate", s.rl.WrapJSONTokenEndpoint("api_reset_validate", s.handleAPIResetValidate))
		mux.HandleFunc("/api/reset/confirm", s.rl.WrapJSONTokenEndpoint("api_reset_confirm", s.handleAPIResetConfirm))
	} else {
		mux.HandleFunc("/api/verify", s.handleAPIVerify)
		mux.HandleFunc("/api/reset/validate", s.handleAPIResetValidate)
		mux.HandleFunc("/api/reset/confirm", s.handleAPIResetConfirm)
	}

	s.srv = &http.Server{Addr: s.addr, Handler: mux}
	return s
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.Stop(context.Background())
	}()

	s.lg.Info().Str("addr", s.addr).Msg("email web server listening")
	err := s.srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Stop(ctx context.Context) error {
	s.lg.Info().Msg("email web server shutting down")
	return s.srv.Shutdown(ctx)
}

func tokenFromQuery(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func writeHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, html)
}

// ---------------- Pages ----------------

func (s *Server) handleVerifyPage(w http.ResponseWriter, r *http.Request) {
	token := template.HTMLEscapeString(tokenFromQuery(r))
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	writeHTML(w, `<!doctype html><html><body>
<h3>Verifying your email…</h3>
<pre id="out"></pre>
<script>
(async () => {
  const res = await fetch('/api/verify', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body: JSON.stringify({token: '`+token+`'})
  });
  const text = await res.text();
  document.getElementById('out').textContent = text || ('HTTP ' + res.status);
})();
</script>
</body></html>`)
}

func (s *Server) handleResetPage(w http.ResponseWriter, r *http.Request) {
	token := template.HTMLEscapeString(tokenFromQuery(r))
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	writeHTML(w, `<!doctype html><html><body>
<h3>Reset your password</h3>
<div id="status">Checking token…</div>
<div id="form" style="display:none;">
  <input id="pw" type="password" placeholder="New password (>=12 chars)" />
  <button id="btn">Submit</button>
</div>
<pre id="out"></pre>

<script>
const token = '`+token+`';

async function validate() {
  const res = await fetch('/api/reset/validate', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body: JSON.stringify({token})
  });
  const text = await res.text();
  if (!res.ok) {
    document.getElementById('status').textContent = 'Invalid or expired token (HTTP ' + res.status + ')';
    document.getElementById('out').textContent = text;
    return;
  }
  document.getElementById('status').textContent = 'Token valid. Enter a new password.';
  document.getElementById('form').style.display = 'block';
  document.getElementById('out').textContent = text;
}

document.getElementById('btn').onclick = async () => {
  const pw = document.getElementById('pw').value || '';
  const res = await fetch('/api/reset/confirm', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body: JSON.stringify({token, new_password: pw})
  });
  const text = await res.text();
  document.getElementById('out').textContent = text || ('HTTP ' + res.status);
};

validate();
</script>
</body></html>`)
}

// ---------------- APIs ----------------

type verifyReq struct {
	Token string `json:"token"`
}

type resetValidateReq struct {
	Token string `json:"token"`
}

type resetConfirmReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (s *Server) handleAPIVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	target := s.authBase + "/auth/v1/verify-email/confirm"
	status, body, err := s.postJSON(r.Context(), target, map[string]string{"token": req.Token})
	if err != nil {
		http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (s *Server) handleAPIResetValidate(w http.ResponseWriter, r *http.Request) {
	var req resetValidateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	u, _ := url.Parse(s.authBase + "/auth/v1/password/reset/validate")
	q := u.Query()
	q.Set("token", req.Token)
	u.RawQuery = q.Encode()

	status, body, err := s.get(r.Context(), u.String())
	if err != nil {
		http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (s *Server) handleAPIResetConfirm(w http.ResponseWriter, r *http.Request) {
	var req resetConfirmReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	req.NewPassword = strings.TrimSpace(req.NewPassword)

	if req.Token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}
	if req.NewPassword == "" {
		http.Error(w, "missing new_password", http.StatusBadRequest)
		return
	}

	target := s.authBase + "/auth/v1/password/reset/confirm"
	status, body, err := s.postJSON(r.Context(), target, map[string]string{
		"token":        req.Token,
		"new_password": req.NewPassword,
	})
	if err != nil {
		http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// ---------------- proxy helpers ----------------

func (s *Server) postJSON(ctx context.Context, target string, payload any) (int, []byte, error) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)
	s.lg.Info().Int("status", resp.StatusCode).Str("target", target).Msg("proxy POST")
	return resp.StatusCode, out, nil
}

func (s *Server) get(ctx context.Context, target string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)
	s.lg.Info().Int("status", resp.StatusCode).Str("target", target).Msg("proxy GET")
	return resp.StatusCode, out, nil
}
