// TypeDuck backend entry point.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gaboolic/moqi-ime/imecore"
	moqipb "github.com/gaboolic/moqi-ime/proto"

	"github.com/gaboolic/moqi-ime/input_methods/fcitx5"
	"github.com/gaboolic/moqi-ime/input_methods/moqi"
	"github.com/gaboolic/moqi-ime/input_methods/rime"
)

type Client struct {
	ID              string
	GUID            string
	IsWindows8Above bool
	IsMetroApp      bool
	IsUiLess        bool
	IsConsole       bool
	Service         imecore.TextService
}

type ServiceFactory func(client *imecore.Client, guid string) imecore.TextService

type asyncResponseSender interface {
	SetAsyncResponseSender(func(*imecore.Response))
}

// Server handles framed frontend requests and dispatches them to input services.
type Server struct {
	mu        sync.RWMutex
	writeMu   sync.Mutex
	clients   map[string]*Client
	factories map[string]ServiceFactory // guid -> factory
	reader    *bufio.Reader
	running   bool
}

const logRetentionDays = 7

func stringifyData(data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(raw)
}

func logRequestSummary(clientID string, req *imecore.Request) {
	log.Printf(
		"request received client=%s method=%s seq=%d id=%q commandId=%d keyCode=%d charCode=%d repeat=%d scan=%d candidates=%d showCandidates=%t cursor=%d",
		clientID,
		req.Method,
		req.SeqNum,
		req.ID.StringValue(),
		req.ID.IntValue(),
		req.KeyCode,
		req.CharCode,
		req.RepeatCount,
		req.ScanCode,
		len(req.CandidateList),
		req.ShowCandidates,
		req.CursorPos,
	)
}

func logResponseSummary(clientID string, resp *imecore.Response) {
	_, err := json.Marshal(resp)
	if err != nil {
		log.Printf("response marshal failed client=%s err=%v", clientID, err)
		return
	}
	// Keep routine response payloads out of logs because they can contain typed text.
}

func NewServer() *Server {
	return &Server{
		clients:   make(map[string]*Client),
		factories: make(map[string]ServiceFactory),
		reader:    bufio.NewReader(os.Stdin),
	}
}

func (s *Server) RegisterService(guid string, factory ServiceFactory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	guid = strings.ToLower(guid)
	s.factories[guid] = factory
	log.Printf("registered input service: %s", guid)
}

func (s *Server) Run() error {
	s.running = true
	log.Println("TypeDuck backend server started")

	for s.running {
		payload, err := readFrame(s.reader)
		if err != nil {
			if err.Error() == "EOF" {
				log.Println("received EOF; stopping backend server")
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		reqMsg, err := decodeClientRequest(payload)
		if err != nil {
			log.Printf("message handling error: %v", err)
			continue
		}
		clientID := reqMsg.GetClientId()
		if clientID == "" {
			log.Printf("message handling error: missing client_id")
			continue
		}

		if err := s.handleMessage(reqMsg); err != nil {
			log.Printf("message handling error: %v", err)
			_ = s.sendResponse(clientID, &imecore.Response{
				SeqNum:  int(reqMsg.GetSeqNum()),
				Success: false,
				Error:   err.Error(),
			})
		}
	}

	return nil
}

func (s *Server) handleMessage(reqMsg *moqipb.ClientRequest) error {
	clientID := reqMsg.GetClientId()
	req := imecore.ParseProtoRequest(reqMsg)

	// logRequestSummary(clientID, &req)

	resp := s.handleRequest(clientID, req)

	return s.sendResponse(clientID, resp)
}

func (s *Server) handleRequest(clientID string, req *imecore.Request) *imecore.Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch req.Method {
	case "init":
		guid := req.ID.StringValue()
		if guid == "" && req.Data != nil {
			guid, _ = req.Data["guid"].(string)
		}
		guid = strings.ToLower(guid)
		if guid == "" {
			log.Printf("init failed client=%s seq=%d reason=missing_guid id=%q", clientID, req.SeqNum, req.ID.StringValue())
			return &imecore.Response{SeqNum: req.SeqNum, Success: false, Error: "missing guid"}
		}

		client := &Client{
			ID:              clientID,
			GUID:            guid,
			IsWindows8Above: req.IsWindows8Above,
			IsMetroApp:      req.IsMetroApp,
			IsUiLess:        req.IsUiLess,
			IsConsole:       req.IsConsole,
		}

		factory, ok := s.factories[guid]
		if !ok {
			log.Printf("init failed client=%s seq=%d reason=unknown_input_method guid=%s", clientID, req.SeqNum, guid)
			return &imecore.Response{SeqNum: req.SeqNum, Success: false, Error: fmt.Sprintf("unknown input method: %s", guid)}
		}

		moqiClient := &imecore.Client{
			ID:              clientID,
			GUID:            guid,
			IsWindows8Above: req.IsWindows8Above,
			IsMetroApp:      req.IsMetroApp,
			IsUiLess:        req.IsUiLess,
			IsConsole:       req.IsConsole,
		}
		client.Service = factory(moqiClient, guid)
		if sender, ok := client.Service.(asyncResponseSender); ok {
			sender.SetAsyncResponseSender(func(resp *imecore.Response) {
				if resp == nil {
					return
				}
				respCopy := *resp
				respCopy.SeqNum = 0
				s.mu.RLock()
				_, exists := s.clients[clientID]
				s.mu.RUnlock()
				if !exists {
					return
				}
				if err := s.sendResponse(clientID, &respCopy); err != nil {
					log.Printf("async response send failed client=%s err=%v", clientID, err)
				}
			})
		}
		s.clients[clientID] = client

		initStart := time.Now()
		initOK := client.Service.Init(req)
		log.Printf("service init completed client=%s seq=%d guid=%s elapsed=%s success=%t", clientID, req.SeqNum, guid, time.Since(initStart), initOK)
		if !initOK {
			delete(s.clients, clientID)
			log.Printf("init failed client=%s seq=%d guid=%s reason=service_init_false", clientID, req.SeqNum, guid)
			return &imecore.Response{SeqNum: req.SeqNum, Success: false, Error: "service initialization failed"}
		}

		log.Printf("init succeeded client=%s seq=%d guid=%s windows8=%t metro=%t uiless=%t console=%t", clientID, req.SeqNum, guid, req.IsWindows8Above, req.IsMetroApp, req.IsUiLess, req.IsConsole)

		return &imecore.Response{SeqNum: req.SeqNum, Success: true}

	case "close":
		if client, ok := s.clients[clientID]; ok {
			client.Service.Close()
			delete(s.clients, clientID)
			log.Printf("client closed client=%s guid=%s", clientID, client.GUID)
		} else {
			log.Printf("client close requested for unknown session client=%s", clientID)
		}
		return &imecore.Response{SeqNum: req.SeqNum, Success: true}

	case "typeduckSettingsUpdate":
		return rime.ApplyTypeDuckSettingsFromLauncher(req, clientID)

	case "typeduckDeploy":
		return rime.DeployTypeDuckFromLauncher(req)

	case "onActivate", "onDeactivate", "filterKeyDown", "onKeyDown",
		"filterKeyUp", "onKeyUp", "onCommand", "onMenu", "onCompositionTerminated",
		"onPreservedKey", "onLangProfileActivated", "highlightCandidate",
		"selectCandidate", "changePage":
		client, ok := s.clients[clientID]
		if !ok {
			log.Printf("request failed client=%s seq=%d method=%s reason=client_not_initialized", clientID, req.SeqNum, req.Method)
			return &imecore.Response{SeqNum: req.SeqNum, Success: false, Error: "client not initialized"}
		}

		return client.Service.HandleRequest(req)

	default:
		log.Printf("request failed client=%s seq=%d method=%s reason=unknown_method", clientID, req.SeqNum, req.Method)
		return &imecore.Response{SeqNum: req.SeqNum, Success: false, Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}

func (s *Server) sendResponse(clientID string, resp *imecore.Response) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	logResponseSummary(clientID, resp)
	msg, err := imecore.BuildProtoResponse(clientID, resp)
	if err != nil {
		return fmt.Errorf("failed to build protobuf response: %w", err)
	}
	data, err := encodeServerResponse(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize response: %w", err)
	}
	return writeFrame(os.Stdout, data)
}

func loadInputMethods(server *Server) {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("failed to get executable path:", err)
	}
	exeDir := filepath.Dir(exePath)

	inputMethodsDir := filepath.Join(exeDir, "input_methods")
	entries, err := os.ReadDir(inputMethodsDir)
	if err != nil {
		log.Printf("failed to read input_methods directory: %v", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		imePath := filepath.Join(inputMethodsDir, entry.Name(), "ime.json")
		data, err := os.ReadFile(imePath)
		if err != nil {
			log.Printf("failed to read %s: %v", imePath, err)
			continue
		}

		var imeConfig map[string]interface{}
		if err := json.Unmarshal(data, &imeConfig); err != nil {
			log.Printf("failed to parse %s: %v", imePath, err)
			continue
		}

		guid, _ := imeConfig["guid"].(string)
		name, _ := imeConfig["name"].(string)
		guid = strings.ToLower(guid)
		if guid == "" {
			log.Printf("%s is missing guid", entry.Name())
			continue
		}

		log.Printf("loading input method: %s (%s)", name, guid)

		switch entry.Name() {
		case "rime":
			server.RegisterService(guid, func(client *imecore.Client, g string) imecore.TextService {
				return rime.New(client)
			})
		case "moqi":
			server.RegisterService(guid, func(client *imecore.Client, g string) imecore.TextService {
				return moqi.New(client)
			})
		case "fcitx5":
			server.RegisterService(guid, func(client *imecore.Client, g string) imecore.TextService {
				return fcitx5.New(client)
			})
		default:
			server.RegisterService(guid, func(client *imecore.Client, g string) imecore.TextService {
				return moqi.New(client)
			})
		}
	}
}

func openLogFile() (*os.File, error) {
	candidates := []string{}

	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "TypeDuckIME", "Log", "TypeDuckBackend.log"))
	}
	if tempDir := os.TempDir(); tempDir != "" {
		candidates = append(candidates, filepath.Join(tempDir, "TypeDuckIME", "Log", "TypeDuckBackend.log"))
	}
	candidates = append(candidates, "TypeDuckBackend.log")

	var lastErr error
	now := time.Now()
	for _, logPath := range candidates {
		logDir := filepath.Dir(logPath)
		if logDir != "." && logDir != "" {
			if err := os.MkdirAll(logDir, 0755); err != nil {
				lastErr = err
				continue
			}
		}

		cleanupOldDailyLogs(logDir, filepath.Base(logPath), now)
		dailyLogPath := filepath.Join(logDir, dailyLogFileName(filepath.Base(logPath), now))
		logFile, err := os.OpenFile(dailyLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			return logFile, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

func dailyLogFileName(baseName string, now time.Time) string {
	ext := filepath.Ext(baseName)
	name := strings.TrimSuffix(baseName, ext)
	if ext == "" {
		return fmt.Sprintf("%s-%s", baseName, now.Format("2006-01-02"))
	}
	return fmt.Sprintf("%s-%s%s", name, now.Format("2006-01-02"), ext)
}

func cleanupOldDailyLogs(logDir, baseName string, now time.Time) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	ext := filepath.Ext(baseName)
	name := strings.TrimSuffix(baseName, ext)
	prefix := name + "-"
	cutoff := now.AddDate(0, 0, -(logRetentionDays - 1)).Format("2006-01-02")

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !strings.HasPrefix(fileName, prefix) || len(fileName) < len(prefix)+10+len(ext) {
			continue
		}

		datePart := fileName[len(prefix) : len(prefix)+10]
		if len(ext) > 0 && !strings.HasPrefix(fileName[len(prefix)+10:], ext) {
			continue
		}
		if !isDateStamp(datePart) {
			continue
		}

		if datePart < cutoff {
			_ = os.Remove(filepath.Join(logDir, fileName))
		}
	}
}

func isDateStamp(value string) bool {
	if len(value) != 10 {
		return false
	}
	for i, ch := range value {
		switch i {
		case 4, 7:
			if ch != '-' {
				return false
			}
		default:
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func main() {
	logFile, err := openLogFile()
	if err != nil {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Printf("failed to create log file; using stderr: %v", err)
	} else {
		defer logFile.Close()
		log.SetOutput(logFile)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	log.Println("=" + strings.Repeat("=", 50))
	log.Println("TypeDuck backend started")
	log.Println("=" + strings.Repeat("=", 50))

	server := NewServer()

	loadInputMethods(server)

	if err := server.Run(); err != nil {
		log.Fatal("server error:", err)
	}
}
