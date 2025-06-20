package bot

import (
	"Unbewohnte/ACATbot/inference"
	"Unbewohnte/ACATbot/spreadsheet"
	"context"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	conf     Config
	model    *inference.Inference
	commands []Command
	sheet    *spreadsheet.GoogleSheetsClient
}

func NewBot(config Config) (*Bot, error) {
	model, err := inference.NewInference(config.OllamaModel)
	if err != nil {
		return nil, err
	}

	api, err := tgbotapi.NewBotAPI(config.ApiToken)
	if err != nil {
		return nil, err
	}

	return &Bot{
		api:   api,
		conf:  config,
		model: model,
	}, nil
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

	if bot.conf.PushToGoogleSheet {
		sheetsClient, err := spreadsheet.NewGoogleSheetsClient(
			context.Background(),
			bot.conf.SheetConfig,
		)
		if err != nil {
			log.Panic(err)
		}

		bot.sheet = sheetsClient
	}
}

func (bot *Bot) Start() error {
	bot.Init()

	log.Printf("Бот авторизован как %s", bot.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		// Обработать команды
		update.Message.Text = strings.TrimSpace(update.Message.Text)
		for _, command := range bot.commands {
			if strings.HasPrefix(update.Message.Text, command.Name) {
				command.Call(update.Message)
				break // Дальше не продолжаем
			}
		}
	}

	return nil
}
