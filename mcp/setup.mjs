#!/usr/bin/env node

import { spawnSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const mcpDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(mcpDir, '..');
const sdkDir = resolve(repoRoot, 'sdk', 'javascript');
const dist = resolve(mcpDir, 'dist', 'index.js');

function usage(exitCode = 0) {
  console.log(`Usage:
  node mcp/setup.mjs --runtime <codex|claude|both>

Options:
  --project-dir <path>  Consuming repository for Claude's local scope (default: cwd)
  --replace             Replace an existing nfc-agent registration
  --dry-run             Print build and registration commands without running them

The helper builds this checkout and derives NFC_AGENT_REPO_PATH automatically.
Optional SimplyPrint and whatt.io credentials remain in your personal environment.`);
  process.exit(exitCode);
}

function parseArgs(argv) {
  const options = {
    runtime: '',
    projectDir: process.cwd(),
    replace: false,
    dryRun: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === '--help' || arg === '-h') usage();
    if (arg === '--runtime') {
      options.runtime = argv[index + 1] || '';
      index += 1;
      continue;
    }
    if (arg === '--project-dir') {
      const value = argv[index + 1];
      if (!value) usage(2);
      options.projectDir = resolve(value);
      index += 1;
      continue;
    }
    if (arg === '--replace') {
      options.replace = true;
      continue;
    }
    if (arg === '--dry-run') {
      options.dryRun = true;
      continue;
    }
    console.error(`setup: unknown option ${arg}`);
    usage(2);
  }
  if (!['codex', 'claude', 'both'].includes(options.runtime)) {
    console.error('setup: --runtime must be codex, claude, or both');
    usage(2);
  }
  return options;
}

function printable(command, args) {
  return [command, ...args].map((part) => (
    /^[A-Za-z0-9_./:@=-]+$/.test(part) ? part : JSON.stringify(part)
  )).join(' ');
}

function run(command, args, { cwd = repoRoot, allowFailure = false, dryRun = false } = {}) {
  console.log(`$ ${printable(command, args)}`);
  if (dryRun) return true;
  const result = spawnSync(command, args, {
    cwd,
    stdio: 'inherit',
    shell: process.platform === 'win32',
  });
  if (result.error) {
    if (allowFailure) return false;
    throw result.error;
  }
  if (result.status !== 0 && !allowFailure) {
    throw new Error(`${command} exited with status ${result.status}`);
  }
  return result.status === 0;
}

function registerCodex(options) {
  if (options.replace) {
    run('codex', ['mcp', 'remove', 'nfc-agent'], {
      allowFailure: true,
      dryRun: options.dryRun,
    });
  }
  run('codex', [
    'mcp', 'add', 'nfc-agent',
    '--env', `NFC_AGENT_REPO_PATH=${repoRoot}`,
    '--', 'node', dist,
  ], { dryRun: options.dryRun });
}

function registerClaude(options) {
  if (options.replace) {
    run('claude', ['mcp', 'remove', '-s', 'local', 'nfc-agent'], {
      cwd: options.projectDir,
      allowFailure: true,
      dryRun: options.dryRun,
    });
  }
  run('claude', [
    'mcp', 'add', 'nfc-agent', '-s', 'local',
    '-e', `NFC_AGENT_REPO_PATH=${repoRoot}`,
    '--', 'node', dist,
  ], {
    cwd: options.projectDir,
    dryRun: options.dryRun,
  });
}

const options = parseArgs(process.argv.slice(2));

run('npm', ['ci', '--silent'], { cwd: sdkDir, dryRun: options.dryRun });
run('npm', ['run', 'build', '--silent'], { cwd: sdkDir, dryRun: options.dryRun });
run('npm', ['ci', '--silent'], { cwd: mcpDir, dryRun: options.dryRun });
run('npm', ['run', 'build', '--silent'], { cwd: mcpDir, dryRun: options.dryRun });

if (options.runtime === 'codex' || options.runtime === 'both') registerCodex(options);
if (options.runtime === 'claude' || options.runtime === 'both') registerClaude(options);

console.log('\nNFC Agent MCP setup complete.');
console.log(`Resolved checkout: ${repoRoot}`);
console.log('Fully restart the agent application and start a new conversation.');
