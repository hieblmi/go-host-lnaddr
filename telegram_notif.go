package main

import (
	"fmt"
	"log"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type telegramNotificator struct {
	ChatId    int64
	Token     string
	MinAmount uint64
}

var _ notificator = (*telegramNotificator)(nil)

func NewTelegramNotificator(cfg notificatorConfig) *telegramNotificator {
	chatId, _ := strconv.ParseInt(cfg.Params["ChatId"], 10, 64)
	return &telegramNotificator{ChatId: chatId, Token: cfg.Params["Token"],
		MinAmount: cfg.MinAmount}
}

func (t *telegramNotificator) Notify(amount uint64, comment string) (err error) {
	if amount < t.MinAmount {
		return fmt.Errorf("amount is too small, required %d got %d", t.MinAmount, amount)
	}
	if comment != "" {
		comment = fmt.Sprintf("Sender said: \"%s\"", comment)
	}
	tgBot, err := tgbotapi.NewBotAPI(t.Token)
	if err != nil {
		log.Println("Couldn't create telegram bot api")
		return err
	}
	body := fmt.Sprintf("Subject: %s\n\nYou've received %d sats to your lightning address. %s",
		"New lightning address payment", amount, comment)

	tgMessage := tgbotapi.NewMessage(t.ChatId, body)
	_, err = tgBot.Send(tgMessage)

	return err
}

func (t *telegramNotificator) Target() string {
	return fmt.Sprintf("ChatId: %s", t.ChatId)
}
