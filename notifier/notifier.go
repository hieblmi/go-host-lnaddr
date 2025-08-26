package notifier

import "github.com/btcsuite/btclog"

var log btclog.Logger

type Config struct {
	Type      string
	MinAmount uint64
	Params    map[string]string
}

type Notifier interface {
	Notify(amount uint64, comment string) error
	Target() string
}

var notifiers []Notifier

func SetupNotifiers(notifierConfigs []Config, logger btclog.Logger) {

	log = logger

	for _, c := range notifierConfigs {
		switch c.Type {
		case "mail":
			notifiers = append(
				notifiers, NewMailNotifier(c),
			)

		case "http":
			notifiers = append(
				notifiers, NewHttpNotifier(c),
			)

		case "telegram":
			notifiers = append(
				notifiers, NewTelegramNotifier(c),
			)

		default:
			log.Infof("Unknown notifier type: %s", c.Type)
		}
	}
}

func BroadcastNotification(amount uint64, comment string) {
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
