package notifier

import (
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type TelegramNotifier struct {
	Cfg       Config
	ChatId    int64
	Token     string
	MinAmount uint64
}

var _ Notifier = (*TelegramNotifier)(nil)

func NewTelegramNotifier(cfg Config) *TelegramNotifier {
	chatId, _ := strconv.ParseInt(cfg.Params["ChatId"], 10, 64)
	return &TelegramNotifier{
		Cfg:       cfg,
		ChatId:    chatId,
		Token:     cfg.Params["Token"],
		MinAmount: cfg.MinAmount,
	}
}

func (t *TelegramNotifier) Notify(amount uint64,
	comment string) (err error) {

	if amount < t.MinAmount {
		return fmt.Errorf("amount is too small, required %d got %d",
			t.MinAmount, amount)
	}
	if comment != "" {
		comment = fmt.Sprintf("Sender said: \"%s\"", comment)
	}
	tgBot, err := tgbotapi.NewBotAPI(t.Token)
	if err != nil {
		log.Warnf("Couldn't create telegram bot api")
		return err
	}
	body := fmt.Sprintf("Subject: lnaddress payment\n\nYou've "+
		"received %d sats to your lightning address. %s", amount,
		comment)

	tgMessage := tgbotapi.NewMessage(t.ChatId, body)
	_, err = tgBot.Send(tgMessage)

	return err
}

func (t *TelegramNotifier) Target() string {
	return fmt.Sprintf("ChatId: %d", t.ChatId)
}
