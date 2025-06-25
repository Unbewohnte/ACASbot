package bot

import (
	"Unbewohnte/ACASbot/conf"
	"Unbewohnte/ACASbot/inference"
	"Unbewohnte/ACASbot/spreadsheet"
	"context"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	conf     *conf.Config
	model    *inference.Inference
	commands []Command
	sheet    *spreadsheet.GoogleSheetsClient
}

func NewBot(config *conf.Config) (*Bot, error) {
	model, err := inference.NewInference(
		config.OllamaModel,
		config.OllamaQueryTimeoutSeconds,
	)
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

	bot.NewCommand(Command{
		Name:        "about",
		Description: "Напечатать информацию о боте",
		Call:        bot.About,
	})

	bot.NewCommand(Command{
		Name:        "togglepublic",
		Description: "Включить или выключить публичный/приватный доступ к боту",
		Call:        bot.TogglePublicity,
	})

	bot.NewCommand(Command{
		Name:        "adduser",
		Description: "Добавить доступ к боту определенному пользователю по ID (напишите боту @userinfobot для получения своего ID)",
		Example:     "adduser 5293210034",
		Call:        bot.AddUser,
	})

	bot.NewCommand(Command{
		Name:        "rmuser",
		Description: "Убрать доступ к боту определенному пользователю по ID",
		Example:     "rmuser 5293210034",
		Call:        bot.RemoveUser,
	})

	bot.NewCommand(Command{
		Name:        "setmaxcontent",
		Description: "Установить новый лимит символов, извлекаемых из текста статьи",
		Example:     "setmaxcontent 340",
		Call:        bot.ChangeMaxContentSize,
	})

	bot.NewCommand(Command{
		Name:        "conf",
		Description: "Написать текущую конфигурацию",
		Call:        bot.PrintConfig,
	})

	bot.NewCommand(Command{
		Name:        "setsheetname",
		Description: "Изменить наименование листа таблицы",
		Example:     "setsheetname Sheet 2",
		Call:        bot.ChangeSheetName,
	})

	bot.NewCommand(Command{
		Name:        "setsheetid",
		Description: "Изменить идентификатор таблицы",
		Example:     "setsheetid s0m3_1d_l1k3_k4DGHJd1",
		Call:        bot.ChangeSpreadhseetID,
	})

	bot.NewCommand(Command{
		Name:        "setquerytimeout",
		Description: "Изменить допустимое время запросов к LLM в секундах. Если запрос будет обрабатываться дольше допустимого, - запрос окончится досрочно.",
		Example:     "setquerytimeout 120",
		Call:        bot.ChangeQueryTimeout,
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

		// Проверка на возможность дальнейшего общения с данным пользователем
		if !bot.conf.Public {
			var allowed bool = false
			for _, allowedID := range bot.conf.AllowedUserIDs {
				if update.Message.From.ID == allowedID {
					allowed = true
					break
				}
			}

			if !allowed {
				// Не пропускаем дальше
				msg := tgbotapi.NewMessage(
					update.Message.Chat.ID,
					"Вам не разрешено пользоваться этим ботом!",
				)
				bot.api.Send(msg)

				if bot.conf.Debug {
					log.Printf("Не допустили к общению пользователя %v", update.Message.From.ID)
				}

				continue
			}
		}

		// Обработать команды
		update.Message.Text = strings.TrimSpace(update.Message.Text)
		for _, command := range bot.commands {
			if strings.HasPrefix(update.Message.Text, command.Name) {
				command.Call(update.Message)
				break // Дальше не продолжаем
			}
		}

		// Проверим, URL ли это
		if strings.HasPrefix(update.Message.Text, "http") {
			// Отправляем команде do
			do := bot.CommandByName("do")
			if do != nil {
				update.Message.Text = "do " + update.Message.Text
				do.Call(update.Message)
			}
		}
	}

	return nil
}
