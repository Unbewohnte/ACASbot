package bot

import (
	"Unbewohnte/ACASbot/internal/inference"
	"Unbewohnte/ACASbot/internal/spreadsheet"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	conf     *Config
	model    *inference.Client
	commands []Command
	sheet    *spreadsheet.GoogleSheetsClient
	server   *WebServer
}

func NewBot(config *Config) (*Bot, error) {
	model, err := inference.NewClient(
		config.Ollama.GeneralModel,
		config.Ollama.EmbeddingModel,
		config.Ollama.QueryTimeoutSeconds,
	)
	if err != nil {
		return nil, err
	}

	api, err := tgbotapi.NewBotAPI(config.Telegram.ApiToken)
	if err != nil {
		log.Printf("ВНИМАНИЕ: Не удалось подключиться к Telegram API (%v). Telegram-модуль будет отключен.", err)
	}

	bot := &Bot{
		api:   api,
		conf:  config,
		model: model,
	}

	bot.server = NewWebServer(bot)

	return bot, nil
}

func (bot *Bot) StartAutoSave(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				if _, err := bot.SaveLocalSpreadsheet(""); err != nil {
					log.Printf("Ошибка автосохранения: %v", err)
				} else {
					log.Printf("Автосохранение выполнено успешно")
				}
			}
		}
	}()
}

func (bot *Bot) init() {
	_, err := bot.conf.OpenDB()
	if err != nil {
		log.Panic(err)
	}

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
		Name:        "do",
		Description: "Анализировать статью",
		Example:     "do https://example.com/article2",
		Group:       "Анализ",
		Call:        bot.Do,
	})

	bot.NewCommand(Command{
		Name:        "toggleSaveSimilar",
		Description: "Не сохранять|Сохранять похожие статьи",
		Group:       "Анализ",
		Call:        bot.ToggleSaveSimilar,
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
		Call:        bot.ChangeSpreadsheetID,
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
		Call:        bot.SetTitlePrompt,
	})

	bot.NewCommand(Command{
		Name:        "setpromptsent",
		Description: "Изменить промпт выявления отношения к объекту",
		Example:     "setpromptses Определи отношение к {{OBJECT}} в следующем тексте. Ответь одним предложением. Текст: {{TEXT}}",
		Group:       "LLM",
		Call:        bot.SetSentimentPrompt,
	})

	bot.NewCommand(Command{
		Name:        "xlsx",
		Description: "Сгенерировать файл XLSX таблицы с результатами анализов",
		Group:       "Таблицы",
		Call:        bot.GenerateSpreadsheet,
	})

	bot.NewCommand(Command{
		Name:        "findsimilar",
		Description: "Определить уникальность статьи без проведения полного анализа. Если есть похожие - сообщить.",
		Group:       "Анализ",
		Example:     "findsimilar https://example.com/article",
		Call:        bot.FindSimilar,
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

	bot.NewCommand(Command{
		Name:        "loadxlsx",
		Description: "Загрузить статьи из XLSX файла (без анализа)",
		Example:     "loadxlsx [прикрепите файл]",
		Group:       "База данных",
		Call:        bot.LoadXLSX,
	})

	bot.NewCommand(Command{
		Name:        "getlogs",
		Description: "Отправить файл логов",
		Group:       "Общее",
		Call:        bot.SendLogs,
	})

	bot.NewCommand(Command{
		Name:        "setxlsxcolumns",
		Description: "Установить конфигурацию колонок для XLSX-файла",
		Example:     "setxlsxcolumns [{\"name\": \"Дата\", \"field\": \"published_at\"}, {\"name\": \"Заголовок\", \"llm_query\": \"Извлеки заголовок из текста: {{.Content}}\"}]",
		Group:       "Таблицы",
		Call:        bot.SetXLSXColumns,
	})

	bot.NewCommand(Command{
		Name:        "showxlsxcolumns",
		Description: "Показать текущую конфигурацию колонок для XLSX-файла",
		Example:     "showxlsxcolumns",
		Group:       "Таблицы",
		Call:        bot.ShowXLSXColumns,
	})

	bot.NewCommand(Command{
		Name:        "togglepushtogoogle",
		Description: "Не отправлять|Отправлять результаты анализа в гугл таблицу",
		Group:       "Таблицы",
		Call:        bot.TogglePushToGoogleSheets,
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

	// Автоматически сохранять таблицу
	bot.StartAutoSave(time.Hour * 1)

	// Запустить веб-сервер
	if bot.conf.Web.Enabled {
		bot.server.Start()
	}
}

func (bot *Bot) Start() error {
	bot.init()

	// Если API Telegram не был инициализирован, переходим в локальный режим
	if bot.api == nil {
		log.Println("Работа без Telegram")
		if !bot.conf.Web.Enabled {
			log.Println("ВНИМАНИЕ: Веб-сервер выключен в конфигурации. Бот работает вхолостую.")
		} else {
			log.Println("Используйте веб-интерфейс для взаимодействия с ботом.")
		}

		// Блокируем горутину, чтобы приложение не завершилось (веб-сервер работает в фоне)
		select {}
		return nil
	}

	log.Printf("Бот авторизован как %s", bot.api.Self.UserName)

	retryDelay := 5 * time.Second
	for {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := bot.api.GetUpdatesChan(u)

		for update := range updates {
			if update.Message == nil {
				continue
			}

			go func(message *tgbotapi.Message) {
				log.Printf("[%s] %s (cap: %s)", message.From.UserName, message.Text, message.Caption)

				// Проверка на возможность дальнейшего общения с данным пользователем
				if !bot.conf.Telegram.Public {
					var allowed bool = false
					for _, allowedID := range bot.conf.Telegram.AllowedUserIDs {
						if message.From.ID == allowedID {
							allowed = true
							break
						}
					}

					if !allowed {
						// Не пропускаем дальше
						msg := tgbotapi.NewMessage(
							message.Chat.ID,
							"Вам не разрешено пользоваться этим ботом!",
						)
						bot.api.Send(msg)

						if bot.conf.Debug {
							log.Printf("Не допустили к общению пользователя %v", message.From.ID)
						}

						return
					}
				}

				// Обработать команды
				message.Text = strings.TrimSpace(message.Text)
				if message.Text == "" {
					message.Text = message.Caption
				}

				for index, command := range bot.commands {
					if strings.HasPrefix(strings.ToLower(message.Text), command.Name) {
						bot.handleTelegramCommand(&bot.commands[index], message)
						return // Дальше не продолжаем
					}
				}

				// Проверим, URL ли это
				if strings.HasPrefix(message.Text, "http") {
					// Отправляем команде do
					do := bot.CommandByName("do")
					if do != nil {
						message.Text = "do " + message.Text
						bot.handleTelegramCommand(do, message)
					}
				} else {
					// Неверно введенная команда
					bot.sendCommandSuggestions(
						message.Chat.ID,
						strings.ToLower(message.Text),
					)
				}
			}(update.Message)
		}

		log.Println("Соединение с Telegram потеряно. Переподключение...")
		time.Sleep(retryDelay)
		if retryDelay < 300*time.Second {
			retryDelay *= 2
		}
	}
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

func (bot *Bot) handleTelegramCommand(command *Command, msg *tgbotapi.Message) {
	var args string

	switch command.Name {
	case "loadxlsx":
		// Для команды loadxlsx обрабатываем прикрепленный файл
		if msg.Document == nil {
			bot.sendError(msg.Chat.ID, "Пожалуйста, прикрепите XLSX файл к команде", msg.MessageID)
			return
		}

		// Проверяем расширение файла
		if !strings.HasSuffix(msg.Document.FileName, ".xlsx") {
			bot.sendError(msg.Chat.ID, "Формат файла должен быть .xlsx", msg.MessageID)
			return
		}

		// Скачиваем файл
		fileURL, err := bot.api.GetFileDirectURL(msg.Document.FileID)
		if err != nil {
			bot.sendError(msg.Chat.ID, "Ошибка получения файла", msg.MessageID)
			return
		}

		// Индикатор загрузки
		processingMsg := tgbotapi.NewMessage(msg.Chat.ID, "📥 Загружаю файл...")
		sentMsg, _ := bot.api.Send(processingMsg)
		defer func() {
			deleteMsg := tgbotapi.NewDeleteMessage(msg.Chat.ID, sentMsg.MessageID)
			bot.api.Send(deleteMsg)
		}()

		// Создаем временный файл
		tmpFile, err := os.CreateTemp("", "acasbot-*.xlsx")
		if err != nil {
			bot.sendError(msg.Chat.ID, "Ошибка создания временного файла", msg.MessageID)
			return
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		// Скачиваем содержимое
		resp, err := http.Get(fileURL)
		if err != nil {
			bot.sendError(msg.Chat.ID, "Ошибка скачивания файла", msg.MessageID)
			return
		}
		defer resp.Body.Close()

		if _, err := io.Copy(tmpFile, resp.Body); err != nil {
			bot.sendError(msg.Chat.ID, "Ошибка сохранения файла", msg.MessageID)
			return
		}

		// Устанавливаем args как путь к временному файлу
		args = tmpFile.Name()
	case "xlsx":
		fileName := "ACASbot_Results.xlsx"
		if _, err := os.Stat(fileName); err == nil {
			// Файл существует, отправляем его
			fileBytes, err := os.ReadFile(fileName)
			if err != nil {
				bot.sendError(msg.Chat.ID, "Ошибка чтения файла: "+err.Error(), msg.MessageID)
				return
			}

			file := tgbotapi.FileBytes{
				Name:  "ACASbot_Results.xlsx",
				Bytes: fileBytes,
			}
			docMsg := tgbotapi.NewDocument(msg.Chat.ID, file)
			docMsg.Caption = "📊 Сгенерированная таблица на основе базы данных"
			docMsg.ReplyToMessageID = msg.MessageID
			_, err = bot.api.Send(docMsg)
			if err != nil {
				bot.sendError(msg.Chat.ID, "Ошибка отправки файла: "+err.Error(), msg.MessageID)
				return
			}

			// Отправляем подтверждающее сообщение
			bot.sendMessage(msg.Chat.ID, "Таблица успешно сгенерирована и отправлена", msg.MessageID)
			return
		}
	case "getlogs":
		// Проверяем, существует ли файл логов
		if _, err := os.Stat(bot.conf.LogsFile); os.IsNotExist(err) {
			bot.sendError(msg.Chat.ID, "Файл логов не найден", msg.MessageID)
			return
		}

		// Проверяем размер файла
		fileInfo, err := os.Stat(bot.conf.LogsFile)
		if err != nil {
			bot.sendError(msg.Chat.ID, "Ошибка проверки размера файла: "+err.Error(), msg.MessageID)
			return
		}

		// Telegram имеет ограничение на размер файла - 50MB
		if fileInfo.Size() > 50*1024*1024 { // 50MB
			bot.sendError(msg.Chat.ID, "Файл логов слишком большой (максимум 50MB)", msg.MessageID)
			return
		}

		// Отправляем файл логов
		file := tgbotapi.NewDocument(msg.Chat.ID, tgbotapi.FilePath(bot.conf.LogsFile))
		file.Caption = "📄 Файл логов ACASbot"
		file.ReplyToMessageID = msg.MessageID

		_, err = bot.api.Send(file)
		if err != nil {
			bot.sendError(msg.Chat.ID, "Ошибка отправки файла логов: "+err.Error(), msg.MessageID)
			return
		}
	default:
		// Убрать имя команды
		parts := strings.Split(strings.TrimSpace(msg.Text), " ")
		if len(parts) < 2 {
			// Это команда без аргументов
			args = ""
		} else {
			args = strings.Join(parts[1:], " ")
		}
	}

	result, err := command.Call(args)
	if err != nil {
		bot.sendError(msg.Chat.ID, "Ошибка: "+err.Error(), msg.MessageID)
		return
	}

	bot.sendMessage(msg.Chat.ID, result, msg.MessageID)
}
