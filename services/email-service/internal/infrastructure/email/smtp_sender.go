package email

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/wneessen/go-mail"
)

type SMTPSender struct {
	lg zerolog.Logger

	host     string
	port     int
	user     string
	pass     string
	from     string
	insecure bool

	timeout time.Duration
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	Timeout  time.Duration
	Insecure bool
}

func NewSMTPSender(cfg SMTPConfig, lg zerolog.Logger) *SMTPSender {
	return &SMTPSender{
		lg:       lg.With().Str("component", "smtp_sender").Logger(),
		host:     cfg.Host,
		port:     cfg.Port,
		user:     cfg.Username,
		pass:     cfg.Password,
		from:     cfg.From,
		insecure: cfg.Insecure,
		timeout:  cfg.Timeout,
	}
}

func (s *SMTPSender) SendVerifyEmail(ctx context.Context, toEmail, url string) error {
	subject := "Verify your email"
	text := fmt.Sprintf("Verify your email by opening this link:\n\n%s\n", url)
	htmlBody := renderBasicHTML(
		"Verify your email",
		"Click the button below to verify your email address.",
		"Verify email",
		url,
	)
	return s.send(ctx, toEmail, subject, text, htmlBody)
}

func (s *SMTPSender) SendPasswordReset(ctx context.Context, toEmail, url string) error {
	subject := "Reset your password"
	text := fmt.Sprintf("Reset your password by opening this link:\n\n%s\n", url)
	htmlBody := renderBasicHTML(
		"Reset your password",
		"Click the button below to reset your password.",
		"Reset password",
		url,
	)
	return s.send(ctx, toEmail, subject, text, htmlBody)
}

func (s *SMTPSender) SendEventCanceled(ctx context.Context, toEmail, eventID, reason string) error {
	subject := "Event Canceled"
	text := fmt.Sprintf("Your registered event (%s) has been canceled.\nReason: %s", eventID, reason)
	return s.send(ctx, toEmail, subject, text, "")
}

func (s *SMTPSender) SendEventUnpublished(ctx context.Context, toEmail, eventID, reason string) error {
	subject := "Event Unpublished"
	text := fmt.Sprintf("Your event (%s) has been unpublished by a moderator.\nReason: %s", eventID, reason)
	return s.send(ctx, toEmail, subject, text, "")
}

func (s *SMTPSender) send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	m := mail.NewMsg()
	if err := m.From(s.from); err != nil {
		return PermanentError{msg: "invalid from address: " + err.Error()}
	}
	if err := m.To(to); err != nil {
		return PermanentError{msg: "invalid to address: " + err.Error()}
	}
	m.Subject(subject)

	// Text fallback + HTML alternative
	m.SetBodyString(mail.TypeTextPlain, textBody)
	m.AddAlternativeString(mail.TypeTextHTML, htmlBody)

	tlsPolicy := mail.TLSMandatory
	if s.insecure {
		tlsPolicy = mail.TLSOpportunistic
	}

	opts := []mail.Option{
		mail.WithPort(s.port),
		mail.WithTLSPolicy(tlsPolicy),
	}
	if s.user != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(s.user), mail.WithPassword(s.pass))
	}

	c, err := mail.NewClient(s.host, opts...)
	if err != nil {
		return PermanentError{msg: "smtp client init failed: " + err.Error()}
	}

	s.lg.Info().Str("host", s.host).Int("port", s.port).Str("to", to).Str("subject", subject).Msg("attempting smtp send")
	if err := c.DialAndSendWithContext(ctx, m); err != nil {
		s.lg.Error().Err(err).Str("to", to).Msg("smtp send failed")

		msg := err.Error()
		if containsAny(msg, "535", "5.7.8", "authentication", "Username and Password not accepted") {
			return PermanentError{msg: "smtp auth failed: " + msg}
		}
		return TemporaryError{msg: "smtp transient failure: " + msg}
	}

	s.lg.Info().Str("to", to).Msg("smtp send ok")
	return nil
}

func renderBasicHTML(title, intro, buttonText, link string) string {
	// minimal safe escaping
	escLink := html.EscapeString(link)
	escTitle := html.EscapeString(title)
	escIntro := html.EscapeString(intro)
	escBtn := html.EscapeString(buttonText)

	// very simple inline HTML (works in Gmail)
	return `<!doctype html>
<html>
  <body style="font-family:Arial,Helvetica,sans-serif; line-height:1.4;">
    <h2>` + escTitle + `</h2>
    <p>` + escIntro + `</p>

    <p>
      <a href="` + escLink + `" style="display:inline-block; padding:10px 14px; text-decoration:none; border-radius:6px; background:#111; color:#fff;">
        ` + escBtn + `
      </a>
    </p>

    <p style="color:#555; font-size:12px;">
      If the button doesn't work, open this link:<br/>
      <a href="` + escLink + `">` + escLink + `</a>
    </p>
  </body>
</html>`
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if x != "" && strings.Contains(s, x) {
			return true
		}
	}
	return false
}
