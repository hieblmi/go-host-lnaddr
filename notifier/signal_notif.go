package notifier

import (
	"fmt"
	"os/exec"
)

type SignalNotifier struct {
	Cfg       Config
	FromPhone string
	ToPhone   string
	MinAmount uint64
}

var _ Notifier = (*SignalNotifier)(nil)

func NewSignalNotifier(cfg Config) *SignalNotifier {
	return &SignalNotifier{
		Cfg:       cfg,
		FromPhone: cfg.Params["FromPhoneNumber"],
		ToPhone:   cfg.Params["ToPhoneNumber"],
		MinAmount: cfg.MinAmount,
	}
}

func (s *SignalNotifier) Notify(amount uint64, comment string) error {
	if amount < s.MinAmount {
		return fmt.Errorf("amount is too small, required %d got %d",
			s.MinAmount, amount)
	}

	if comment != "" {
		comment = fmt.Sprintf("Sender said: \"%s\"", comment)
	}

	msg := fmt.Sprintf("Subject: lnaddress payment\n\nYou've received %d "+
		"sats to your lightning address. %s", amount, comment)

	// Prepare: signal-cli -a FromPhoneNumber send -m "message" ToPhoneNumber
	cmd := exec.Command("signal-cli", "-a", s.FromPhone, "send", "-m", msg,
		s.ToPhone)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("signal-cli failed: %w (%s)", err,
			string(output))
	}

	return nil
}

func (s *SignalNotifier) Target() string {
	return fmt.Sprintf("%s => %s", s.FromPhone, s.ToPhone)
}
