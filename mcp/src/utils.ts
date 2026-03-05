import { NFCAgentWebSocket } from '@simplyprint/nfc-agent';

/** Convert http(s) agent URL to ws(s)://host:port/v1/ws */
export function toWsUrl(agentUrl: string): string {
  return agentUrl
    .replace(/^https/, 'wss')
    .replace(/^http/, 'ws')
    .replace(/\/?$/, '/v1/ws');
}

/** Run an operation with a short-lived WebSocket connection */
export async function withWs<T>(
  agentUrl: string,
  fn: (ws: NFCAgentWebSocket) => Promise<T>,
  timeoutMs = 10000
): Promise<T> {
  const ws = new NFCAgentWebSocket({
    url: toWsUrl(agentUrl),
    autoReconnect: false,
    timeout: timeoutMs,
  });
  await ws.connect();
  try {
    return await fn(ws);
  } finally {
    ws.disconnect();
  }
}
