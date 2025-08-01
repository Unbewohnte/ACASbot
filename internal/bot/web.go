package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

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
		http.SetCookie(w, &http.Cookie{
			Name:    "auth",
			Value:   "authenticated",
			Expires: time.Now().Add(24 * time.Hour),
			Path:    "/",
		})
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
}

func (ws *WebServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	cookie, err := r.Cookie("auth")
	if err != nil || cookie.Value != "authenticated" {
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

	// Ищем и вызываем команду
	for _, command := range ws.bot.commands {
		if command.Name == commandName {
			// Вызываем команду и получаем результат
			response, err := command.Call(args)
			if err != nil {
				ws.SendLog("Error executing command: " + err.Error())
				return
			}
			ws.SendAnalysisResult(response)
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
			ws.SendAnalysisResult(response)
		}
	}
}

// SendAnalysisResult sends analysis results to web clients
func (ws *WebServer) SendAnalysisResult(result string) {
	ws.broadcast(WebMessage{
		Type:    "analysis",
		Content: result,
	})
}

// SendLog sends log messages to web clients
func (ws *WebServer) SendLog(log string) {
	ws.broadcast(WebMessage{
		Type:    "log",
		Content: log,
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
