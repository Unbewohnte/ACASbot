package bot

import (
	"Unbewohnte/ACASbot/spreadsheet"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Command struct {
	Name        string
	Description string
	Example     string
	Group       string
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

	groups := make(map[string][]Command)
	sort.Strings(groups)

	for _, command := range bot.commands {
		groups[command.Group] = append(groups[command.Group], command)
	}

	for group, commands := range groups {
		helpMessage += fmt.Sprintf("\n\n*[%s]*\n", group)
		for _, command := range commands {
			helpMessage += fmt.Sprintf("\n*Команда:* \"%s\"\n*Описание:* %s\n", command.Name, command.Description)
			if command.Example != "" {
				helpMessage += fmt.Sprintf("*Пример:* `%s`\n", command.Example)
			}
		}
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		helpMessage,
	)
	msg.ParseMode = "Markdown"
	bot.api.Send(msg)
}

func (bot *Bot) ChangeObj(message *tgbotapi.Message) {
	parts := strings.Split(strings.TrimSpace(message.Text), " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Имя объекта не указано",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	bot.conf.Object = strings.Join(parts[1:], " ")
	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		fmt.Sprintf("Объект сменен на \"%s\"", bot.conf.Object),
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

	// Добавляем связь (если есть)
	if bot.conf.FullAnalysis && result.Affiliation != "" {
		response.WriteString(fmt.Sprintf("*Связь с \"%s\":* %s\n\n", bot.conf.Object, result.Affiliation))
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

	if bot.conf.Debug {
		log.Println(msg.Text)
	}

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
		Note:            result.Affiliation,
		SentimentType:   result.Sentiment,
	}

	// Добавляем в Google Sheets
	if bot.conf.Sheets.PushToGoogleSheet {
		if err := bot.sheet.AddAnalysisResultWithRetry(entry, 3); err != nil {
			log.Printf("Ошибка добавления в Google Sheet: %v", err)
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				"❌ Ошибка внесения изменений в онлайн таблицу: "+err.Error(),
			)
			bot.api.Send(msg)
		} else {
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				"💾 Запись успешно добавлена в онлайн таблицу!",
			)
			bot.api.Send(msg)
		}
	}

	// Добавляем в локальный файл
	if bot.conf.Sheets.SaveSheetLocally {
		if err := spreadsheet.AppendToLocalSheet(bot.conf.Sheets.Local.Filename, entry); err != nil {
			log.Printf("Ошибка сохранения в локальный файл: %v", err)
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				"❌ Ошибка сохранения в локальный файл: "+err.Error(),
			)
			bot.api.Send(msg)
		} else {
			log.Println("Успешно сохранено в локальный файл")
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				"💾 Результат сохранен в локальный файл!",
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

Бот для анализа статей на отношение к определенной объекта/личности, а также получения некоторых метаданных: заголовка и краткого описания.
Результаты анализа могут автоматически добавляться в Google таблицу при настройке.

Source: https://github.com/Unbewohnte/ACASbot
Лицензия: GPLv3
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

	for _, allowedID := range bot.conf.Telegram.AllowedUserIDs {
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

	bot.conf.Telegram.AllowedUserIDs = append(bot.conf.Telegram.AllowedUserIDs, id)

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
	if bot.conf.Telegram.Public {
		bot.conf.Telegram.Public = false
		bot.api.Send(
			tgbotapi.NewMessage(message.Chat.ID, "Доступ к боту теперь только у избранных."),
		)
	} else {
		bot.conf.Telegram.Public = true
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

	tmp := bot.conf.Telegram.AllowedUserIDs
	bot.conf.Telegram.AllowedUserIDs = []int64{}
	for _, allowedID := range tmp {
		if allowedID == id {
			continue
		}

		bot.conf.Telegram.AllowedUserIDs = append(bot.conf.Telegram.AllowedUserIDs, allowedID)
	}

	// Сохраним в файл
	bot.conf.Update()

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Пользователь успешно удален!",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
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
	response += "\n*[ОБЩЕЕ]*:\n"
	response += fmt.Sprintf("*Объект*: `%v`\n", bot.conf.Object)
	response += fmt.Sprintf("*Метаданные объекта*: `%v`\n", bot.conf.ObjectMetadata)
	response += fmt.Sprintf("*Общедоступный?*: `%v`\n", bot.conf.Telegram.Public)
	response += fmt.Sprintf("*Полный анализ?*: `%v`\n", bot.conf.FullAnalysis)
	response += fmt.Sprintf("*Лимит для анализа*: `%v`\n", bot.conf.MaxContentSize)
	response += fmt.Sprintf("*Разрешенные пользователи*: `%+v`\n", bot.conf.Telegram.AllowedUserIDs)
	response += "\n*[LLM]*:\n"
	response += fmt.Sprintf("*LLM*: `%v`\n", bot.conf.Ollama.Model)
	response += fmt.Sprintf("*Временной лимит на ответ LLM в секундах*: `%v`\n", bot.conf.Ollama.QueryTimeoutSeconds)
	response += fmt.Sprintf("*Промпт заголовка*: `%v`\n", bot.conf.Ollama.Prompts.Title)
	response += fmt.Sprintf("*Промпт связи с объектом*: `%v`\n", bot.conf.Ollama.Prompts.Affiliation)
	response += fmt.Sprintf("*Короткий промпт отношения к объекту*: `%v`\n", bot.conf.Ollama.Prompts.SentimentShort)
	response += fmt.Sprintf("*Полный промпт отношения к объекту*: `%v`\n", bot.conf.Ollama.Prompts.SentimentLong)
	response += "\n*[ТАБЛИЦЫ]*:\n"
	response += fmt.Sprintf("*Сохранять в локальную таблицу?*: `%v`\n", bot.conf.Sheets.SaveSheetLocally)
	response += fmt.Sprintf("*Отправлять в Google таблицу?*: `%v`\n", bot.conf.Sheets.PushToGoogleSheet)
	response += fmt.Sprintf("*Наименование листа таблицы*: `%v`\n", bot.conf.Sheets.Google.Config.SheetName)
	response += fmt.Sprintf("*ID Google таблицы*: `%v`\n", bot.conf.Sheets.Google.Config.SpreadsheetID)

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		response,
	)
	msg.ParseMode = "Markdown"
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
}

func (bot *Bot) ChangeSpreadhseetID(message *tgbotapi.Message) {
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

	bot.conf.Sheets.Google.Config.SpreadsheetID = parts[1]
	if bot.sheet != nil {
		bot.sheet.SpreadsheetID = bot.conf.Sheets.Google.Config.SpreadsheetID
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Значение успешно изменено.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) ChangeSheetName(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указано новое имя.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	newName, _ := strings.CutPrefix(message.Text, parts[0])
	bot.conf.Sheets.Google.Config.SheetName = strings.TrimSpace(newName)
	if bot.sheet != nil {
		bot.sheet.SheetName = bot.conf.Sheets.Google.Config.SheetName
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Имя успешно изменено.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) ChangeQueryTimeout(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указано количество секунд.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	timeoutSeconds, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Неверное значение количества секунд.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	bot.conf.Ollama.QueryTimeoutSeconds = uint(timeoutSeconds)
	bot.model.TimeoutSeconds = bot.conf.Ollama.QueryTimeoutSeconds

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Время таймаута запросов к LLM успешно изменено.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) GeneralQuery(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указан запрос.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	query := strings.Join(parts[1:], " ")
	answer, err := bot.model.Query(query)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не удалось ответить на запрос.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		answer,
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	log.Printf("Ответ: %s", msg.Text)
}

func (bot *Bot) SetObjectData(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указана дополнительная информация об объекте.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	objData, _ := strings.CutPrefix(message.Text, parts[0])

	bot.conf.ObjectMetadata = strings.TrimSpace(objData)

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Информация успешно добавлена.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

type promptType string

const (
	PROMPT_AFFILIATION     promptType = "affiliation"
	PROMPT_TITLE           promptType = "title"
	PROMPT_SENTIMENT_LONG  promptType = "sentiment_long"
	PROMPT_SENTIMENT_SHORT promptType = "sentiment_short"
)

func (bot *Bot) setPrompt(message *tgbotapi.Message, promptType promptType) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указан новый промпт.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	prompt, _ := strings.CutPrefix(message.Text, parts[0])

	switch promptType {
	case PROMPT_TITLE:
		bot.conf.Ollama.Prompts.Title = prompt
	case PROMPT_AFFILIATION:
		bot.conf.Ollama.Prompts.Affiliation = prompt
	case PROMPT_SENTIMENT_LONG:
		bot.conf.Ollama.Prompts.SentimentLong = prompt
	case PROMPT_SENTIMENT_SHORT:
		bot.conf.Ollama.Prompts.SentimentShort = prompt
	default:
		return
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Новый промпт успешно применен.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) SetAffiliationPrompt(message *tgbotapi.Message) {
	bot.setPrompt(message, PROMPT_AFFILIATION)
}

func (bot *Bot) SettTitlePrompt(message *tgbotapi.Message) {
	bot.setPrompt(message, PROMPT_TITLE)
}

func (bot *Bot) SetSentimentPrompt(message *tgbotapi.Message) {
	bot.setPrompt(message, PROMPT_SENTIMENT_LONG)
}

func (bot *Bot) SetSentimentShortPrompt(message *tgbotapi.Message) {
	bot.setPrompt(message, PROMPT_SENTIMENT_SHORT)
}

func (bot *Bot) GetLocalSpreadsheet(message *tgbotapi.Message) {
	if !bot.conf.Sheets.SaveSheetLocally {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Локальное сохранение результатов отключено!",
		)
		bot.api.Send(msg)
		return
	}

	if _, err := os.Stat(bot.conf.Sheets.Local.Filename); os.IsNotExist(err) {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Локальный файл с результатами не найден",
		)
		bot.api.Send(msg)
		return
	}

	fileBytes, err := os.ReadFile(bot.conf.Sheets.Local.Filename)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Ошибка чтения файла: "+err.Error(),
		)
		bot.api.Send(msg)
		return
	}

	file := tgbotapi.FileBytes{
		Name:  "ACASbot_Results.xlsx",
		Bytes: fileBytes,
	}

	// Отправляем документ
	msg := tgbotapi.NewDocument(
		message.Chat.ID,
		file,
	)
	msg.Caption = "Локальная копия результатов анализа"
	msg.ReplyToMessageID = message.MessageID

	if _, err := bot.api.Send(msg); err != nil {
		log.Printf("Ошибка отправки файла: %v", err)
		errorMsg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не удалось отправить файл: "+err.Error(),
		)
		bot.api.Send(errorMsg)
	}
}
