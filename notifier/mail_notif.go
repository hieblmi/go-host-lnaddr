package notifier

import (
	"fmt"
	"net"
	"net/smtp"
	"time"
)

type MailNotifier struct {
	Cfg       Config
	To        string
	From      string
	Server    string
	Login     string
	Password  string
	MinAmount uint64
}

var _ Notifier = (*MailNotifier)(nil)

func NewMailNotifier(cfg Config) *MailNotifier {
	return &MailNotifier{
		Cfg:       cfg,
		To:        cfg.Params["Target"],
		From:      cfg.Params["From"],
		Server:    cfg.Params["SmtpServer"],
		Login:     cfg.Params["Login"],
		Password:  cfg.Params["Password"],
		MinAmount: cfg.MinAmount,
	}
}

func (m *MailNotifier) Notify(amount uint64, comment string) (err error) {
	if amount < m.MinAmount {
		return fmt.Errorf("amount is too small, required %d got %d",
			m.MinAmount, amount)
	}

	var host string
	host, _, err = net.SplitHostPort(m.Server)
	if err != nil {
		return
	}

	auth := smtp.PlainAuth("", m.Login, m.Password, host)
	if comment != "" {
		comment = fmt.Sprintf("Sender said: \"%s\"", comment)
	}
	body := fmt.Sprintf("Date: %s\nFrom: %s\nSubject: %s\n\nYou've "+
		"received %d sats to your lightning address. %s",
		time.Now().Format(time.RFC1123Z), m.From, "lnaddress payment",
		amount, comment)

	err = smtp.SendMail(
		m.Server, auth, m.From, []string{m.To}, []byte(body),
	)

	return
}

func (m *MailNotifier) Target() string {
	return fmt.Sprintf("%s => %s", m.From, m.To)
}
