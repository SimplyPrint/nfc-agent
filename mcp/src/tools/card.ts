import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { NFCAgentClient } from '@simplyprint/nfc-agent';
import type { NDEFRecord } from '@simplyprint/nfc-agent';
import { withWs } from '../utils.js';

export function registerCardTools(
  server: McpServer,
  client: NFCAgentClient,
  agentUrl: string
): void {
  server.registerTool(
    'nfc_read_card',
    {
      title: 'Read NFC Card',
      description:
        'Read everything from the card on a reader in one call: UID, card type, protocol, NDEF data (parsed text/URL/JSON), ' +
        'and the full raw memory dump (pages for NTAG/Ultralight, blocks + failedBlocks for MIFARE Classic). ' +
        'This is the primary tool to use when you want to know what is on a card.',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        refresh: z.boolean().default(false).describe('Force bypass cache and re-read card'),
      }),
    },
    async ({ reader, refresh }) => {
      const url = `${agentUrl}/v1/readers/${reader}/read${refresh ? '?refresh=true' : ''}`;
      const res = await fetch(url);
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as Record<string, unknown>;
        throw new Error(typeof err['error'] === 'string' ? err['error'] : `HTTP ${res.status}`);
      }
      const card = await res.json();
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(card, null, 2) }],
      };
    }
  );

  server.registerTool(
    'nfc_write_card',
    {
      title: 'Write NFC Card',
      description: 'Write NDEF data to the card on a reader',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        data: z.string().describe('Data to write'),
        dataType: z.enum(['text', 'json', 'url', 'binary']).default('text').describe('NDEF data type'),
        url: z.string().optional().describe('URL to write (use dataType=url)'),
      }),
    },
    async ({ reader, data, dataType, url }) => {
      await client.writeCard(reader, { data, dataType, url });
      return {
        content: [{ type: 'text' as const, text: `Successfully wrote ${dataType} data to card on reader ${reader}` }],
      };
    }
  );

  server.registerTool(
    'nfc_write_records',
    {
      title: 'Write NDEF Records',
      description: 'Write multiple NDEF records to the card on a reader',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        records: z.array(
          z.object({
            type: z.string().describe('NDEF record type (e.g. "T" for text, "U" for URI)'),
            data: z.string().describe('Record payload'),
            mimeType: z.string().optional().describe('MIME type for custom records'),
          })
        ).describe('NDEF records to write'),
      }),
    },
    async ({ reader, records }) => {
      await withWs(agentUrl, (ws) => ws.writeRecords(reader, records as NDEFRecord[]));
      return {
        content: [{ type: 'text' as const, text: `Wrote ${records.length} NDEF record(s) to card on reader ${reader}` }],
      };
    }
  );

  server.registerTool(
    'nfc_erase_card',
    {
      title: 'Erase NFC Card',
      description: 'Erase all NDEF data from the card on a reader',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
      }),
    },
    async ({ reader }) => {
      await withWs(agentUrl, (ws) => ws.eraseCard(reader));
      return {
        content: [{ type: 'text' as const, text: `Card on reader ${reader} erased` }],
      };
    }
  );

  server.registerTool(
    'nfc_lock_card',
    {
      title: 'Lock NFC Card',
      description: 'Permanently lock an NFC card — this CANNOT be undone',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        confirm: z.boolean().describe('Must be true to confirm the irreversible lock'),
      }),
      annotations: { destructiveHint: true },
    },
    async ({ reader, confirm }) => {
      if (!confirm) {
        return {
          content: [{ type: 'text' as const, text: 'Set confirm: true to permanently lock the card. This cannot be undone.' }],
        };
      }
      await withWs(agentUrl, (ws) => ws.lockCard(reader));
      return {
        content: [{ type: 'text' as const, text: `Card on reader ${reader} has been permanently locked` }],
      };
    }
  );

  server.registerTool(
    'nfc_set_password',
    {
      title: 'Set Card Password',
      description: 'Set a password on an NTAG/Ultralight card (password auth protects pages from startPage onward)',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        password: z.string().length(8).describe('4-byte password as 8 hex chars (e.g. "AABBCCDD")'),
        pack: z.string().length(4).describe('2-byte PACK as 4 hex chars (e.g. "EEFF")'),
        startPage: z.number().int().min(0).describe('First page to protect with password'),
      }),
    },
    async ({ reader, password, pack, startPage }) => {
      const res = await fetch(`${agentUrl}/v1/readers/${reader}/password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password, pack, startPage }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as Record<string, unknown>;
        throw new Error(typeof err['error'] === 'string' ? err['error'] : `HTTP ${res.status}`);
      }
      return {
        content: [{ type: 'text' as const, text: `Password set on card on reader ${reader}` }],
      };
    }
  );

  server.registerTool(
    'nfc_remove_password',
    {
      title: 'Remove Card Password',
      description: 'Remove the password from an NTAG/Ultralight card',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        password: z.string().length(8).describe('Current 4-byte password as 8 hex chars'),
      }),
    },
    async ({ reader, password }) => {
      const res = await fetch(`${agentUrl}/v1/readers/${reader}/password`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as Record<string, unknown>;
        throw new Error(typeof err['error'] === 'string' ? err['error'] : `HTTP ${res.status}`);
      }
      return {
        content: [{ type: 'text' as const, text: `Password removed from card on reader ${reader}` }],
      };
    }
  );

  server.registerTool(
    'nfc_poll_card',
    {
      title: 'Wait for NFC Card',
      description: 'Wait for an NFC card to be presented to a reader (polls until card detected or timeout)',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        timeoutMs: z.number().int().min(1000).max(60000).default(10000).describe('Timeout in milliseconds (1s–60s)'),
        intervalMs: z.number().int().min(200).max(5000).default(500).describe('Poll interval in milliseconds'),
      }),
    },
    async ({ reader, timeoutMs, intervalMs }) => {
      const card = await new Promise<import('@simplyprint/nfc-agent').Card | null>((resolve) => {
        const poller = client.pollCard(reader, { interval: intervalMs });
        const timer = setTimeout(() => { poller.stop(); resolve(null); }, timeoutMs);
        poller.on('card', (c) => { clearTimeout(timer); poller.stop(); resolve(c); });
        poller.start();
      });
      return {
        content: [{
          type: 'text' as const,
          text: card
            ? JSON.stringify(card, null, 2)
            : `No card detected on reader ${reader} within ${timeoutMs}ms`,
        }],
      };
    }
  );
}
