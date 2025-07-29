package bot

import (
	"Unbewohnte/ACASbot/internal/article"
	"Unbewohnte/ACASbot/internal/similarity"
	"Unbewohnte/ACASbot/internal/spreadsheet"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tealeg/xlsx"
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
	for i := range bot.commands {
		if bot.commands[i].Name == name {
			return &bot.commands[i]
		}
	}

	return nil
}

func constructCommandHelpMessage(command Command) string {
	commandHelp := ""
	commandHelp += fmt.Sprintf("\n*Команда:* \"%s\"\n*Описание:* %s\n", command.Name, command.Description)
	if command.Example != "" {
		commandHelp += fmt.Sprintf("*Пример:* `%s`\n", command.Example)
	}

	return commandHelp
}

func (bot *Bot) Help(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) >= 2 {
		// Ответить лишь по конкретной команде
		command := bot.CommandByName(parts[1])
		if command != nil {
			helpMessage := constructCommandHelpMessage(*command)
			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				helpMessage,
			)
			msg.ParseMode = "Markdown"
			bot.api.Send(msg)
			return
		}
	}

	var helpMessage string

	commandsByGroup := make(map[string][]Command)
	for _, command := range bot.commands {
		commandsByGroup[command.Group] = append(commandsByGroup[command.Group], command)
	}

	groups := []string{}
	for g := range commandsByGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	for _, group := range groups {
		helpMessage += fmt.Sprintf("\n\n*[%s]*\n", group)
		for _, command := range commandsByGroup[group] {
			helpMessage += constructCommandHelpMessage(command)
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

	bot.conf.Analysis.Object = strings.Join(parts[1:], " ")
	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		fmt.Sprintf("Объект сменен на \"%s\"", bot.conf.Analysis.Object),
	)

	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) formatAnalysisResult(art *article.Article) string {
	var response strings.Builder

	// Добавляем заголовок
	response.WriteString(fmt.Sprintf("*Заголовок:* %s\n\n", art.Title))

	// Дата публикации
	if art.PublishedAt != 0 {
		pubDate := time.Unix(art.PublishedAt, 0)
		response.WriteString(
			fmt.Sprintf("*Дата публикации:* %s\n\n",
				fmt.Sprintf("%d/%d/%d",
					pubDate.Day(),
					pubDate.Month(),
					pubDate.Year(),
				),
			),
		)
	}

	response.WriteString(fmt.Sprintf("*Связь с \"%s\":* %s\n\n", bot.conf.Analysis.Object, art.Affiliation))

	// Добавляем отношение
	if art.Sentiment != "" {
		response.WriteString(fmt.Sprintf("*Отношение:* %s\n", art.Sentiment))
		if art.Justification != "" {
			response.WriteString(fmt.Sprintf("*Обоснование:* %s\n", art.Justification))
		}
	}

	// Добавляем ошибки (если есть)
	if len(art.Errors) > 0 {
		response.WriteString("\n⚠️ *Ошибки при анализе:*\n")
		for _, err := range art.Errors {
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

	processingMsg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Анализирую статью...")
	processingMsg.ReplyToMessageID = message.MessageID
	sentMsg, _ := bot.api.Send(processingMsg)
	defer func() {
		deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID)
		bot.api.Send(deleteMsg)
	}()

	// Анализируем статью
	art, err := bot.analyzeArticle(url)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка обработки страницы: "+err.Error())
		errorMsg.ReplyToMessageID = message.MessageID
		bot.api.Send(errorMsg)
		return
	}
	if art.PublishedAt == 0 {
		now := time.Now()
		art.PublishedAt = now.Unix()
	}

	// Проверка на похожесть
	userConfig, err := bot.conf.GetDB().GetUserConfig(message.From.ID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка загрузки ваших настроек", message.MessageID)
		return
	}

	// Проверка точного дубликата
	if existingArticle, err := bot.conf.GetDB().GetExactDuplicate(art.Content); err != nil {
		bot.sendError(message.Chat.ID, "Ошибка проверки дубликатов", message.MessageID)
		return
	} else if existingArticle != nil {
		bot.notifyExactDuplicate(message, existingArticle)
		return
	}

	// Получение вектора
	embedding, err := bot.model.GetEmbedding(art.Content)
	if err != nil {
		log.Printf("Не получилось bot.model.GetEmbedding(art.Content): %s", err)
		bot.sendError(message.Chat.ID, "Ошибка векторизации", message.MessageID)
		return
	}

	// Поиск схожих статей
	similar, err := bot.conf.GetDB().FindSimilar(
		embedding,
		userConfig.VectorSimilarityThreshold,
		userConfig.DaysLookback,
	)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка нахождения схожих статей", message.MessageID)
		return
	}

	var verified []article.Article
	if len(similar) > 0 {
		composite := similarity.NewCompositeSimilarity(userConfig.CompositeVectorWeight)
		for _, candidate := range similar {
			score, err := composite.Compare(
				art.Content,
				candidate.Content,
				embedding,
				candidate.Embedding,
			)
			candidate.TrueSimilarity = score

			if err == nil && score >= userConfig.FinalSimilarityThreshold {
				bot.conf.GetDB().IncrementCitation(candidate.ID)
				candidate.Citations += 1
				verified = append(verified, candidate)
			}

			if bot.conf.Debug {
				log.Printf("Кандидат: True: %.3f; Vector: %.3f", candidate.TrueSimilarity, candidate.Similarity)
			}
		}
	}

	// Установка нескольких ссылок
	if len(verified) > 0 {
		art.SimilarURLs = make([]string, 0, len(verified))
		for _, a := range verified {
			art.SimilarURLs = append(art.SimilarURLs, a.SourceURL)
		}
	}

	if len(verified) == 0 {
		// Уникальная
		if err := bot.saveNewArticle(art, embedding, url); err != nil {
			bot.sendError(message.Chat.ID, "Ошибка сохранения", message.MessageID)
			log.Printf("Ошибка сохранения: %s", err)
			return
		}
	} else if len(verified) > 0 && bot.conf.Analysis.SaveSimilarArticles {
		if err := bot.saveNewArticle(art, embedding, url); err != nil {
			bot.sendError(message.Chat.ID, "Ошибка сохранения", message.MessageID)
			log.Printf("Ошибка сохранения: %s", err)
			return
		}
		bot.sendSuccess(message.Chat.ID, "Статья успешно добавлена в базу", message.MessageID)
	} else {
		bot.sendSuccess(message.Chat.ID, "Статья не добавлена в базу, так как сохранение похожих статей не разрешено", message.MessageID)
	}

	duplicatesText := bot.generateDuplicatesMessage(verified, *art)

	// Формирование итогового сообщения
	responseText := bot.formatAnalysisResult(art)
	fullMessage := "📋 *Результаты анализа*\n" + responseText
	if duplicatesText != "" {
		fullMessage += "\n\n" + duplicatesText
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, fullMessage)
	msg.ParseMode = "Markdown"
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)

	// Дальнейшие действия (Google Sheets, локальное сохранение и т.д.)
	if bot.conf.Debug {
		log.Println(msg.Text)
	}
	if bot.conf.Sheets.PushToGoogleSheet {
		if err := bot.sheet.AddAnalysisResultWithRetry(art, 3); err != nil {
			log.Printf("Ошибка добавления в Google Sheet: %v", err)
			msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка внесения изменений в онлайн таблицу: "+err.Error())
			bot.api.Send(msg)
		} else {
			msg := tgbotapi.NewMessage(message.Chat.ID, "💾 Запись успешно добавлена в онлайн таблицу!")
			bot.api.Send(msg)
		}
	}
}

func (bot *Bot) About(message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		`ACAS bot (Article Context And Sentiment bot).

Бот для анализа статей на отношение к определенной объекта/личности, а также получения некоторых метаданных: заголовка и краткого описания.
Результаты анализа могут автоматически добавляться в Google таблицу, локальную таблицу при настройке.

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

	bot.conf.Analysis.MaxContentSize = uint(newMaxContentSize)

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
	userConfig, err := bot.conf.GetDB().GetUserConfig(message.From.ID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка загрузки ваших настроек", message.MessageID)
		return
	}

	var response string = ""

	response += "*Нынешняя конфигурация*: \n"
	response += "\n*[АНАЛИЗ]*\n"
	response += fmt.Sprintf("*Запоминать статьи на*: `%v` дней\n", userConfig.DaysLookback)
	response += fmt.Sprintf("*Лимит символов текста статьи для анализа*: `%v`\n", bot.conf.Analysis.MaxContentSize)
	response += fmt.Sprintf("*Порог векторного сходства*: `%v` (%v%%)\n",
		userConfig.VectorSimilarityThreshold,
		userConfig.VectorSimilarityThreshold*100.0,
	)
	response += fmt.Sprintf("*Веса композитного сходства*: `%.2f` (Векторный: %.2f%%; Текстовый: %.2f%%)\n",
		userConfig.CompositeVectorWeight,
		userConfig.CompositeVectorWeight*100.0,
		(1.0-userConfig.CompositeVectorWeight)*100.0,
	)
	response += fmt.Sprintf("*Конечный порог сходства*: `%v` (%v%%)\n",
		userConfig.FinalSimilarityThreshold,
		userConfig.FinalSimilarityThreshold*100.0,
	)
	response += fmt.Sprintf("*Объект*: `%v`\n", bot.conf.Analysis.Object)
	response += fmt.Sprintf("*Метаданные объекта*: `%v`\n", bot.conf.Analysis.ObjectMetadata)
	response += fmt.Sprintf("*Лимит символов текста статьи для анализа*: `%v`\n", bot.conf.Analysis.MaxContentSize)
	response += "\n*[ОБЩЕЕ]*:\n"
	response += fmt.Sprintf("*Общедоступный?*: `%v`\n", bot.conf.Telegram.Public)
	response += fmt.Sprintf("*Разрешенные пользователи*: `%+v`\n", bot.conf.Telegram.AllowedUserIDs)
	response += "\n*[LLM]*:\n"
	response += fmt.Sprintf("*LLM*: `%v`\n", bot.conf.Ollama.GeneralModel)
	response += fmt.Sprintf("*Эмбеддинговая LLM*: `%v`\n", bot.conf.Ollama.EmbeddingModel)
	response += fmt.Sprintf("*Временной лимит на ответ LLM*: `%v` секунд\n", bot.conf.Ollama.QueryTimeoutSeconds)
	response += fmt.Sprintf("*Промпт заголовка*: `%v`\n", bot.conf.Ollama.Prompts.Title)
	response += fmt.Sprintf("*Промпт связи с объектом*: `%v`\n", bot.conf.Ollama.Prompts.Affiliation)
	response += fmt.Sprintf("*Промпт отношения к объекту*: `%v`\n", bot.conf.Ollama.Prompts.Sentiment)
	response += "\n*[ТАБЛИЦЫ]*:\n"
	response += fmt.Sprintf("*Отправлять результат анализа в Google таблицу?*: `%v`\n", bot.conf.Sheets.PushToGoogleSheet)
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

	bot.conf.Analysis.ObjectMetadata = strings.TrimSpace(objData)

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
	PROMPT_AFFILIATION promptType = "affiliation"
	PROMPT_TITLE       promptType = "title"
	PROMPT_SENTIMENT   promptType = "sentiment"
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
	case PROMPT_SENTIMENT:
		bot.conf.Ollama.Prompts.Sentiment = prompt
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
	bot.setPrompt(message, PROMPT_SENTIMENT)
}

func (bot *Bot) ListModels(message *tgbotapi.Message) {
	models, err := bot.model.ListModels()
	if err != nil {
		errorMsg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не удалось получить список локальных моделей: "+err.Error(),
		)
		bot.api.Send(errorMsg)
		return
	}

	response := "Доступные модели:\n"
	for _, model := range models {
		response += fmt.Sprintf("`%s` (%s, %s)\n",
			model.Name,
			model.Details.ParameterSize,
			model.Details.QuantizationLevel,
		)
	}
	response += fmt.Sprintf("\nТекущая:\n `%s`\n", bot.model.ModelName)

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		response,
	)
	msg.ParseMode = "Markdown"
	bot.api.Send(msg)
}

func (bot *Bot) SetModel(message *tgbotapi.Message) {
	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указано имя модели.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	newModel, _ := strings.CutPrefix(message.Text, parts[0])
	newModel = strings.TrimSpace(newModel)

	availableModels, err := bot.model.ListModels()
	if err != nil {
		errorMsg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не удалось получить список локальных моделей: "+err.Error(),
		)
		bot.api.Send(errorMsg)
		return
	}

	for _, availableModel := range availableModels {
		if availableModel.Name == newModel {
			bot.model.ModelName = newModel
			bot.conf.Ollama.GeneralModel = newModel

			msg := tgbotapi.NewMessage(
				message.Chat.ID,
				fmt.Sprintf("Модель успешно сменена на \"%s\"", bot.model.ModelName),
			)
			bot.api.Send(msg)

			bot.conf.Update()
			return
		}
	}

	errorMsg := tgbotapi.NewMessage(
		message.Chat.ID,
		fmt.Sprintf("Такой модели не существует, оставлена \"%s\"", bot.model.ModelName),
	)
	bot.api.Send(errorMsg)
}

func (bot *Bot) ToggleSaveSimilar(message *tgbotapi.Message) {
	if bot.conf.Analysis.SaveSimilarArticles {
		bot.conf.Analysis.SaveSimilarArticles = false
		bot.api.Send(
			tgbotapi.NewMessage(message.Chat.ID, "Сохранение похожих статей запрещено."),
		)
	} else {
		bot.conf.Analysis.SaveSimilarArticles = true
		bot.api.Send(
			tgbotapi.NewMessage(message.Chat.ID, "Сохранение похожих статей разрешено."),
		)
	}

	// Обновляем конфигурационный файл
	bot.conf.Update()
}

func (bot *Bot) ChangeVectorSimilarityThreshold(message *tgbotapi.Message) {
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

	newThreshold, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || newThreshold < 0 || newThreshold > 1.0 {
		bot.sendError(message.Chat.ID, "Некорректное значение. Используйте число от 0.0 до 1.0", message.MessageID)
		return
	}

	// Get and update user config
	userConfig, err := bot.conf.GetDB().GetUserConfig(message.From.ID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка загрузки ваших настроек", message.MessageID)
		return
	}

	userConfig.VectorSimilarityThreshold = newThreshold
	if err := bot.conf.GetDB().SaveUserConfig(userConfig); err != nil {
		bot.sendError(message.Chat.ID, "Ошибка сохранения настроек", message.MessageID)
		return
	}

	bot.sendSuccess(message.Chat.ID, "Порог векторной схожести обновлён!", message.MessageID)
}

func (bot *Bot) ChangeDaysLookback(message *tgbotapi.Message) {
	userConfig, err := bot.conf.GetDB().GetUserConfig(message.From.ID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка загрузки ваших настроек", message.MessageID)
		return
	}

	parts := strings.Split(message.Text, " ")
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Не указано новое значение дней.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	newDaysLookback, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	if newDaysLookback <= 0 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение. Необходимо указать значение дней > 0",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	userConfig.DaysLookback = uint(newDaysLookback)
	if err := bot.conf.GetDB().SaveUserConfig(userConfig); err != nil {
		bot.sendError(message.Chat.ID, "Ошибка сохранения настроек", message.MessageID)
		return
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Значение успешно изменено.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
}

func (bot *Bot) ChangeFinalSimilarityThreshold(message *tgbotapi.Message) {
	userConfig, err := bot.conf.GetDB().GetUserConfig(message.From.ID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка загрузки ваших настроек", message.MessageID)
		return
	}

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

	newSimilarityThreshold, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	if newSimilarityThreshold < 0 || newSimilarityThreshold > 1.0 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение. Необходимо указать значение 0.0 < значение < 1.0",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	userConfig.FinalSimilarityThreshold = newSimilarityThreshold
	if err := bot.conf.GetDB().SaveUserConfig(userConfig); err != nil {
		bot.sendError(message.Chat.ID, "Ошибка сохранения настроек", message.MessageID)
		return
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Значение успешно изменено.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
}

func (bot *Bot) ChangeCompositeWeights(message *tgbotapi.Message) {
	userConfig, err := bot.conf.GetDB().GetUserConfig(message.From.ID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка загрузки ваших настроек", message.MessageID)
		return
	}

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

	newConpositeSimilarity, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение.",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	if newConpositeSimilarity < 0 || newConpositeSimilarity > 1.0 {
		msg := tgbotapi.NewMessage(
			message.Chat.ID,
			"Указано некорректное значение. Необходимо указать значение 0.0 < значение < 1.0",
		)
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
		return
	}

	userConfig.CompositeVectorWeight = newConpositeSimilarity
	if err := bot.conf.GetDB().SaveUserConfig(userConfig); err != nil {
		bot.sendError(message.Chat.ID, "Ошибка сохранения настроек", message.MessageID)
		return
	}

	msg := tgbotapi.NewMessage(
		message.Chat.ID,
		"Значение успешно изменено.",
	)
	msg.ReplyToMessageID = message.MessageID
	bot.api.Send(msg)
}

func (bot *Bot) ForgetArticles(message *tgbotapi.Message) {
	err := bot.conf.GetDB().DeleteAllArticles()
	if err != nil {
		bot.sendError(message.Chat.ID, "Не удалось удалить статьи", message.MessageID)
		return
	}

	bot.sendSuccess(message.Chat.ID, "Все статьи успешно \"забыты\"", message.MessageID)
}

func (bot *Bot) GenerateSpreadsheet(message *tgbotapi.Message) {
	articles, err := bot.conf.GetDB().GetAllArticles()
	if err != nil {
		log.Printf("Не вышло получить все статьи из базы данных: %s", err)
		bot.sendError(message.Chat.ID, "Ошибка загрузки статей", message.MessageID)
		return
	}

	// Генерируем Excel в памяти
	fileBuffer, err := spreadsheet.GenerateFromDatabase(articles)
	if err != nil {
		log.Printf("Не получилось сгенерировать XLSX файл: %s", err)
		bot.sendError(message.Chat.ID, "Ошибка генерации файла: "+err.Error(), message.MessageID)
		return
	}

	// Сохраняем как файл
	fileName := "ACASbot_Results.xlsx"
	realFile, err := os.Create(fileName)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка сохранения файла: "+err.Error(), message.MessageID)
	} else {
		defer realFile.Close()
		realFile.Write(fileBuffer.Bytes())
	}

	// Отправляем файл как документ
	file := tgbotapi.FileBytes{
		Name:  fileName,
		Bytes: fileBuffer.Bytes(),
	}

	msg := tgbotapi.NewDocument(message.Chat.ID, file)
	msg.Caption = "📊 Сгенерированная таблица на основе базы данных"
	msg.ReplyToMessageID = message.MessageID

	_, err = bot.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки файла: %v", err)
		bot.sendError(message.Chat.ID, "Не удалось отправить файл: "+err.Error(), message.MessageID)
	}
}

func (bot *Bot) SaveLocalSpreadsheet() error {
	articles, err := bot.conf.GetDB().GetAllArticles()
	if err != nil {
		return err
	}

	// Генерируем Excel в памяти
	fileBuffer, err := spreadsheet.GenerateFromDatabase(articles)
	if err != nil {
		return err
	}

	// Сохраняем как файл
	file, err := os.Create("ACASbot_Results.xlsx")
	if err != nil {
		return err
	}
	defer file.Close()
	file.Write(fileBuffer.Bytes())

	return nil
}

func (bot *Bot) FindSimilar(message *tgbotapi.Message) {
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

	// Показываем индикатор загрузки
	processingMsg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Ищу похожие статьи...")
	processingMsg.ReplyToMessageID = message.MessageID
	sentMsg, _ := bot.api.Send(processingMsg)
	defer func() {
		deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID)
		bot.api.Send(deleteMsg)
	}()

	// Извлекаем содержимое статьи
	art, err := bot.getArticle(url)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка загрузки статьи: "+err.Error())
		errorMsg.ReplyToMessageID = message.MessageID
		bot.api.Send(errorMsg)
		return
	}

	// Получаем пользовательские настройки
	userConfig, err := bot.conf.GetDB().GetUserConfig(message.From.ID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка загрузки настроек", message.MessageID)
		return
	}

	// Проверяем точные дубликаты
	if existing, err := bot.conf.GetDB().GetExactDuplicate(art.Content); err == nil && existing != nil {
		bot.notifyExactDuplicate(message, existing)
		return
	}

	// Получаем эмбеддинг
	embedding, err := bot.model.GetEmbedding(art.Content)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка векторизации", message.MessageID)
		return
	}

	// Ищем похожие статьи
	similar, err := bot.conf.GetDB().FindSimilar(
		embedding,
		userConfig.VectorSimilarityThreshold,
		userConfig.DaysLookback,
	)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка поиска похожих статей", message.MessageID)
		return
	}

	// Формируем сообщение с результатами
	var duplicatesText string
	if len(similar) > 0 {
		composite := similarity.NewCompositeSimilarity(userConfig.CompositeVectorWeight)
		var verified []article.Article

		for _, candidate := range similar {
			score, err := composite.Compare(
				art.Content,
				candidate.Content,
				embedding,
				candidate.Embedding,
			)
			if err == nil && score >= userConfig.FinalSimilarityThreshold {
				candidate.TrueSimilarity = score
				verified = append(verified, candidate)
			}
		}

		duplicatesText = bot.generateDuplicatesMessage(verified, *art)
	}

	// Формируем и отправляем результат
	if duplicatesText != "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, duplicatesText)
		msg.ParseMode = "Markdown"
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, "✅ Похожие статьи не найдены")
		msg.ReplyToMessageID = message.MessageID
		bot.api.Send(msg)
	}
}
func parseExcelDate(cellValue string) (time.Time, error) {
	// Удаляем лишние пробелы
	cellValue = strings.TrimSpace(cellValue)

	// 1. Пробуем распарсить как Excel serial number
	if serial, err := strconv.Atoi(cellValue); err == nil {
		// Excel date epoch is 1899-12-30 (note: not 31)
		baseDate := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		return baseDate.AddDate(0, 0, serial), nil
	}

	// 2. Пробуем стандартные форматы с четырехзначным годом
	formats := []string{
		"02.01.2006", // dd.mm.yyyy
		"02/01/2006", // dd/mm/yyyy
		"2006-01-02", // yyyy-mm-dd
		"01-02-2006", // mm-dd-yyyy (US format)
		"01.02.2006", // mm.dd.yyyy
		"02-01-2006", // dd-mm-yyyy
	}

	for _, format := range formats {
		if t, err := time.Parse(format, cellValue); err == nil {
			return t, nil
		}
	}

	// 3. Пробуем форматы с двухзначным годом (корректируем год)
	twoDigitYearFormats := []string{
		"02.01.06", // dd.mm.yy
		"02/01/06", // dd/mm/yy
		"01-02-06", // mm-dd-yy
		"01.02.06", // mm.dd.yy
		"02-01-06", // dd-mm-yy
	}

	for _, format := range twoDigitYearFormats {
		if t, err := time.Parse(format, cellValue); err == nil {
			// Корректируем год (предполагаем 00-79 -> 2000-2079, 80-99 -> 1980-1999)
			year := t.Year()
			if year >= 100 { // Если year уже четырехзначный (может быть в некоторых парсерах)
				return t, nil
			}
			if year >= 80 {
				t = t.AddDate(1900, 0, 0)
			} else {
				t = t.AddDate(2000, 0, 0)
			}
			return t, nil
		}
	}

	// 4. Пробуем разобрать вручную для форматов типа "07-23-25"
	if parts := strings.Split(cellValue, "-"); len(parts) == 3 && len(parts[2]) == 2 {
		monthStr := parts[0]
		dayStr := parts[1]
		yearStr := parts[2]

		// Преобразуем двухзначный год
		year, err := strconv.Atoi(yearStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid year: %s", yearStr)
		}

		// Корректируем год (00-79 -> 2000-2079, 80-99 -> 1980-1999)
		if year >= 80 {
			year += 1900
		} else {
			year += 2000
		}

		// Собираем полную дату и парсим
		fullDate := fmt.Sprintf("%s-%s-%04d", monthStr, dayStr, year)
		return time.Parse("01-02-2006", fullDate)
	}

	return time.Time{}, fmt.Errorf("unrecognized date format: %s", cellValue)
}

func (bot *Bot) LoadXLSX(message *tgbotapi.Message) {
	// Проверяем, есть ли прикрепленный файл
	if message.Document == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, прикрепите XLSX файл к команде")
		bot.api.Send(msg)
		return
	}

	// Проверяем расширение файла
	if !strings.HasSuffix(message.Document.FileName, ".xlsx") {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Формат файла должен быть .xlsx")
		bot.api.Send(msg)
		return
	}

	// Скачиваем файл
	fileURL, err := bot.api.GetFileDirectURL(message.Document.FileID)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка получения файла", message.MessageID)
		return
	}

	// Индикатор загрузки
	processingMsg := tgbotapi.NewMessage(message.Chat.ID, "📥 Загружаю файл...")
	sentMsg, _ := bot.api.Send(processingMsg)

	// Создаем временный файл
	tmpFile, err := os.CreateTemp("", "acasbot-*.xlsx")
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка создания временного файла", message.MessageID)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Скачиваем содержимое
	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf("Ошибка скачивания файла: %s", err)
		bot.sendError(message.Chat.ID, "Ошибка скачивания файла", message.MessageID)
		return
	}
	defer resp.Body.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		log.Printf("Ошибка сохранения файла: %s", err)
		bot.sendError(message.Chat.ID, "Ошибка сохранения файла", message.MessageID)
		return
	}

	// Парсим XLSX
	xlFile, err := xlsx.OpenFile(tmpFile.Name())
	if err != nil {
		log.Printf("Ошибка чтения файла: %s", err)
		bot.sendError(message.Chat.ID, "Ошибка чтения XLSX файла", message.MessageID)
		return
	}

	// Обрабатываем данные
	successCount := 0
	skipCount := 0
	db := bot.conf.GetDB()

	for _, sheet := range xlFile.Sheets {
		for i, row := range sheet.Rows {
			// Пропускаем заголовок
			if i == 0 || len(row.Cells) == 0 {
				continue
			}

			// Извлекаем данные из строки
			cells := row.Cells
			if len(cells) < 6 {
				continue
			}

			title := strings.TrimSpace(cells[2].String())
			sourceURL := strings.TrimSpace(cells[3].String())
			if title == "" || sourceURL == "" {
				skipCount++
				continue
			}

			// Парсим дату публикации
			pubDate, err := parseExcelDate(cells[0].String())
			if err != nil {
				log.Printf("Failed to parse date: %s", err)
				pubDate = time.Now()
			}

			// Формируем статью
			art := &article.Article{
				PublishedAt: pubDate.Unix(),
				Affiliation: cells[4].String(),
				Sentiment:   cells[5].String(),
				Title:       cells[2].String(),
				SourceURL:   cells[3].String(),
				CreatedAt:   time.Now().Unix(),
				SimilarURLs: []string{},
				Embedding:   []float64{},
			}

			// Проверяем дубликат по URL
			exists, err := db.HasArticleByURL(art.SourceURL)
			if err != nil || exists {
				skipCount++
				continue
			}

			// Сохраняем в БД
			if err := db.SaveArticle(art); err != nil {
				log.Printf("Ошибка сохранения в базу: %v", err)
				skipCount++
			} else {
				successCount++
			}
		}
	}

	// Отправляем отчет
	report := fmt.Sprintf(
		"✅ Успешно загружено: %d статей\n🚫 Пропущено (дубликаты/ошибки): %d",
		successCount, skipCount,
	)
	log.Printf("Загружено %d статей", successCount)

	// Удаляем индикатор
	deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID)
	bot.api.Send(deleteMsg)

	msg := tgbotapi.NewMessage(message.Chat.ID, report)
	bot.api.Send(msg)
}

func (bot *Bot) SendLogs(message *tgbotapi.Message) {
	// Check if log file exists
	if _, err := os.Stat(bot.conf.LogsFile); os.IsNotExist(err) {
		bot.sendError(message.Chat.ID, "Файл логов не найден", message.MessageID)
		return
	}

	// Read log file
	logFile, err := os.Open(bot.conf.LogsFile)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка чтения файла логов", message.MessageID)
		log.Printf("Error opening log file: %v", err)
		return
	}
	defer logFile.Close()

	// Get file stats
	fileInfo, err := logFile.Stat()
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка получения информации о файле", message.MessageID)
		log.Printf("Error getting file stats: %v", err)
		return
	}

	if fileInfo.Size() > 50*1024*1024 {
		bot.sendError(message.Chat.ID, "Файл логов слишком большой (максимум 50MB)", message.MessageID)
		return
	}

	// Read file content
	fileBytes, err := io.ReadAll(logFile)
	if err != nil {
		bot.sendError(message.Chat.ID, "Ошибка чтения содержимого файла", message.MessageID)
		log.Printf("Error reading log file: %v", err)
		return
	}

	// Create message with log file
	file := tgbotapi.FileBytes{
		Name:  "ACASbot_logs.txt",
		Bytes: fileBytes,
	}

	msg := tgbotapi.NewDocument(message.Chat.ID, file)
	msg.Caption = "📋 Логи бота"
	msg.ReplyToMessageID = message.MessageID

	// Send the file
	if _, err := bot.api.Send(msg); err != nil {
		bot.sendError(message.Chat.ID, "Ошибка отправки файла", message.MessageID)
		log.Printf("Error sending log file: %v", err)
	}
}
