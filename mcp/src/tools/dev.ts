import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { spawn } from 'child_process';
import * as path from 'path';

function runCommand(
  cmd: string,
  args: string[],
  cwd: string
): Promise<{ stdout: string; stderr: string; exitCode: number }> {
  return new Promise((resolve) => {
    const proc = spawn(cmd, args, { cwd, shell: false });
    const stdout: string[] = [];
    const stderr: string[] = [];

    proc.stdout.on('data', (d: Buffer) => stdout.push(d.toString()));
    proc.stderr.on('data', (d: Buffer) => stderr.push(d.toString()));

    proc.on('close', (code) => {
      resolve({
        stdout: stdout.join(''),
        stderr: stderr.join(''),
        exitCode: code ?? 1,
      });
    });
  });
}

export function registerDevTools(server: McpServer, repoPath: string, agentUrl: string): void {
  server.registerTool(
    'nfc_agent_build',
    {
      title: 'Build NFC Agent',
      description: 'Build the NFC Agent binary from source using `go build`',
      inputSchema: z.object({
        outputPath: z.string().optional().describe('Output binary path (default: ./nfc-agent)'),
      }),
    },
    async ({ outputPath }) => {
      const args = ['build'];
      if (outputPath) {
        const resolved = path.resolve(repoPath, outputPath);
        if (!resolved.startsWith(path.resolve(repoPath) + path.sep) && resolved !== path.resolve(repoPath)) {
          throw new Error(`outputPath must be within the repo directory (got: ${outputPath})`);
        }
        args.push('-o', resolved);
      }
      args.push('./cmd/nfc-agent/...');
      const result = await runCommand('go', args, repoPath);
      const output = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
      return {
        content: [
          {
            type: 'text' as const,
            text: result.exitCode === 0
              ? `Build succeeded\n${output}`
              : `Build failed (exit ${result.exitCode})\n${output}`,
          },
        ],
      };
    }
  );

  server.registerTool(
    'nfc_agent_test',
    {
      title: 'Run NFC Agent Tests',
      description: 'Run Go tests for the NFC Agent source',
      inputSchema: z.object({
        pkg: z.string().default('./...').describe('Package pattern (e.g. "./..." or "./internal/core/...")'),
        run: z.string().optional().describe('Test name pattern to filter (passed to -run)'),
        verbose: z.boolean().default(false).describe('Enable verbose output (-v)'),
      }),
    },
    async ({ pkg, run, verbose }) => {
      const args = ['test', '-race', pkg];
      if (verbose) args.push('-v');
      if (run) args.push('-run', run);
      const result = await runCommand('go', args, repoPath);
      const output = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
      return {
        content: [
          {
            type: 'text' as const,
            text: result.exitCode === 0
              ? `Tests passed\n${output}`
              : `Tests failed (exit ${result.exitCode})\n${output}`,
          },
        ],
      };
    }
  );

  server.registerTool(
    'nfc_agent_start',
    {
      title: 'Start NFC Agent',
      description: 'Start the NFC Agent from source (go run) in the background — returns the process PID',
      inputSchema: z.object({
        noTray: z.boolean().default(false).describe('Run headless without system tray'),
        port: z.number().int().optional().describe('Override port (default: 32145)'),
      }),
    },
    async ({ noTray, port }) => {
      const effectivePort = port ?? 32145;
      const agentHttpUrl = `http://127.0.0.1:${effectivePort}`;
      const env: Record<string, string> = { ...process.env as Record<string, string> };
      if (port) env['NFC_AGENT_PORT'] = String(port);

      // go run does not accept '...' patterns — use the package path directly
      const goArgs = ['run', './cmd/nfc-agent'];
      if (noTray) goArgs.push('--no-tray');

      const proc = spawn('go', goArgs, {
        cwd: repoPath,
        detached: true,
        stdio: 'ignore',
        env,
      });
      proc.unref();

      // Poll the health endpoint until the agent is ready (up to 45s for compilation)
      const deadline = Date.now() + 45_000;
      let ready = false;
      while (Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, 1500));
        try {
          const res = await fetch(`${agentHttpUrl}/v1/health`, { signal: AbortSignal.timeout(1000) });
          if (res.ok) { ready = true; break; }
        } catch {
          // still starting up
        }
      }

      return {
        content: [
          {
            type: 'text' as const,
            text: ready
              ? `NFC Agent started and ready (PID ${proc.pid}, port ${effectivePort})`
              : `go run launched (PID ${proc.pid}) but agent did not respond within 45s — check logs with nfc_agent_logs`,
          },
        ],
      };
    }
  );

  server.registerTool(
    'nfc_agent_logs',
    {
      title: 'Get NFC Agent Logs',
      description: 'Fetch recent logs from the running NFC Agent',
      inputSchema: z.object({
        limit: z.number().int().min(1).max(500).default(50).describe('Number of log entries to fetch'),
        level: z.enum(['debug', 'info', 'warn', 'error']).optional().describe('Filter by log level'),
        category: z.string().optional().describe('Filter by log category'),
      }),
    },
    async ({ limit, level, category }) => {
      const params = new URLSearchParams({ limit: String(limit) });
      if (level) params.set('level', level);
      if (category) params.set('category', category);

      const res = await fetch(`${agentUrl}/v1/logs?${params}`);
      if (!res.ok) {
        throw new Error(`Failed to fetch logs: HTTP ${res.status}`);
      }
      const data = await res.json();
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(data, null, 2) }],
      };
    }
  );
}
