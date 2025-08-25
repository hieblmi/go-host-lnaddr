package main

type notifierConfig struct {
	Type      string
	MinAmount uint64
	Params    map[string]string
}

type notifier interface {
	Notify(amount uint64, comment string) error
	Target() string
}

var notifiers []notifier

func setupNotifiers(cfg ServerConfig) {
	for _, c := range cfg.Notifiers {
		switch c.Type {
		case "mail":
			notifiers = append(
				notifiers, NewMailNotificator(c),
			)

		case "http":
			notifiers = append(
				notifiers, NewHttpNotificator(c),
			)

		case "telegram":
			notifiers = append(
				notifiers, NewTelegramNotificator(c),
			)
		}
	}
}

func broadcastNotification(amount uint64, comment string) {
	log.Infof("Received %d sats with comment: %s", amount, comment)
	for _, n := range notifiers {
		err := n.Notify(amount, comment)
		if err != nil {
			log.Infof("Error sending notification to %s: %s",
				n.Target(), err)
		} else {
			log.Infof("Notification sent to %s", n.Target())
		}
	}
}
