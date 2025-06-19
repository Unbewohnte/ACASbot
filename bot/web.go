package bot

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

func (bot *Bot) ExtractWebContent(articleURL string) (ArticleContent, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header = via[0].Header.Clone()
			return nil
		},
	}

	req, err := http.NewRequest("GET", articleURL, nil)
	if err != nil {
		return ArticleContent{}, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header = http.Header{
		"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"},
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
		"Accept-Language": {"ru-RU,ru;q=0.8,en-US;q=0.5,en;q=0.3"},
		"Connection":      {"keep-alive"},
	}

	resp, err := client.Do(req)
	if err != nil {
		return ArticleContent{}, fmt.Errorf("ошибка загрузки страницы: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return ArticleContent{}, fmt.Errorf("HTTP статус %d: %s", resp.StatusCode, string(bodyBytes))
	}

	parsedURL, err := url.Parse(articleURL)
	if err != nil {
		return ArticleContent{}, fmt.Errorf("ошибка парсинга URL: %w", err)
	}

	// Пробуем go-readability в первую очередь
	article, err := readability.FromReader(resp.Body, parsedURL)
	if err == nil && len(article.TextContent) > 100 {
		return ArticleContent{
			Title:   article.Title,
			Content: article.TextContent,
			Success: true,
		}, nil
	}

	// Кодировка
	var reader io.Reader = resp.Body
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "charset=windows-1251") {
		reader = transform.NewReader(resp.Body, charmap.Windows1251.NewDecoder())
	} else if strings.Contains(contentType, "charset=ISO-8859-5") {
		reader = transform.NewReader(resp.Body, charmap.ISO8859_5.NewDecoder())
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(reader, 2*1024*1024))
	if err != nil {
		return ArticleContent{}, fmt.Errorf("ошибка чтения тела: %w", err)
	}

	// Проверяем CloudFlare
	if strings.Contains(string(bodyBytes), "Cloudflare") {
		return ArticleContent{}, fmt.Errorf("страница защищена CloudFlare")
	}

	// Пробуем кастомный парсинг
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return ArticleContent{}, fmt.Errorf("ошибка парсинга HTML: %w", err)
	}

	return bot.extractCustomContent(doc)
}

func (bot *Bot) extractCustomContent(doc *goquery.Document) (ArticleContent, error) {
	// Сначала пробуем структурированный подход
	if content := bot.extractStructuredContent(doc); content.Success {
		return content, nil
	}

	// Затем fallback-метод
	content, err := bot.extractFallbackContent(doc)
	if err != nil {
		return ArticleContent{}, err
	}

	return ArticleContent{
		Content: content,
		Success: false,
	}, nil
}

func (bot *Bot) extractStructuredContent(doc *goquery.Document) ArticleContent {
	articleSelection := doc.Find("article, main, .article, .post, .content")
	if articleSelection.Length() == 0 {
		return ArticleContent{Success: false}
	}

	var title string
	for _, selector := range []string{"h1", "h2", ".title", ".article-title"} {
		if title == "" {
			title = strings.TrimSpace(articleSelection.Find(selector).First().Text())
		}
	}

	content := strings.TrimSpace(articleSelection.Text())
	content = strings.Join(strings.Fields(content), " ")

	if len(content) < 100 || uint(len(content)) > bot.conf.MaxContentSize {
		return ArticleContent{Success: false}
	}

	return ArticleContent{
		Title:   title,
		Content: content,
		Success: true,
	}
}

func (bot *Bot) extractFallbackContent(doc *goquery.Document) (string, error) {
	// Очистка документа
	doc.Find("script, style, noscript, iframe, nav, footer").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})

	// Поиск основного контента
	mainContent := ""
	doc.Find("p, div, article").Each(func(i int, s *goquery.Selection) {
		if text := strings.TrimSpace(s.Text()); len(text) > len(mainContent) {
			mainContent = text
		}
	})

	if len(mainContent) < 500 {
		mainContent = strings.TrimSpace(doc.Find("body").Text())
	}

	mainContent = strings.Join(strings.Fields(mainContent), " ")
	if len(mainContent) < 100 {
		return "", fmt.Errorf("недостаточно текста")
	}

	if uint(len(mainContent)) > bot.conf.MaxContentSize {
		mainContent = mainContent[:bot.conf.MaxContentSize]
	}

	return mainContent, nil
}

func (bot *Bot) analyzeArticle(msg *tgbotapi.Message) {
	responseMsg := tgbotapi.NewMessage(msg.Chat.ID, "")
	responseMsg.ReplyToMessageID = msg.MessageID

	articleContent, err := bot.ExtractWebContent(msg.Text)
	if err != nil {
		log.Printf("Ошибка извлечения: %v", err)
		responseMsg.Text = "❌ Ошибка обработки страницы"
		bot.api.Send(responseMsg)
		return
	}

	if bot.conf.Debug {
		status := "структурированный"
		if !articleContent.Success {
			status = "фолбэк"
		}
		log.Printf("Использован %s метод. Заголовок: %s. Содержание: %s",
			status,
			articleContent.Title,
			articleContent.Content,
		)
	}

	var (
		wg      sync.WaitGroup
		results = make(chan string, 3)
		errors  = make(chan error, 3)
	)

	needTitle := !articleContent.Success || articleContent.Title == ""
	if needTitle {
		wg.Add(1)
		go bot.queryTitle(articleContent.Content, &wg, results, errors)
	}

	switch bot.conf.FullAnalysis {
	case true:
		// Полный анализ
		wg.Add(2)
		go bot.queryTheme(articleContent.Content, &wg, results, errors)
		go bot.querySentiment(articleContent.Content, false, &wg, results, errors)
		wg.Wait()

	case false:
		// Краткий анализ
		wg.Add(1)
		go bot.querySentiment(articleContent.Content, true, &wg, results, errors)
		wg.Wait()
	}
	close(results)
	close(errors)

	// Формирование ответа
	var response strings.Builder
	if articleContent.Success && !needTitle {
		response.WriteString(fmt.Sprintf("*Заголовок:* %s\n\n", articleContent.Title))
	}

	for res := range results {
		response.WriteString(res + "\n\n")
	}

	if len(errors) > 0 {
		response.WriteString("\n⚠️ Некоторые части анализа не удались")
	}

	responseMsg.Text = "📋 *Анализ статьи*\n\n" + response.String()
	responseMsg.ParseMode = "Markdown"
	bot.api.Send(responseMsg)
}
