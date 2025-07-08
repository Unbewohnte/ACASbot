package bot

import (
	"Unbewohnte/ACASbot/conf"
	"Unbewohnte/ACASbot/inference"
	"Unbewohnte/ACASbot/spreadsheet"
	"context"
	"fmt"
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
		config.Ollama.Model,
		config.Ollama.QueryTimeoutSeconds,
	)
	if err != nil {
		return nil, err
	}

	api, err := tgbotapi.NewBotAPI(config.Telegram.ApiToken)
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
		Group:       "Общее",
		Call:        bot.Help,
	})

	bot.NewCommand(Command{
		Name:        "changeobj",
		Description: "Изменить имя объекта, отношение к которому будет анализировано.",
		Example:     "changeobj Человечество",
		Group:       "Анализ",
		Call:        bot.ChangeObj,
	})

	bot.NewCommand(Command{
		Name:        "toggleanalysis",
		Description: "Включить или выключить полный анализ статей",
		Group:       "Анализ",
		Call:        bot.ToggleAnalysis,
	})

	bot.NewCommand(Command{
		Name:        "do",
		Description: "Анализировать статью",
		Example:     "do https://example.com/article2",
		Group:       "Анализ",
		Call:        bot.Do,
	})

	bot.NewCommand(Command{
		Name:        "about",
		Description: "Напечатать информацию о боте",
		Group:       "Общее",
		Call:        bot.About,
	})

	bot.NewCommand(Command{
		Name:        "togglepublic",
		Description: "Включить или выключить публичный/приватный доступ к боту",
		Group:       "Телеграм",
		Call:        bot.TogglePublicity,
	})

	bot.NewCommand(Command{
		Name:        "adduser",
		Description: "Добавить доступ к боту определенному пользователю по ID (напишите боту @userinfobot для получения своего ID)",
		Example:     "adduser 5293210034",
		Group:       "Телеграм",
		Call:        bot.AddUser,
	})

	bot.NewCommand(Command{
		Name:        "rmuser",
		Description: "Убрать доступ к боту определенному пользователю по ID",
		Example:     "rmuser 5293210034",
		Group:       "Телеграм",
		Call:        bot.RemoveUser,
	})

	bot.NewCommand(Command{
		Name:        "setmaxcontent",
		Description: "Установить новый лимит символов, извлекаемых из текста статьи",
		Example:     "setmaxcontent 340",
		Group:       "Анализ",
		Call:        bot.ChangeMaxContentSize,
	})

	bot.NewCommand(Command{
		Name:        "conf",
		Description: "Написать текущую конфигурацию",
		Group:       "Общее",
		Call:        bot.PrintConfig,
	})

	bot.NewCommand(Command{
		Name:        "setsheetname",
		Description: "Изменить наименование листа таблицы",
		Example:     "setsheetname Sheet 2",
		Group:       "Таблицы",
		Call:        bot.ChangeSheetName,
	})

	bot.NewCommand(Command{
		Name:        "setsheetid",
		Description: "Изменить идентификатор таблицы",
		Example:     "setsheetid s0m3_1d_l1k3_k4DGHJd1",
		Group:       "Таблицы",
		Call:        bot.ChangeSpreadhseetID,
	})

	bot.NewCommand(Command{
		Name:        "setquerytimeout",
		Description: "Изменить допустимое время запросов к LLM в секундах. Если запрос будет обрабатываться дольше допустимого, - запрос окончится досрочно.",
		Example:     "setquerytimeout 120",
		Group:       "LLM",
		Call:        bot.ChangeQueryTimeout,
	})

	bot.NewCommand(Command{
		Name:        "ask",
		Description: "Задать общий запрос модели",
		Example:     "ask Как получить API token телеграм?",
		Group:       "LLM",
		Call:        bot.GeneralQuery,
	})

	bot.NewCommand(Command{
		Name:        "setobjectdata",
		Description: "Указать метаданные об объекте",
		Example:     "setobjectdata Ростов-на-Дону - город на юге России, включает в себя ...",
		Group:       "Общее",
		Call:        bot.SetObjectData,
	})

	bot.NewCommand(Command{
		Name:        "setpromptaf",
		Description: "Изменить промпт связи",
		Example:     "setpromptaf При чем здесь {{OBJECT}}? Текст: {{TEXT}}",
		Group:       "LLM",
		Call:        bot.SetAffiliationPrompt,
	})

	bot.NewCommand(Command{
		Name:        "setpromptti",
		Description: "Изменить промпт нахождения заголовка",
		Example:     "setpromptti Найди заголовок текста. Текст: {{TEXT}}",
		Group:       "LLM",
		Call:        bot.SettTitlePrompt,
	})

	bot.NewCommand(Command{
		Name:        "setpromptse",
		Description: "Изменить промпт выявления отношения к объекту",
		Example:     "setpromptse Определи отношение к {{OBJECT}} в следующем тексте. Текст: {{TEXT}}",
		Group:       "LLM",
		Call:        bot.SetSentimentPrompt,
	})

	bot.NewCommand(Command{
		Name:        "setpromptses",
		Description: "Изменить короткий промпт выявления отношения к объекту",
		Example:     "setpromptses Определи отношение к {{OBJECT}} в следующем тексте. Ответь одним предложением. Текст: {{TEXT}}",
		Group:       "LLM",
		Call:        bot.SetSentimentShortPrompt,
	})

	bot.NewCommand(Command{
		Name:        "getlocalsheet",
		Description: "Запросить файл локальной таблицы с результатами анализов",
		Group:       "Таблицы",
		Call:        bot.GetLocalSpreadsheet,
	})

	bot.NewCommand(Command{
		Name:        "clearlocalsheet",
		Description: "Удалить файл локальной таблицы с результатами анализов",
		Group:       "Таблицы",
		Call:        bot.ClearLocalSpreadsheet,
	})

	bot.NewCommand(Command{
		Name:        "models",
		Description: "Напечатать доступные боту локальные LLM",
		Group:       "LLM",
		Call:        bot.ListModels,
	})

	bot.NewCommand(Command{
		Name:        "setmodel",
		Description: "Указать имя новой локальной LLM, которая будет использоваться",
		Example:     "setmodel gemma3:12b",
		Group:       "LLM",
		Call:        bot.SetModel,
	})

	if bot.conf.Sheets.PushToGoogleSheet {
		sheetsClient, err := spreadsheet.NewGoogleSheetsClient(
			context.Background(),
			bot.conf.Sheets.Google.Config,
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

loop:
	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		// Проверка на возможность дальнейшего общения с данным пользователем
		if !bot.conf.Telegram.Public {
			var allowed bool = false
			for _, allowedID := range bot.conf.Telegram.AllowedUserIDs {
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
			if strings.HasPrefix(strings.ToLower(update.Message.Text), command.Name) {
				go command.Call(update.Message)
				goto loop // Дальше не продолжаем
			}
		}

		// Проверим, URL ли это
		if strings.HasPrefix(update.Message.Text, "http") {
			// Отправляем команде do
			do := bot.CommandByName("do")
			if do != nil {
				update.Message.Text = "do " + update.Message.Text
				go do.Call(update.Message)
			}
		} else {
			// Неверно введенная команда
			bot.sendCommandSuggestions(
				update.Message.Chat.ID,
				strings.ToLower(update.Message.Text),
			)
		}
	}

	return nil
}

func (bot *Bot) sendCommandSuggestions(chatID int64, input string) {
	suggestions := bot.findSimilarCommands(input)
	if len(suggestions) == 0 {
		return
	}

	message := "Неизвестная команда. Возможно, имеется в виду одна из этих команд:\n"
	for _, cmd := range suggestions {
		command := bot.CommandByName(cmd)
		if command != nil {
			message += fmt.Sprintf("`%s` - %s\n", command.Name, command.Description)
		}
	}
	message += "\nДля справки используйте `help [команда](опционально)`"

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "Markdown"
	bot.api.Send(msg)
}
