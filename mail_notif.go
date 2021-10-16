package main

import (
	"fmt"
	"net"
	"net/smtp"
)

type mailNotificator struct {
	To       string
	From     string
	Server   string
	Login    string
	Password string
}

var _ notificator = (*mailNotificator)(nil)

func NewMailNotificator(cfg notificatorConfig) *mailNotificator {
	return &mailNotificator{To: cfg.Target, From: cfg.Params["From"],
		Server: cfg.Params["SmtpServer"], Login: cfg.Params["Login"],
		Password: cfg.Params["Password"]}
}

func (m *mailNotificator) Notify(amount uint, comment string) (err error) {
	var host string
	host, _, err = net.SplitHostPort(m.Server)
	if err != nil {
		return
	}
	auth := smtp.PlainAuth("go-host-lnaddr", m.Login, m.Password, host)
	if comment != "" {
		comment = fmt.Sprintf("Sender said: \"%s\"", comment)
	}
	body := fmt.Sprintf("Subject: %s\n\nYou've received %d sats to your lightning address. %s",
		"New lightning address payment", amount, comment)
	err = smtp.SendMail(m.Server, auth, m.From, []string{m.To}, []byte(body))
	return
}

func (m *mailNotificator) Target() string {
	return fmt.Sprintf("%s => %s", m.From, m.To)
}