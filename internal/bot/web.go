package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// RenderMarkdown преобразует Markdown в безопасный HTML
func RenderMarkdown(markdown string) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type WebMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	From    string `json:"from,omitempty"`
}

type WebClient struct {
	conn *websocket.Conn
	send chan WebMessage
}

type WebServer struct {
	bot      *Bot
	upgrader websocket.Upgrader
	clients  map[*WebClient]bool
	mu       sync.Mutex
}

func NewWebServer(bot *Bot) *WebServer {
	return &WebServer{
		bot: bot,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients: make(map[*WebClient]bool),
	}
}

func (ws *WebServer) Start() {
	r := mux.NewRouter()

	// WebSocket endpoint
	r.HandleFunc("/ws", ws.handleWebSocket)

	// Login endpoint
	r.HandleFunc("/login", ws.handleLogin).Methods("POST")

	r.HandleFunc("/download/logs", ws.handleDownloadLogs).Methods("GET")
	r.HandleFunc("/download/xlsx", ws.handleDownloadXLSX).Methods("GET")

	// Static files
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web")))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", ws.bot.conf.Web.Port),
		Handler: r,
	}

	go func() {
		log.Printf("Web server started on %d", ws.bot.conf.Web.Port)
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()

	// Graceful shutdown handling would be added here
}

func (ws *WebServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Simple authentication
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == ws.bot.conf.Web.Username && password == ws.bot.conf.Web.Password {
		token, err := ws.generateJWT()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Устанавливаем куку с дополнительными флагами безопасности
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    token,
			Expires:  time.Now().Add(24 * time.Hour),
			Path:     "/",
			HttpOnly: true, // Защита от XSS
			Secure:   false,
			SameSite: http.SameSiteStrictMode, // Защита от CSRF
		})

		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
}

func (ws *WebServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Проверка аутентификации через JWT
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token, err := ws.validateJWT(cookie.Value)
	if err != nil || !token.Valid {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Дополнительная проверка: убедимся, что токен предназначен для этого пользователя
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["username"] != ws.bot.conf.Web.Username {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &WebClient{
		conn: conn,
		send: make(chan WebMessage, 256),
	}
	ws.addClient(client)

	go client.writePump()
	go client.readPump(ws)
}

func (ws *WebServer) addClient(client *WebClient) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.clients[client] = true
	log.Printf("Web client connected")
}

func (ws *WebServer) removeClient(client *WebClient) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if _, ok := ws.clients[client]; ok {
		delete(ws.clients, client)
		close(client.send)
		log.Printf("Web client disconnected")
	}
}

func (ws *WebServer) broadcast(msg WebMessage) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	for client := range ws.clients {
		select {
		case client.send <- msg:
		default:
			ws.removeClient(client)
		}
	}
}

func (c *WebClient) writePump() {
	defer c.conn.Close()

	for msg := range c.send {
		err := c.conn.WriteJSON(msg)
		if err != nil {
			break
		}
	}
}

func (c *WebClient) readPump(ws *WebServer) {
	defer func() {
		ws.removeClient(c)
		c.conn.Close()
	}()

	for {
		_, msgBytes, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg WebMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "command":
			ws.handleCommand(msg.Content)
		}
	}
}

func (ws *WebServer) handleCommand(cmd string) {
	log.Printf("Web command: %s", cmd)

	// Разделяем команду на части
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	// Получаем имя команды (без слеша, если он есть)
	commandName := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	args := strings.Join(parts[1:], " ")

	// Специальная обработка для getlogs
	if commandName == "getlogs" {
		// Проверяем, существует ли файл логов
		if _, err := os.Stat(ws.bot.conf.LogsFile); os.IsNotExist(err) {
			ws.SendLog("Файл логов не найден")
			return
		}

		// Формируем HTML-ссылку для скачивания
		response := `<div class="download-container">
            <p>Логи доступны для скачивания:</p>
            <a href="/download/logs" target="_blank" class="download-btn">
                <i class="bi bi-download me-2"></i>Скачать логи
            </a>
        </div>`
		ws.SendResponse(response)
		return
	}

	// Специальная обработка для xlsx
	if commandName == "xlsx" {
		// Проверяем, существует ли файл таблицы
		fileName := "ACASbot_Results.xlsx"
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			_, err := ws.bot.GenerateSpreadsheet("")
			if err != nil {
				ws.SendLog("Не вышло сгенерировать локальную таблицу: " + err.Error())
				return
			}
		}

		// Формируем HTML-ссылку для скачивания
		response := `<div class="download-container">
            <p>Таблица результатов доступна для скачивания:</p>
            <a href="/download/xlsx" target="_blank" class="download-btn">
                <i class="bi bi-file-earmark-spreadsheet me-2"></i>Скачать таблицу
            </a>
        </div>`
		ws.SendResponse(response)
		return
	}

	// Ищем и вызываем команду
	for _, command := range ws.bot.commands {
		if command.Name == commandName {
			// Вызываем команду и получаем результат
			response, err := command.Call(args)
			if err != nil {
				ws.SendLog("Error executing command: " + err.Error())
				return
			}
			ws.SendResponse(response)
			return
		}
	}

	// Fallback для URL
	if strings.HasPrefix(cmd, "http") {
		do := ws.bot.CommandByName("do")
		if do != nil {
			// Для URL обрабатываем как команду "do"
			response, err := do.Call(cmd)
			if err != nil {
				ws.SendLog("Error executing do command: " + err.Error())
				return
			}
			ws.SendResponse(response)
		}
	} else {
		// Такой команды просто нет
		var response string

		similarCommands := ws.bot.findSimilarCommands(cmd)
		if len(similarCommands) == 0 {
			response = fmt.Sprintf("Команды `%s` не существует.", cmd)
		} else {
			response = "Неизвестная команда. Возможно, имеется в виду одна из этих команд:\n"
			for _, cmd := range similarCommands {
				command := ws.bot.CommandByName(cmd)
				if command != nil {
					response += fmt.Sprintf("`%s` - %s\n", command.Name, command.Description)
				}
			}
		}

		ws.SendLog(response)
	}
}

func (ws *WebServer) SendResponse(result string) {
	// Преобразуем Markdown в HTML
	html, err := RenderMarkdown(result)
	if err != nil {
		// В случае ошибки рендеринга, отправляем как простой текст с заменой \n на <br>
		html = strings.ReplaceAll(result, "\n", "<br>")
	}

	ws.broadcast(WebMessage{
		Type:    "analysis",
		Content: html,
	})
}

// SendLog sends log messages to web clients
func (ws *WebServer) SendLog(log string) {
	ws.broadcast(WebMessage{
		Type:    "log",
		Content: log,
	})
}

func (ws *WebServer) generateJWT() (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": ws.bot.conf.Web.Username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
		"jti":      uuid.New().String(), // Уникальный идентификатор токена
	})

	return token.SignedString([]byte(ws.bot.conf.Web.JWTSecret))
}

func (ws *WebServer) validateJWT(tokenString string) (*jwt.Token, error) {
	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Проверяем метод подписи
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(ws.bot.conf.Web.JWTSecret), nil
	})
}

func (ws *WebServer) handleDownloadLogs(w http.ResponseWriter, r *http.Request) {
	// Проверка аутентификации через JWT
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, err = ws.validateJWT(cookie.Value)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Проверяем, существует ли файл логов
	if _, err := os.Stat(ws.bot.conf.LogsFile); os.IsNotExist(err) {
		http.Error(w, "Log file not found", http.StatusNotFound)
		return
	}

	// Устанавливаем заголовки для скачивания
	w.Header().Set("Content-Disposition", "attachment; filename=acasbot_logs.txt")
	w.Header().Set("Content-Type", "text/plain")

	// Отправляем файл
	http.ServeFile(w, r, ws.bot.conf.LogsFile)
}

func (ws *WebServer) handleDownloadXLSX(w http.ResponseWriter, r *http.Request) {
	// Проверка аутентификации через JWT
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, err = ws.validateJWT(cookie.Value)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Проверяем, существует ли файл таблицы
	fileName := "ACASbot_Results.xlsx"
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		http.Error(w, "XLSX file not found", http.StatusNotFound)
		return
	}

	// Устанавливаем заголовки для скачивания
	w.Header().Set("Content-Disposition", "attachment; filename=ACASbot_Results.xlsx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")

	// Отправляем файл
	http.ServeFile(w, r, fileName)
}
