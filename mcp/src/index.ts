#!/usr/bin/env node
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { NFCAgentClient } from '@simplyprint/nfc-agent';

import { loadConfig } from './config.js';
import { SimplyPrintClient } from './clients/simplyprint.js';
import { WhattioClient } from './clients/whattio.js';
import { registerAgentTools } from './tools/agent.js';
import { registerCardTools } from './tools/card.js';
import { registerAdvancedTools } from './tools/advanced.js';
import { registerDevTools } from './tools/dev.js';
import { registerSimplyPrintTools } from './tools/simplyprint.js';
import { registerWhattioTools } from './tools/whattio.js';

async function main() {
  const config = loadConfig();

  const server = new McpServer(
    {
      name: 'nfc-agent-mcp',
      version: '0.1.0',
    },
    {
      capabilities: { tools: {} },
    }
  );

  // NFC Agent client (always available)
  const nfc = new NFCAgentClient({ baseUrl: config.agentUrl });

  // Core tools — always registered
  registerAgentTools(server, nfc, config.agentUrl);
  registerCardTools(server, nfc, config.agentUrl);
  registerAdvancedTools(server, nfc, config.agentUrl);

  // Dev tools — only when NFC_AGENT_REPO_PATH is set
  if (config.hasDevTools) {
    registerDevTools(server, config.repoPath!, config.agentUrl);
  }

  // SimplyPrint tools — only when SIMPLYPRINT_API_KEY is set
  if (config.hasSimplyPrint) {
    const sp = new SimplyPrintClient(config.simplyPrintBaseUrl, config.simplyPrintApiKey!);
    registerSimplyPrintTools(server, sp, nfc, config.agentUrl);
  }

  // whatt.io tools — only when WHATTIO_TOKEN is set
  if (config.hasWhattio) {
    const whattio = new WhattioClient(config.whattioToken!, config.whattioTeamId);
    // Set team if configured
    await whattio.setTeam().catch(() => {
      // Non-fatal — team switching may fail if token is for a specific team already
    });
    registerWhattioTools(server, whattio, nfc);
  }

  const transport = new StdioServerTransport();
  await server.connect(transport);

  // Log active integrations to stderr (not stdout, which is reserved for MCP protocol)
  const active = ['NFC Agent'];
  if (config.hasDevTools) active.push('Dev Tools');
  if (config.hasSimplyPrint) active.push('SimplyPrint');
  if (config.hasWhattio) active.push('whatt.io');
  process.stderr.write(`[nfc-agent-mcp] Started — ${active.join(', ')}\n`);
}

main().catch((err) => {
  process.stderr.write(`[nfc-agent-mcp] Fatal error: ${err}\n`);
  process.exit(1);
});
