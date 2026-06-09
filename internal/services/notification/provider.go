package notification

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	ID      string
	To      string
	Subject string
	Body    string
}

type ProviderResult struct {
	ProviderMessageID string
}

type Provider interface {
	Send(context.Context, Message) (ProviderResult, error)
}

type SMTPProvider struct {
	Addr string
	From string
}

func NewSMTPProvider(addr string) SMTPProvider {
	return SMTPProvider{
		Addr: strings.TrimSpace(addr),
		From: "no-reply@cityevents.local",
	}
}

func (p SMTPProvider) Send(ctx context.Context, msg Message) (ProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return ProviderResult{}, err
	}
	body := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", p.From, msg.To, msg.Subject, msg.Body))
	if err := smtp.SendMail(p.Addr, nil, p.From, []string{msg.To}, body); err != nil {
		return ProviderResult{}, err
	}
	return ProviderResult{ProviderMessageID: msg.ID + "-" + time.Now().UTC().Format("20060102150405")}, nil
}
