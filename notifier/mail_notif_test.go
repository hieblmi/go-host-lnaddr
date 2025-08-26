package notifier

import (
	"errors"
	"strings"
	"testing"
)

func TestMailNotifier_MinAmount(t *testing.T) {
	cfg := Config{
		Type:      "mail",
		MinAmount: 100,
		Params: map[string]string{
			"Target":     "to@example.com",
			"From":       "from@example.com",
			"SmtpServer": "localhost:2525",
			"Login":      "user",
			"Password":   "pass",
		},
	}

	n := NewMailNotifier(cfg)

	err := n.Notify(50, "hello")
	if err == nil {
		t.Fatalf("expected error for amount below MinAmount, got nil")
	}
	if !strings.Contains(err.Error(), "amount is too small") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMailNotifier_Target(t *testing.T) {
	cfg := Config{
		Type: "mail",
		Params: map[string]string{
			"Target":     "recipient@example.com",
			"From":       "sender@example.com",
			"SmtpServer": "smtp.example.com:587",
			"Login":      "user",
			"Password":   "pass",
		},
	}

	n := NewMailNotifier(cfg)

	target := n.Target()
	expected := "sender@example.com => recipient@example.com"
	if target != expected {
		t.Fatalf("unexpected target. want %q got %q", expected, target)
	}
}

// compile-time assertion to ensure interface satisfaction stays intact
func TestMailNotifier_ImplementsInterface(t *testing.T) {
	var _ Notifier = (*MailNotifier)(nil)
	if false {
		t.Fatal(errors.New("unreachable"))
	}
}
