import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { NFCAgentClient } from '@simplyprint/nfc-agent';
import { withWs } from '../utils.js';

export function registerAdvancedTools(server: McpServer, client: NFCAgentClient, agentUrl: string): void {
  // MIFARE Classic

  server.registerTool(
    'nfc_read_mifare_block',
    {
      title: 'Read MIFARE Block',
      description: 'Read a 16-byte block from a MIFARE Classic card',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        block: z.number().int().min(0).describe('Block number to read'),
        key: z.string().length(12).optional().describe('6-byte auth key as 12 hex chars (default: FFFFFFFFFFFF)'),
        keyType: z.enum(['A', 'B']).default('A').describe('Key type A or B'),
      }),
    },
    async ({ reader, block, key, keyType }) => {
      const data = await client.readMifareBlock(reader, block, { key, keyType });
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(data, null, 2) }],
      };
    }
  );

  server.registerTool(
    'nfc_write_mifare_block',
    {
      title: 'Write MIFARE Block',
      description: 'Write 16 bytes to a block on a MIFARE Classic card',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        block: z.number().int().min(0).describe('Block number to write'),
        data: z.string().length(32).describe('16 bytes as 32 hex chars'),
        key: z.string().length(12).optional().describe('6-byte auth key as 12 hex chars'),
        keyType: z.enum(['A', 'B']).default('A').describe('Key type A or B'),
      }),
    },
    async ({ reader, block, data, key, keyType }) => {
      await client.writeMifareBlock(reader, block, { data, key, keyType });
      return {
        content: [{ type: 'text' as const, text: `Block ${block} written on reader ${reader}` }],
      };
    }
  );

  server.registerTool(
    'nfc_write_mifare_blocks',
    {
      title: 'Write MIFARE Blocks (Batch)',
      description: 'Write multiple blocks to a MIFARE Classic card in a single operation',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        blocks: z.array(
          z.object({
            block: z.number().int().min(0).describe('Block number'),
            data: z.string().length(32).describe('16 bytes as 32 hex chars'),
          })
        ).min(1).describe('Blocks to write'),
        key: z.string().length(12).optional().describe('6-byte auth key as 12 hex chars'),
        keyType: z.enum(['A', 'B']).default('A').describe('Key type A or B'),
      }),
    },
    async ({ reader, blocks, key, keyType }) => {
      const result = await client.writeMifareBlocks(reader, { blocks, key, keyType });
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }],
      };
    }
  );

  server.registerTool(
    'nfc_derive_uid_key',
    {
      title: 'Derive MIFARE Key from UID',
      description: 'Derive a MIFARE Classic key from the card UID using AES-128-ECB (used by some manufacturers)',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        aesKey: z.string().length(32).describe('16-byte master AES key as 32 hex chars'),
      }),
    },
    async ({ reader, aesKey }) => {
      const result = await client.deriveUIDKeyAES(reader, { aesKey });
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }],
      };
    }
  );

  // MIFARE Ultralight / NTAG

  server.registerTool(
    'nfc_read_ultralight_page',
    {
      title: 'Read Ultralight/NTAG Page',
      description: 'Read a 4-byte page from a MIFARE Ultralight or NTAG card',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        page: z.number().int().min(0).describe('Page number to read'),
        password: z.string().length(8).optional().describe('4-byte password as 8 hex chars (if password-protected)'),
      }),
    },
    async ({ reader, page, password }) => {
      const data = await client.readUltralightPage(reader, page, { password });
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(data, null, 2) }],
      };
    }
  );

  server.registerTool(
    'nfc_write_ultralight_page',
    {
      title: 'Write Ultralight/NTAG Page',
      description: 'Write 4 bytes to a page on a MIFARE Ultralight or NTAG card',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        page: z.number().int().min(0).describe('Page number to write'),
        data: z.string().length(8).describe('4 bytes as 8 hex chars'),
        password: z.string().length(8).optional().describe('4-byte password as 8 hex chars (if password-protected)'),
      }),
    },
    async ({ reader, page, data, password }) => {
      await client.writeUltralightPage(reader, page, { data, password });
      return {
        content: [{ type: 'text' as const, text: `Page ${page} written on reader ${reader}` }],
      };
    }
  );

  server.registerTool(
    'nfc_write_ultralight_pages',
    {
      title: 'Write Ultralight/NTAG Pages (Batch)',
      description: 'Write multiple pages to a MIFARE Ultralight or NTAG card in a single operation',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        pages: z.array(
          z.object({
            page: z.number().int().min(0).describe('Page number'),
            data: z.string().length(8).describe('4 bytes as 8 hex chars'),
          })
        ).min(1).describe('Pages to write'),
        password: z.string().length(8).optional().describe('4-byte password as 8 hex chars (if password-protected)'),
      }),
    },
    async ({ reader, pages, password }) => {
      const result = await client.writeUltralightPages(reader, { pages, password });
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }],
      };
    }
  );

  // Raw memory dump

  server.registerTool(
    'nfc_dump_card',
    {
      title: 'Dump Card Memory',
      description:
        'Read the full raw memory of the card on a reader. ' +
        'NTAG/Ultralight: returns all pages (4 bytes each). ' +
        'MIFARE Classic: returns all readable blocks (16 bytes each) — keys tried automatically (default, Creality UID-derived, NFC Forum, etc). ' +
        'failedBlocks lists any sectors that could not be read due to unknown keys.',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
      }),
    },
    async ({ reader }) => {
      // Use 30s timeout — MIFARE Classic full dump with key trials takes longer than the default 10s.
      const dump = await withWs(agentUrl, (ws) => ws.dumpCard(reader), 30000);
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(dump, null, 2) }],
      };
    }
  );

  // HTTP dump (alternative — no WS needed)
  server.registerTool(
    'nfc_dump_card_http',
    {
      title: 'Dump Card Memory (HTTP)',
      description: 'Read the full raw memory of the card via HTTP GET — same as nfc_dump_card but uses the REST endpoint directly.',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
      }),
    },
    async ({ reader }) => {
      const res = await fetch(`${agentUrl}/v1/readers/${reader}/dump`);
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as Record<string, unknown>;
        throw new Error(typeof err['error'] === 'string' ? err['error'] : `HTTP ${res.status}`);
      }
      const dump = await res.json();
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(dump, null, 2) }],
      };
    }
  );
}
