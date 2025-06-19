package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Command struct {
	Name        string
	Description string
	Example     string
	Call        func(*tgbotapi.Message) error
}

func (bot *Bot) NewCommand(cmd Command) {
	bot.commands = append(bot.commands, cmd)
}

func (bot *Bot) Init() {
	bot.NewCommand(Command{
		Name:        "help",
		Description: "Напечатать вспомогательное сообщение",
		Call:        bot.Help,
	})

	bot.NewCommand(Command{
		Name:        "changeorg",
		Description: "Изменить имя организации, отношение к которой будет анализировано.",
		Example:     "changeorg Человечество",
		Call:        bot.ChangeOrg,
	})

	bot.NewCommand(Command{
		Name:        "toggleanalysis",
		Description: "Включить или выключить полный анализ статей",
		Call:        bot.ToggleAnalysis,
	})

	bot.NewCommand(Command{
		Name:        "do",
		Description: "Анализировать статью",
		Example:     "do https://example.com/article2",
		Call:        bot.Do,
	})
}

func (bot *Bot) Help(message *tgbotapi.Message) error {
	var helpMessage string

	for _, command := range bot.commands {
		helpMessage += fmt.Sprintf("\nКоманда: \"%s\"\nОписание: %s\n", command.Name, command.Description)
		if command.Example != "" {
			helpMessage += fmt.Sprintf("Пример: %s\n", command.Example)
		}
	}

	_, err := bot.api.Send(
		tgbotapi.NewMessage(
			message.Chat.ID,
			helpMessage,
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

func (bot *Bot) Do(message *tgbotapi.Message) error {
	var err error = nil

	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Вы не указали URL",
		)
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.api.Send(msg)
		return err
	}

	url := parts[1]

	if strings.HasPrefix(url, "http") {
		bot.analyzeArticle(url, message)
	} else {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Пожалуйста, отправьте действительный URL, начинающийся с http/https",
		)
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.api.Send(msg)
	}

	return err
}

func (bot *Bot) ToggleAnalysis(message *tgbotapi.Message) error {
	var err error = nil

	if bot.conf.FullAnalysis {
		bot.conf.FullAnalysis = false
		_, err = bot.api.Send(tgbotapi.NewMessage(message.Chat.ID, "Полный анализ выключен"))
	} else {
		bot.conf.FullAnalysis = true
		_, err = bot.api.Send(tgbotapi.NewMessage(message.Chat.ID, "Полный анализ включен"))
	}

	return err
}
