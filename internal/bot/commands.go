package bot

import (
	"Unbewohnte/ACASbot/internal/domain"
	"Unbewohnte/ACASbot/internal/similarity"
	"Unbewohnte/ACASbot/internal/spreadsheet"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tealeg/xlsx"
)

type Command struct {
	Name        string
	Description string
	Example     string
	Group       string
	Call        func(string) (string, error)
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

func (bot *Bot) Help(args string) (string, error) {
	if strings.TrimSpace(args) != "" {
		// Ответить лишь по конкретной команде
		command := bot.CommandByName(args)
		if command != nil {
			return constructCommandHelpMessage(*command), nil
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

	return helpMessage, nil
}

func (bot *Bot) ChangeObj(args string) (string, error) {
	if args == "" {
		return "", errors.New("имя объекта не указано")
	}

	bot.conf.Analysis.Object = args

	// Обновляем конфигурационный файл
	bot.conf.Update()

	return fmt.Sprintf("Объект сменен на \"%s\"", bot.conf.Analysis.Object), nil
}

func (bot *Bot) formatAnalysisResult(art *domain.Article) string {
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

func (bot *Bot) Do(args string) (string, error) {
	if args == "" {
		return "", errors.New("вы не указали URL")
	}

	if !strings.HasPrefix(args, "http") {
		return "", errors.New("пожалуйста, отправьте действительный URL, начинающийся с http/https")
	}

	url := args

	// Анализируем статью
	art, err := bot.analyzeArticle(url)
	if err != nil {
		return "", fmt.Errorf("ошибка обработки страницы: %w", err)
	}
	if art.PublishedAt == 0 {
		now := time.Now()
		art.PublishedAt = now.Unix()
	}

	// Проверка точного дубликата
	if existingArticle, err := bot.conf.GetDB().GetExactDuplicate(art.Content); err == nil && existingArticle != nil {
		return bot.notifyExactDuplicate(existingArticle), nil
	}

	// Получение вектора
	embedding, err := bot.model.GetEmbedding(art.Content)
	if err != nil {
		return "", errors.New("ошибка векторизации")
	}

	// Поиск схожих статей
	similar, err := bot.conf.GetDB().FindSimilar(
		embedding,
		bot.conf.Analysis.VectorSimilarityThreshold,
		uint(bot.conf.Analysis.DaysLookback),
	)
	if err != nil {
		return "", errors.New("ошибка нахождения схожих статей")
	}

	var verified []domain.Article
	if len(similar) > 0 {
		composite := similarity.NewCompositeSimilarity(bot.conf.Analysis.CompositeVectorWeight)
		for _, candidate := range similar {
			score, err := composite.Compare(
				art.Content,
				candidate.Content,
				embedding,
				candidate.Embedding,
			)
			if err == nil && score >= bot.conf.Analysis.FinalSimilarityThreshold {
				candidate.TrueSimilarity = score
				verified = append(verified, candidate)

				// Добавляем ссылку на текущую статью в оригинальную
				if err := bot.conf.GetDB().AddSimilarURL(candidate.ID, url); err != nil {
					log.Printf("ошибка добавления URL в оригинальную статью: %v", err)
				}

				// Инкремент цитирований
				if err := bot.conf.GetDB().IncrementCitation(candidate.ID); err != nil {
					log.Printf("ошибка инкремента количества цитирований: %v", err)
				}
			}
		}
	}

	// Устанавливаем флаг оригинальности для новой статьи
	art.Original = len(verified) == 0

	// Сохранение статьи в базу
	if len(verified) == 0 || bot.conf.Analysis.SaveSimilarArticles {
		if err := bot.saveNewArticle(art, embedding, url); err != nil {
			return "", errors.New("ошибка сохранения")
		}
	}

	duplicatesText := bot.generateDuplicatesMessage(verified, *art)

	// Формирование итогового сообщения
	responseText := bot.formatAnalysisResult(art)
	fullMessage := "📋 *Результаты анализа*\n" + responseText
	if duplicatesText != "" {
		fullMessage += "\n\n" + duplicatesText
	}

	// Обработка Google Sheets
	if bot.conf.Sheets.PushToGoogleSheet {
		if err := bot.sheet.AddAnalysisResultWithRetry(art, 3); err != nil {
			log.Printf("ошибка добавления в Google Sheet: %v", err)
			fullMessage += "\n\n❌ ошибка внесения изменений в онлайн таблицу: " + err.Error()
		} else {
			fullMessage += "\n\n💾 запись успешно добавлена в онлайн таблицу!"
		}
	}

	return fullMessage, nil
}

func (bot *Bot) About(args string) (string, error) {
	return `ACAS bot (Article Context And Sentiment bot).

Бот для анализа статей на отношение к определенной объекта/личности, а также получения некоторых метаданных: заголовка и краткого описания.
Результаты анализа могут автоматически добавляться в Google таблицу, локальную таблицу при настройке.

Source: https://github.com/Unbewohnte/ACASbot
Лицензия: GPLv3
`, nil
}

func (bot *Bot) AddUser(args string) (string, error) {
	if args == "" {
		return "", errors.New("ID пользователя не указан")
	}

	id, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		return "", errors.New("неверный ID пользователя")
	}

	for _, allowedID := range bot.conf.Telegram.AllowedUserIDs {
		if id == allowedID {
			return "Этот пользователь уже есть в списке разрешенных.", nil
		}
	}

	bot.conf.Telegram.AllowedUserIDs = append(bot.conf.Telegram.AllowedUserIDs, id)

	// Сохраним в файл
	bot.conf.Update()

	return "Пользователь успешно добавлен", nil
}

func (bot *Bot) TogglePublicity(args string) (string, error) {
	if bot.conf.Telegram.Public {
		bot.conf.Telegram.Public = false
		bot.conf.Update()
		return "Доступ к боту теперь только у избранных.", nil
	} else {
		bot.conf.Telegram.Public = true
		bot.conf.Update()
		return "Доступ к боту теперь у всех.", nil
	}
}
func (bot *Bot) RemoveUser(args string) (string, error) {
	if args == "" {
		return "", errors.New("ID пользователя не указан")
	}

	id, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		return "", errors.New("неверный ID пользователя")
	}

	found := false
	newAllowedUserIDs := []int64{}
	for _, allowedID := range bot.conf.Telegram.AllowedUserIDs {
		if allowedID == id {
			found = true
			continue
		}
		newAllowedUserIDs = append(newAllowedUserIDs, allowedID)
	}

	if !found {
		return "", errors.New("пользователь не найден в списке разрешенных")
	}

	bot.conf.Telegram.AllowedUserIDs = newAllowedUserIDs
	bot.conf.Update()

	return "Пользователь успешно удален!", nil
}

func (bot *Bot) ChangeMaxContentSize(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано новое значение")
	}

	newMaxContentSize, err := strconv.ParseUint(args, 10, 64)
	if err != nil {
		return "", errors.New("указано некорректное значение")
	}

	if newMaxContentSize <= 0 {
		return "", errors.New("указано некорректное значение. Необходимо указать значение > 0")
	}

	bot.conf.Analysis.MaxContentSize = uint(newMaxContentSize)
	bot.conf.Update()

	return "Значение лимита символов текста статьи для анализа успешно изменено на " +
		strconv.FormatUint(newMaxContentSize, 10) + " символов.", nil
}

func (bot *Bot) PrintConfig(args string) (string, error) {
	var response strings.Builder

	response.WriteString("*Нынешняя конфигурация*: \n")
	response.WriteString("\n*[АНАЛИЗ]*\n")
	response.WriteString(fmt.Sprintf("*Запоминать статьи на*: `%v` дней\n", bot.conf.Analysis.DaysLookback))
	response.WriteString(fmt.Sprintf("*Лимит символов текста статьи для анализа*: `%v`\n", bot.conf.Analysis.MaxContentSize))
	response.WriteString(fmt.Sprintf("*Порог векторного сходства*: `%v` (%v%%)\n",
		bot.conf.Analysis.VectorSimilarityThreshold,
		bot.conf.Analysis.VectorSimilarityThreshold*100.0))
	response.WriteString(fmt.Sprintf("*Веса композитного сходства*: `%.2f` (Векторный: %.2f%%; Текстовый: %.2f%%)\n",
		bot.conf.Analysis.CompositeVectorWeight,
		bot.conf.Analysis.CompositeVectorWeight*100.0,
		(1.0-bot.conf.Analysis.CompositeVectorWeight)*100.0))
	response.WriteString(fmt.Sprintf("*Конечный порог сходства*: `%v` (%v%%)\n",
		bot.conf.Analysis.FinalSimilarityThreshold,
		bot.conf.Analysis.FinalSimilarityThreshold*100.0))
	response.WriteString(fmt.Sprintf("*Объект*: `%v`\n", bot.conf.Analysis.Object))
	response.WriteString(fmt.Sprintf("*Метаданные объекта*: `%v`\n", bot.conf.Analysis.ObjectMetadata))
	response.WriteString(fmt.Sprintf("*Сохранять похожие статьи*: `%v`\n", bot.conf.Analysis.SaveSimilarArticles))

	response.WriteString("\n*[ОБЩЕЕ]*:\n")
	response.WriteString(fmt.Sprintf("*Общедоступный?*: `%v`\n", bot.conf.Telegram.Public))
	response.WriteString(fmt.Sprintf("*Разрешенные пользователи*: `%+v`\n", bot.conf.Telegram.AllowedUserIDs))

	response.WriteString("\n*[LLM]*:\n")
	response.WriteString(fmt.Sprintf("*LLM*: `%v`\n", bot.conf.Ollama.GeneralModel))
	response.WriteString(fmt.Sprintf("*Эмбеддинговая LLM*: `%v`\n", bot.conf.Ollama.EmbeddingModel))
	response.WriteString(fmt.Sprintf("*Временной лимит на ответ LLM*: `%v` секунд\n", bot.conf.Ollama.QueryTimeoutSeconds))
	response.WriteString(fmt.Sprintf("*Промпт заголовка*: `%v`\n", bot.conf.Ollama.Prompts.Title))
	response.WriteString(fmt.Sprintf("*Промпт связи с объектом*: `%v`\n", bot.conf.Ollama.Prompts.Affiliation))
	response.WriteString(fmt.Sprintf("*Промпт отношения к объекту*: `%v`\n", bot.conf.Ollama.Prompts.Sentiment))

	response.WriteString("\n*[ТАБЛИЦЫ]*:\n")
	response.WriteString(fmt.Sprintf("*Отправлять результат анализа в Google таблицу?*: `%v`\n", bot.conf.Sheets.PushToGoogleSheet))
	response.WriteString(fmt.Sprintf("*Наименование листа таблицы*: `%v`\n", bot.conf.Sheets.Google.Config.SheetName))
	response.WriteString(fmt.Sprintf("*ID Google таблицы*: `%v`\n", bot.conf.Sheets.Google.Config.SpreadsheetID))

	return response.String(), nil
}

func (bot *Bot) ChangeSpreadsheetID(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано новое значение")
	}

	bot.conf.Sheets.Google.Config.SpreadsheetID = args
	if bot.sheet != nil {
		bot.sheet.SpreadsheetID = bot.conf.Sheets.Google.Config.SpreadsheetID
	}

	bot.conf.Update()

	return "ID Google таблицы успешно изменен на: " + args, nil
}

func (bot *Bot) ChangeSheetName(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано новое имя")
	}

	bot.conf.Sheets.Google.Config.SheetName = args
	if bot.sheet != nil {
		bot.sheet.SheetName = bot.conf.Sheets.Google.Config.SheetName
	}

	bot.conf.Update()

	return "Имя листа Google таблицы успешно изменено на: " + args, nil
}

func (bot *Bot) ChangeQueryTimeout(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано количество секунд")
	}

	timeoutSeconds, err := strconv.ParseUint(args, 10, 64)
	if err != nil {
		return "", errors.New("неверное значение количества секунд")
	}

	bot.conf.Ollama.QueryTimeoutSeconds = uint(timeoutSeconds)
	bot.model.TimeoutSeconds = bot.conf.Ollama.QueryTimeoutSeconds

	bot.conf.Update()

	return fmt.Sprintf("Время таймаута запросов к LLM успешно изменено на %d секунд", timeoutSeconds), nil
}
func (bot *Bot) GeneralQuery(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указан запрос")
	}

	answer, err := bot.model.Query(args)
	if err != nil {
		return "", fmt.Errorf("не удалось ответить на запрос: %w", err)
	}

	return answer, nil
}
func (bot *Bot) SetObjectData(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указана дополнительная информация об объекте")
	}

	bot.conf.Analysis.ObjectMetadata = strings.TrimSpace(args)
	bot.conf.Update()

	return "Информация об объекте успешно обновлена", nil
}

type promptType string

const (
	PROMPT_AFFILIATION promptType = "affiliation"
	PROMPT_TITLE       promptType = "title"
	PROMPT_SENTIMENT   promptType = "sentiment"
)

func (bot *Bot) setPrompt(args string, promptType promptType) (string, error) {
	if args == "" {
		return "", errors.New("не указан новый промпт")
	}

	switch promptType {
	case PROMPT_TITLE:
		bot.conf.Ollama.Prompts.Title = args
	case PROMPT_AFFILIATION:
		bot.conf.Ollama.Prompts.Affiliation = args
	case PROMPT_SENTIMENT:
		bot.conf.Ollama.Prompts.Sentiment = args
	default:
		return "", errors.New("неизвестный тип промпта")
	}

	bot.conf.Update()

	return "Новый промпт успешно применен", nil
}

func (bot *Bot) SetAffiliationPrompt(args string) (string, error) {
	return bot.setPrompt(args, PROMPT_AFFILIATION)
}

func (bot *Bot) SetTitlePrompt(args string) (string, error) {
	return bot.setPrompt(args, PROMPT_TITLE)
}

func (bot *Bot) SetSentimentPrompt(args string) (string, error) {
	return bot.setPrompt(args, PROMPT_SENTIMENT)
}
func (bot *Bot) ListModels(args string) (string, error) {
	models, err := bot.model.ListModels()
	if err != nil {
		return "", fmt.Errorf("не удалось получить список локальных моделей: %w", err)
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

	return response, nil
}
func (bot *Bot) SetModel(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано имя модели")
	}

	newModel := strings.TrimSpace(args)
	availableModels, err := bot.model.ListModels()
	if err != nil {
		return "", fmt.Errorf("не удалось получить список локальных моделей: %w", err)
	}

	for _, availableModel := range availableModels {
		if availableModel.Name == newModel {
			bot.model.ModelName = newModel
			bot.conf.Ollama.GeneralModel = newModel
			bot.conf.Update()
			return fmt.Sprintf("Модель успешно сменена на \"%s\"", bot.model.ModelName), nil
		}
	}

	return fmt.Sprintf("Такой модели не существует, оставлена \"%s\"", bot.model.ModelName), nil
}

func (bot *Bot) ToggleSaveSimilar(args string) (string, error) {
	if bot.conf.Analysis.SaveSimilarArticles {
		bot.conf.Analysis.SaveSimilarArticles = false
		bot.conf.Update()
		return "Сохранение похожих статей запрещено.", nil
	} else {
		bot.conf.Analysis.SaveSimilarArticles = true
		bot.conf.Update()
		return "Сохранение похожих статей разрешено.", nil
	}
}

func (bot *Bot) ChangeVectorSimilarityThreshold(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано новое значение")
	}

	newThreshold, err := strconv.ParseFloat(args, 64)
	if err != nil || newThreshold < 0 || newThreshold > 1.0 {
		return "", errors.New("некорректное значение. Используйте число от 0.0 до 1.0")
	}

	bot.conf.Analysis.VectorSimilarityThreshold = newThreshold
	bot.conf.Update()

	return fmt.Sprintf("Порог векторной схожести успешно изменен на %.2f (%.0f%%)",
		newThreshold, newThreshold*100.0), nil
}
func (bot *Bot) ChangeDaysLookback(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано новое значение дней")
	}

	newDaysLookback, err := strconv.ParseUint(args, 10, 64)
	if err != nil {
		return "", errors.New("указано некорректное значение")
	}

	if newDaysLookback <= 0 {
		return "", errors.New("указано некорректное значение. Необходимо указать значение дней > 0")
	}

	bot.conf.Analysis.DaysLookback = uint(newDaysLookback)
	bot.conf.Update()

	return fmt.Sprintf("Значение дней для поиска изменено на %d дней", newDaysLookback), nil
}

func (bot *Bot) ChangeFinalSimilarityThreshold(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано новое значение")
	}

	newThreshold, err := strconv.ParseFloat(args, 64)
	if err != nil {
		return "", errors.New("указано некорректное значение")
	}

	if newThreshold < 0 || newThreshold > 1.0 {
		return "", errors.New("указано некорректное значение. Необходимо указать значение 0.0 < значение < 1.0")
	}

	oldThreshold := bot.conf.Analysis.FinalSimilarityThreshold
	bot.conf.Analysis.FinalSimilarityThreshold = newThreshold
	bot.conf.Update()

	return fmt.Sprintf("Конечный порог схожести успешно изменен с %.2f на %.2f (%.0f%%)",
		oldThreshold, newThreshold, newThreshold*100.0), nil
}

func (bot *Bot) ChangeCompositeWeights(args string) (string, error) {
	if args == "" {
		return "", errors.New("не указано новое значение")
	}

	newWeight, err := strconv.ParseFloat(args, 64)
	if err != nil || newWeight < 0 || newWeight > 1.0 {
		return "", errors.New("некорректное значение. Используйте число от 0.0 до 1.0")
	}

	bot.conf.Analysis.CompositeVectorWeight = newWeight
	bot.conf.Update()

	return fmt.Sprintf("Веса композитного сходства успешно изменены: %.2f (Векторный: %.0f%%, Текстовый: %.0f%%)",
		newWeight, newWeight*100.0, (1.0-newWeight)*100.0), nil
}

func (bot *Bot) ForgetArticles(args string) (string, error) {
	err := bot.conf.GetDB().DeleteAllArticles()
	if err != nil {
		return "", fmt.Errorf("не удалось удалить статьи: %w", err)
	}

	return "Все статьи успешно \"забыты\"", nil
}

func (bot *Bot) GenerateSpreadsheet(args string) (string, error) {
	articles, err := bot.conf.GetDB().GetAllArticles()
	if err != nil {
		return "", fmt.Errorf("ошибка загрузки статей: %w", err)
	}

	// Генерируем Excel в памяти
	fileBuffer, err := spreadsheet.GenerateFromDatabase(articles)
	if err != nil {
		return "", fmt.Errorf("ошибка генерации файла: %w", err)
	}

	// Сохраняем как файл
	fileName := "ACASbot_Results.xlsx"
	file, err := os.Create(fileName)
	if err != nil {
		return "", fmt.Errorf("ошибка сохранения файла: %w", err)
	}
	defer file.Close()
	_, err = file.Write(fileBuffer.Bytes())
	if err != nil {
		return "", fmt.Errorf("ошибка записи файла: %w", err)
	}

	return fmt.Sprintf("Таблица успешно сгенерирована и сохранена как %s", fileName), nil
}

func (bot *Bot) SaveLocalSpreadsheet(args string) (string, error) {
	articles, err := bot.conf.GetDB().GetAllArticles()
	if err != nil {
		return "", err
	}

	// Генерируем Excel в памяти
	fileBuffer, err := spreadsheet.GenerateFromDatabase(articles)
	if err != nil {
		return "", err
	}

	// Сохраняем как файл
	fileName := "ACASbot_Results.xlsx"
	file, err := os.Create(fileName)
	if err != nil {
		return "", err
	}
	defer file.Close()
	_, err = file.Write(fileBuffer.Bytes())
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Локальная таблица успешно сохранена как %s", fileName), nil
}

func (bot *Bot) FindSimilar(args string) (string, error) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return "", errors.New("вы не указали URL")
	}

	url := parts[0]
	if !strings.HasPrefix(url, "http") {
		return "", errors.New("пожалуйста, отправьте действительный URL, начинающийся с http/https")
	}

	// Извлекаем содержимое статьи
	art, err := bot.getArticle(url)
	if err != nil {
		return "", fmt.Errorf("ошибка загрузки статьи: %w", err)
	}

	// Получаем эмбеддинг
	embedding, err := bot.model.GetEmbedding(art.Content)
	if err != nil {
		return "", errors.New("ошибка векторизации")
	}

	// Ищем похожие статьи
	similar, err := bot.conf.GetDB().FindSimilar(
		embedding,
		bot.conf.Analysis.VectorSimilarityThreshold,
		uint(bot.conf.Analysis.DaysLookback),
	)
	if err != nil {
		return "", errors.New("ошибка поиска похожих статей")
	}

	// Формируем результат
	if len(similar) == 0 {
		return "✅ Похожие статьи не найдены", nil
	}

	// Проверка точных дубликатов
	if existing, err := bot.conf.GetDB().GetExactDuplicate(art.Content); err == nil && existing != nil {
		return fmt.Sprintf("⚠️ Найден точный дубликат: %s\nURL: %s", existing.Title, existing.SourceURL), nil
	}

	// Проверка с использованием композитного сходства
	composite := similarity.NewCompositeSimilarity(bot.conf.Analysis.CompositeVectorWeight)
	var verified []domain.Article
	for _, candidate := range similar {
		score, err := composite.Compare(
			art.Content,
			candidate.Content,
			embedding,
			candidate.Embedding,
		)
		if err == nil && score >= bot.conf.Analysis.FinalSimilarityThreshold {
			candidate.TrueSimilarity = score
			verified = append(verified, candidate)
		}
	}

	// Формируем сообщение с результатами
	if len(verified) == 0 {
		return "✅ Похожие статьи не найдены (после применения композитного сходства)", nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("🔍 Найдено %d похожих статей:\n\n", len(verified)))

	for i, article := range verified {
		result.WriteString(fmt.Sprintf("%d. *%s*\n", i+1, article.Title))
		result.WriteString(fmt.Sprintf("   🔗 [Источник](%s)\n", article.SourceURL))
		result.WriteString(fmt.Sprintf("   💡 Сходство: %.2f%%\n\n", article.TrueSimilarity*100))
	}

	return result.String(), nil
}

func ParseExcelDate(cellValue string) (time.Time, error) {
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

func (bot *Bot) LoadXLSX(args string) (string, error) {
	// В новой системе args должен содержать путь к XLSX-файлу
	if args == "" {
		return "", errors.New("укажите путь к XLSX файлу")
	}

	// Проверяем расширение файла
	if !strings.HasSuffix(args, ".xlsx") {
		return "", errors.New("формат файла должен быть .xlsx")
	}

	// Проверяем существование файла
	if _, err := os.Stat(args); os.IsNotExist(err) {
		return "", fmt.Errorf("файл %s не найден", args)
	}

	// Парсим XLSX
	xlFile, err := xlsx.OpenFile(args)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения XLSX файла: %w", err)
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
				skipCount++
				continue
			}

			title := strings.TrimSpace(cells[2].String())
			sourceURL := strings.TrimSpace(cells[3].String())
			if title == "" || sourceURL == "" {
				skipCount++
				continue
			}

			// Парсим дату публикации
			pubDate, err := ParseExcelDate(cells[0].String())
			if err != nil {
				log.Printf("Failed to parse date: %s", err)
				pubDate = time.Now()
			}

			// Формируем статью
			art := &domain.Article{
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

	return fmt.Sprintf(
		"✅ Успешно загружено: %d статей\n🚫 Пропущено (дубликаты/ошибки): %d",
		successCount, skipCount,
	), nil
}

func (bot *Bot) SendLogs(args string) (string, error) {
	// Проверяем, существует ли файл логов
	if _, err := os.Stat(bot.conf.LogsFile); os.IsNotExist(err) {
		return "", errors.New("файл логов не найден")
	}

	// Читаем лог-файл
	logContent, err := os.ReadFile(bot.conf.LogsFile)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения файла логов: %w", err)
	}

	// Проверяем размер файла
	if len(logContent) > 50*1024*1024 { // 50MB
		return "", errors.New("файл логов слишком большой (максимум 50MB)")
	}

	// Для веб-интерфейса возвращаем содержимое логов
	return string(logContent), nil
}

func (bot *Bot) SetXLSXColumns(args string) (string, error) {
	if args == "" {
		return "", errors.New("укажите JSON с настройкой колонок")
	}

	var columns []domain.XLSXColumn
	if err := json.Unmarshal([]byte(args), &columns); err != nil {
		return "", errors.New("неверный формат JSON")
	}

	// Сохраняем в общий конфиг
	bot.conf.Sheets.XLSXColumns = columns
	bot.conf.Update()

	return "Конфиг колонок XLSX обновлен", nil
}

func (bot *Bot) ShowXLSXColumns(args string) (string, error) {
	columnsJSON, err := json.MarshalIndent(bot.conf.Sheets.XLSXColumns, "", "  ")
	if err != nil {
		return "", errors.New("ошибка форматирования конфигурации")
	}

	return fmt.Sprintf("Текущие колонки XLSX:\n```json\n%s\n```", string(columnsJSON)), nil
}

func (bot *Bot) TogglePushToGoogleSheets(args string) (string, error) {
	bot.conf.Sheets.PushToGoogleSheet = !bot.conf.Sheets.PushToGoogleSheet
	bot.conf.Update()

	if bot.conf.Sheets.PushToGoogleSheet {
		return "Добавление данных в гугл таблицу включено.", nil
	} else {
		return "Добавление данных в гугл таблицу отключено.", nil
	}
}
