package notifier

import (
	"errors"
	"testing"
)

func TestTelegramNotifier_MinAmount(t *testing.T) {
	cfg := Config{
		Type:      "telegram",
		MinAmount: 500,
		Params: map[string]string{
			"ChatId": "12345",
			"Token":  "dummy-token",
		},
	}

	n := NewTelegramNotifier(cfg)

	if err := n.Notify(100, "hi"); err == nil {
		t.Fatalf("expected error for amount below MinAmount, got nil")
	}
}

func TestTelegramNotifier_Target(t *testing.T) {
	cfg := Config{
		Type: "telegram",
		Params: map[string]string{
			"ChatId": "6789",
			"Token":  "dummy",
		},
	}

	n := NewTelegramNotifier(cfg)

	target := n.Target()
	expected := "ChatId: 6789"
	if target != expected {
		t.Fatalf("unexpected target. want %q got %q", expected, target)
	}
}

// compile-time assertion to ensure interface satisfaction stays intact
func TestTelegramNotifier_ImplementsInterface(t *testing.T) {
	var _ Notifier = (*TelegramNotifier)(nil)
	if false {
		t.Fatal(errors.New("unreachable"))
	}
}
