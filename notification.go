package main

import "log"

type notificatorConfig struct {
	Type   string
	Target string
	Params map[string]string
}

type notificator interface {
	Notify(amount uint) error
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

func broadcastNotification(amount uint) {
	for _, n := range notificators {
		err := n.Notify(amount)
		if err != nil {
			log.Printf("Error sending notification to %s: %s", n.Target(), err)
		}
	}
}
