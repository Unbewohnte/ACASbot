package bot

import (
	"Unbewohnte/ACATbot/inference"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ArticleContent struct {
	Title   string
	Content string
	Success bool
}

type Bot struct {
	api   *tgbotapi.BotAPI
	conf  Config
	model *inference.Inference
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

func (bot *Bot) Start() error {
	log.Printf("Бот авторизован как %s", bot.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		if update.Message.Text == "help" || update.Message.Text == "start" {
			bot.Help(update.Message)
		} else if strings.HasPrefix(update.Message.Text, "changeorg") {
			bot.ChangeOrg(update.Message)
		} else {
			// Обработка URL
			bot.Analyze(update.Message)
		}
	}

	return nil
}
