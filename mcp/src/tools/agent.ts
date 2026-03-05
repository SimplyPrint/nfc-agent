import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { NFCAgentClient } from '@simplyprint/nfc-agent';

export function registerAgentTools(server: McpServer, client: NFCAgentClient, agentUrl: string): void {
  server.registerTool(
    'nfc_agent_status',
    {
      title: 'NFC Agent Status',
      description: 'Check if the NFC Agent is running and get version information',
    },
    async () => {
      try {
        const version = await client.getVersion();
        return {
          content: [
            {
              type: 'text' as const,
              text: JSON.stringify({ connected: true, ...version }, null, 2),
            },
          ],
        };
      } catch {
        return {
          content: [
            {
              type: 'text' as const,
              text: JSON.stringify({ connected: false, error: 'Agent not reachable at configured URL' }),
            },
          ],
        };
      }
    }
  );

  server.registerTool(
    'nfc_agent_stop',
    {
      title: 'Stop NFC Agent',
      description: 'Shutdown the NFC Agent daemon',
      inputSchema: z.object({
        confirm: z.boolean().describe('Must be true to confirm shutdown'),
      }),
    },
    async ({ confirm }) => {
      if (!confirm) {
        return {
          content: [{ type: 'text' as const, text: 'Set confirm: true to shutdown the agent' }],
        };
      }
      const response = await fetch(`${agentUrl}/v1/shutdown`, {
        method: 'POST',
      });
      return {
        content: [
          {
            type: 'text' as const,
            text: response.ok ? 'Agent shutdown initiated' : `Shutdown failed: HTTP ${response.status}`,
          },
        ],
      };
    }
  );

  server.registerTool(
    'nfc_list_readers',
    {
      title: 'List NFC Readers',
      description: 'List all connected NFC readers',
    },
    async () => {
      const readers = await client.getReaders();
      return {
        content: [
          {
            type: 'text' as const,
            text: readers.length === 0
              ? 'No NFC readers connected'
              : JSON.stringify(readers, null, 2),
          },
        ],
      };
    }
  );

  server.registerTool(
    'nfc_list_supported_readers',
    {
      title: 'List Supported NFC Readers',
      description: 'List all NFC reader hardware models supported by the agent',
    },
    async () => {
      const result = await client.getSupportedReaders();
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }],
      };
    }
  );
}
