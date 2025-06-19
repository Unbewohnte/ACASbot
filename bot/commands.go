package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (bot *Bot) Help(message *tgbotapi.Message) error {
	_, err := bot.api.Send(
		tgbotapi.NewMessage(
			message.Chat.ID,
			fmt.Sprintf(
				"Отправьте ссылку на статью для ее анализа от лица \"%s\".\nНапишите \"changeorg [новое имя организации]\" для изменения организации, отношение к которой будет анализировано",
				bot.conf.OrganizationName,
			),
		),
	)

	return err
}

func (bot *Bot) ChangeOrg(message *tgbotapi.Message) error {
	parts := strings.Split(strings.TrimSpace(message.Text), " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Имя организации не указано",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return nil
	}
	bot.conf.OrganizationName = strings.Join(parts[1:], " ")
	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		fmt.Sprintf("Организация сменена на \"%s\"", bot.conf.OrganizationName),
	)
	msg.ReplyToMessageID = message.MessageID
	_, err := bot.api.Send(msg)

	return err
}

func (bot *Bot) Analyze(message *tgbotapi.Message) error {
	var err error = nil

	if strings.HasPrefix(message.Text, "http") {
		bot.analyzeArticle(message)
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, отправьте действительный URL, начинающийся с http/https")
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.api.Send(msg)
	}

	return err
}
