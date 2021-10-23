package main

import "log"

type notificatorConfig struct {
	Type      string
	Target    string
	MinAmount uint64
	Params    map[string]string
}

type notificator interface {
	Notify(amount uint64, comment string) error
	Target() string
}

var notificators []notificator

func setupNotificators(cfg Config) {
	for _, c := range cfg.Notificators {
		switch c.Type {
		case "mail":
			notificators = append(notificators, NewMailNotificator(c))
		case "http":
			notificators = append(notificators, NewHttpNotificator(c))
		}
	}
}

func broadcastNotification(amount uint64, comment string) {
	log.Printf("Received %d sats with comment: %s", amount, comment)
	for _, n := range notificators {
		err := n.Notify(amount, comment)
		if err != nil {
			log.Printf("Error sending notification to %s: %s", n.Target(), err)
		} else {
			log.Printf("Notification sent to %s", n.Target())
		}
	}
}
