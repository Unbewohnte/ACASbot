package bot

import (
	"Unbewohnte/ACASbot/spreadsheet"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Command struct {
	Name        string
	Description string
	Example     string
	Call        func(*tgbotapi.Message)
}

func (bot *Bot) NewCommand(cmd Command) {
	bot.commands = append(bot.commands, cmd)
}

func (bot *Bot) CommandByName(name string) *Command {
	for _, command := range bot.commands {
		if command.Name == name {
			return &command
		}
	}

	return nil
}

func (bot *Bot) Help(message *tgbotapi.Message) {
	var helpMessage string

	for _, command := range bot.commands {
		helpMessage += fmt.Sprintf("\n*Команда:* \"%s\"\n*Описание:* %s\n", command.Name, command.Description)
		if command.Example != "" {
			helpMessage += fmt.Sprintf("*Пример:* %s\n", command.Example)
		}
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		helpMessage,
	)
	msg.ParseMode = "Markdown"
	bot.api.Send(msg)
}

func (bot *Bot) ChangeOrg(message *tgbotapi.Message) {
	parts := strings.Split(strings.TrimSpace(message.Text), " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Имя организации не указано",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	bot.conf.OrganizationName = strings.Join(parts[1:], " ")
	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		fmt.Sprintf("Организация сменена на *\"%s\"*", bot.conf.OrganizationName),
	)

	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) formatAnalysisResult(result *ArticleAnalysis) string {
	var response strings.Builder

	// Добавляем заголовок
	if result.Content.Success && result.Content.Title != "" {
		response.WriteString(fmt.Sprintf("*Заголовок:* %s\n\n", result.Content.Title))
	} else if result.TitleFromModel != "" {
		response.WriteString(fmt.Sprintf("*Заголовок:* %s\n\n", result.TitleFromModel))
	}

	// Дата публикации
	if result.Content.PubDate != nil {
		response.WriteString(fmt.Sprintf("*Дата публикации:* %s\n\n", result.Content.PubDate))
	}

	// Добавляем тему (если есть)
	if bot.conf.FullAnalysis && result.Theme != "" {
		response.WriteString(fmt.Sprintf("*Тема:* %s\n\n", result.Theme))
	}

	// Добавляем отношение
	if result.Sentiment != "" {
		response.WriteString(fmt.Sprintf("*Отношение:* %s\n", result.Sentiment))
		if result.Justification != "" {
			response.WriteString(fmt.Sprintf("*Обоснование:* %s\n", result.Justification))
		}
	}

	// Добавляем ошибки (если есть)
	if len(result.Errors) > 0 {
		response.WriteString("\n⚠️ *Ошибки при анализе:*\n")
		for _, err := range result.Errors {
			response.WriteString(fmt.Sprintf("- %s\n", err.Error()))
		}
	}

	return response.String()
}

func (bot *Bot) Do(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Вы не указали URL")
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	url := parts[1]
	if !strings.HasPrefix(url, "http") {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, отправьте действительный URL, начинающийся с http/https")
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	// Анализируем статью
	result, err := bot.analyzeArticle(url)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка обработки страницы: "+err.Error())
		errorMsg.ReplyToMessageID = message.MessageID
		bot.api.Send(errorMsg)
		return
	}

	// Форматируем ответ
	responseText := bot.formatAnalysisResult(result)
	msg := tgbotapi.NewMessage(message.Chat.ID, "📋 *Результаты анализа*\n\n"+responseText)
	msg.ParseMode = "Markdown"
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Добавляем в Google Sheets
	if bot.conf.PushToGoogleSheet {
		if result.Content.PubDate == nil {
			now := time.Now()
			result.Content.PubDate = &now
		}

		summary := result.Content.Title
		if summary == "" {
			summary = result.TitleFromModel
		}

		entry := &spreadsheet.SheetEntry{
			PublicationDate: *result.Content.PubDate,
			Source:          extractDomain(url),
			Summary:         summary,
			URL:             url,
			SentimentType:   result.Sentiment,
		}

		if err := bot.sheet.AddAnalysisResultWithRetry(entry, 3); err != nil {
			log.Printf("Ошибка добавления в Google Sheet: %v", err)
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				"Ошибка внесения изменений в таблицу: "+err.Error(),
			)
			bot.api.Send(msg)
		} else {
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				"Запись успешно добавлена в таблицу!",
			)
			bot.api.Send(msg)
		}
	}
}

func extractDomain(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	return u.Host
}

func (bot *Bot) ToggleAnalysis(message *tgbotapi.Message) {
	if bot.conf.FullAnalysis {
		bot.conf.FullAnalysis = false
		bot.api.Send(tgbotapi.NewMessage(message.Chat.ID, "Полный анализ выключен"))
	} else {
		bot.conf.FullAnalysis = true
		bot.api.Send(tgbotapi.NewMessage(message.Chat.ID, "Полный анализ включен"))
	}

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) About(message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		`ACAS bot (Article Context And Sentiment bot).

Бот для анализа статей на отношение к определенной организации/личности, а также получения некоторых метаданных: заголовка и текста.
Результаты анализа могут автоматически добавляться в Google таблицу при настройке.
`,
	)

	bot.api.Send(msg)
}

func (bot *Bot) AddUser(message *tgbotapi.Message) {
	parts := strings.Split(strings.TrimSpace(message.Text), " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"ID пользователя не указан",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Неверный ID пользователя",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	for _, allowedID := range bot.conf.AllowedUserIDs {
		if id == allowedID {
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				"Этот пользователь уже есть в списке разрешенных.",
			)
			msg.ReplyToMessageID = message.MessageID
			bot.api.Send(msg)
			return
		}
	}

	bot.conf.AllowedUserIDs = append(bot.conf.AllowedUserIDs, id)

	// Сохраним в файл
	bot.conf.Update()

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Пользователь успешно добавлен!",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
}

func (bot *Bot) TogglePublicity(message *tgbotapi.Message) {
	if bot.conf.Public {
		bot.conf.Public = false
		bot.api.Send(
			tgbotapi.NewMessage(message.Chat.ID, "Доступ к боту теперь только у избранных."),
		)
	} else {
		bot.conf.Public = true
		bot.api.Send(
			tgbotapi.NewMessage(message.Chat.ID, "Доступ к боту теперь у всех."),
		)
	}

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) RemoveUser(message *tgbotapi.Message) {
	parts := strings.Split(strings.TrimSpace(message.Text), " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"ID пользователя не указан",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Неверный ID пользователя",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	tmp := bot.conf.AllowedUserIDs
	bot.conf.AllowedUserIDs = []int64{}
	for _, allowedID := range tmp {
		if allowedID == id {
			continue
		}

		bot.conf.AllowedUserIDs = append(bot.conf.AllowedUserIDs, allowedID)
	}

	// Сохраним в файл
	bot.conf.Update()

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Пользователь успешно удален!",
	)
	msg.ReplyToMessageID = message.MessageID
	_, err = bot.api.Send(msg)
}

func (bot *Bot) ChangeMaxContentSize(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указано новое значение.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	newMaxContentSize, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	if newMaxContentSize <= 0 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение. Необходимо указать значение > 0",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	bot.conf.MaxContentSize = uint(newMaxContentSize)

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Значение успешно изменено.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) PrintConfig(message *tgbotapi.Message) {
	var response string = ""

	response += "*Нынешняя конфигурация*: \n"
	response += fmt.Sprintf("*Наименование организации*: %v\n", bot.conf.OrganizationName)
	response += fmt.Sprintf("*Полный анализ?*: %v\n", bot.conf.FullAnalysis)
	response += fmt.Sprintf("*Лимит символов для анализа*: %v\n", bot.conf.MaxContentSize)
	response += fmt.Sprintf("*LLM*: %v\n", bot.conf.OllamaModel)
	response += fmt.Sprintf("*Отправлять в Google таблицу?*: %v\n", bot.conf.PushToGoogleSheet)
	response += fmt.Sprintf("*Общедоступный?*: %v\n", bot.conf.Public)
	response += fmt.Sprintf("*Разрешенные пользователи*: %+v\n", bot.conf.AllowedUserIDs)

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		response,
	)
	msg.ParseMode = "Markdown"
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
}
