#!/usr/bin/env node
/**
 * Test script for raw card dump via JavaScript SDK.
 *
 * Tests:
 *   1. HTTP: GET /v1/readers/0/dump (fetch)
 *   2. WS: dumpCard() command
 *   3. WS: subscribe({ includeRaw: true }) + card_data event (optional, tap a card)
 *   4. Creality CFS decode from MIFARE Classic raw blocks
 *
 * Usage:
 *   cd /path/to/nfc-agent
 *   npm run build --prefix sdk/javascript   # build SDK first
 *   node scripts/test_dump.mjs
 *   node scripts/test_dump.mjs --test subscribe
 */

import { createRequire } from 'module';

// Load the local SDK build directly
const require = createRequire(import.meta.url);
let NFCAgentWebSocket;

try {
  const sdk = await import('../sdk/javascript/dist/index.js');
  NFCAgentWebSocket = sdk.NFCAgentWebSocket;
} catch (err) {
  console.error('ERROR: Could not load JS SDK. Run: npm run build --prefix sdk/javascript');
  console.error(err.message);
  process.exit(1);
}

// Node.js 18+ has global fetch; polyfill for older versions
if (typeof fetch === 'undefined') {
  console.error('ERROR: fetch not available. Use Node.js 18+');
  process.exit(1);
}

const BASE_URL = 'http://127.0.0.1:32145/v1';

// Creality CFS AES key for data blocks (from CrealityMaterialStandard.php)
const CREALITY_AES_KEY_DATA = Buffer.from('H@CFkRnz@KAtBJp2');

const CREALITY_FILAMENT_NAMES = {
  '010101': 'PLA',
  '010201': 'PETG',
  '010301': 'ABS',
  '010401': 'TPU',
  '010501': 'ASA',
  '010601': 'PC',
  '010701': 'PA',
  '010801': 'PLA-CF',
  '010901': 'PETG-CF',
  '010A01': 'ABS-CF',
  '010B01': 'PA-CF',
  '010C01': 'PLA-Silk',
  '010D01': 'PLA-Matte',
};

const LENGTH_CODE_TO_WEIGHT = {
  '0330': '1000g',
  '0247': '750g',
  '0198': '600g',
  '0165': '500g',
  '0082': '250g',
};

// ============================================================================
// Creality decode
// ============================================================================

function decryptCrealityBlock(hexData) {
  const { createDecipheriv } = require('crypto');
  const enc = Buffer.from(hexData, 'hex');
  const decipher = createDecipheriv('aes-128-ecb', CREALITY_AES_KEY_DATA, null);
  decipher.setAutoPadding(false);
  return Buffer.concat([decipher.update(enc), decipher.final()]).toString('ascii');
}

function tryDecodeCreality(blocks) {
  const blockNums = [4, 5, 6];
  for (const n of blockNums) {
    if (!blocks[n]) {
      console.log(`  Block ${n} not available — Creality decode skipped`);
      return;
    }
  }

  let decrypted = '';
  for (const n of blockNums) {
    const dec = decryptCrealityBlock(blocks[n]);
    console.log(`  Block ${n} decrypted: ${JSON.stringify(dec)}`);
    decrypted += dec;
  }

  console.log(`\n  Full payload (48 chars): ${JSON.stringify(decrypted)}`);

  if (decrypted.length < 34) {
    console.log(`  Payload too short: ${decrypted.length} chars`);
    return;
  }

  const payload = decrypted.padEnd(48, '0').slice(0, 48);
  const filamentId = payload.slice(12, 18).toUpperCase();
  const colorHex = payload.slice(18, 24).toUpperCase();
  const lengthCode = payload.slice(24, 28);

  console.log('\n  Parsed Creality fields:');
  console.log(`    Date Code:     ${payload.slice(0, 6)}`);
  console.log(`    Vendor ID:     ${payload.slice(6, 10)}`);
  console.log(`    Batch Code:    ${payload.slice(10, 12)}`);
  console.log(`    Filament ID:   ${filamentId} (${CREALITY_FILAMENT_NAMES[filamentId] ?? 'Unknown'})`);
  console.log(`    Color (hex):   #${colorHex}`);
  console.log(`    Length Code:   ${lengthCode} (${LENGTH_CODE_TO_WEIGHT[lengthCode] ?? 'Unknown'})`);
  console.log(`    Serial Num:    ${payload.slice(28, 34)}`);
}

// ============================================================================
// Dump printer
// ============================================================================

function printDump(dump) {
  console.log(`  UID:  ${dump.uid ?? 'N/A'}`);
  console.log(`  Type: ${dump.type ?? 'N/A'}`);

  if (dump.pages?.length) {
    console.log(`  Pages (${dump.pages.length} total):`);
    dump.pages.slice(0, 8).forEach((page, i) => {
      console.log(`    Page ${String(i).padStart(3)}: ${page}`);
    });
    if (dump.pages.length > 8) {
      console.log(`    ... and ${dump.pages.length - 8} more pages`);
    }
  }

  if (dump.blocks && Object.keys(dump.blocks).length > 0) {
    const sortedKeys = Object.keys(dump.blocks).sort((a, b) => parseInt(a) - parseInt(b));
    console.log(`  Blocks (${sortedKeys.length} readable):`);
    sortedKeys.slice(0, 8).forEach((k) => {
      console.log(`    Block ${String(k).padStart(3)}: ${dump.blocks[k]}`);
    });
    if (sortedKeys.length > 8) {
      console.log(`    ... and ${sortedKeys.length - 8} more blocks`);
    }
    if (dump.failedBlocks?.length) {
      console.log(`  Failed blocks (unknown keys): ${dump.failedBlocks.join(', ')}`);
    }

    // Try Creality decode
    if (dump.blocks['4'] || dump.blocks['5'] || dump.blocks['6']) {
      console.log('\n  Attempting Creality CFS decode...');
      tryDecodeCreality(dump.blocks);
    }
  }
}

// ============================================================================
// Test 1: HTTP dump
// ============================================================================

async function testHttpDump(readerIndex = 0) {
  console.log('\n[1] HTTP GET /v1/readers/0/dump');
  console.log('-'.repeat(50));

  try {
    const resp = await fetch(`${BASE_URL}/readers/${readerIndex}/dump`);
    if (!resp.ok) {
      const body = await resp.text();
      console.log(`  HTTP dump failed: ${resp.status} ${body.slice(0, 200)}`);
      return false;
    }
    const dump = await resp.json();
    console.log('  HTTP dump response received:');
    printDump(dump);
    return true;
  } catch (err) {
    console.log(`  ERROR: ${err.message}`);
    return false;
  }
}

// ============================================================================
// Test 2: WS dumpCard
// ============================================================================

async function testWsDump(readerIndex = 0) {
  console.log('\n[2] WebSocket dumpCard() command');
  console.log('-'.repeat(50));

  const ws = new NFCAgentWebSocket({ autoReconnect: false });
  try {
    await ws.connect();
    console.log('  Connected to NFC Agent WS');

    const dump = await Promise.race([
      ws.dumpCard(readerIndex),
      new Promise((_, reject) => setTimeout(() => reject(new Error('Timeout')), 15000)),
    ]);

    console.log('  dumpCard() response received:');
    printDump(dump);
    return true;
  } catch (err) {
    console.log(`  dumpCard failed: ${err.message}`);
    return false;
  } finally {
    ws.disconnect();
  }
}

// ============================================================================
// Test 3: WS subscribe with includeRaw
// ============================================================================

async function testWsSubscribeRaw(readerIndex = 0) {
  console.log('\n[3] WebSocket subscribe({ includeRaw: true })');
  console.log('-'.repeat(50));
  console.log('  Tap a card on the reader to see card_detected + card_data events...');
  console.log('  (waiting up to 30 seconds, Ctrl+C to skip)');

  const ws = new NFCAgentWebSocket({ autoReconnect: false });

  return new Promise(async (resolve) => {
    let timer;

    ws.on('card_detected', (event) => {
      console.log(`\n  >> card_detected: UID=${event.card.uid}, Type=${event.card.type}`);
      console.log(`     (waiting for card_data event...)`);
    });

    ws.on('card_data', (event) => {
      console.log(`\n  >> card_data received!`);
      printDump(event);
      clearTimeout(timer);
      ws.disconnect();
      console.log('\n  Subscribe test PASSED');
      resolve(true);
    });

    ws.on('error', (err) => {
      console.log(`  WS error: ${err.message}`);
    });

    try {
      await ws.connect();
      console.log('  Connected to NFC Agent WS');
      await ws.subscribe(readerIndex, { includeRaw: true });
      console.log(`  Subscribed to reader ${readerIndex} with includeRaw: true`);

      timer = setTimeout(() => {
        console.log('\n  No card tapped within 30s — subscribe test skipped');
        ws.disconnect();
        resolve(false);
      }, 30000);
    } catch (err) {
      console.log(`  Connection failed: ${err.message}`);
      resolve(false);
    }
  });
}

// ============================================================================
// Main
// ============================================================================

const args = process.argv.slice(2);
const testArg = args.find((a) => a.startsWith('--test='))?.split('=')[1]
  ?? (args[args.indexOf('--test') + 1]);
const readerIndex = parseInt(
  args.find((a) => a.startsWith('--reader='))?.split('=')[1]
  ?? args[args.indexOf('--reader') + 1]
  ?? '0'
);

const runHttp = !testArg || testArg === 'http' || testArg === 'all';
const runWs = !testArg || testArg === 'ws' || testArg === 'all';
const runSubscribe = testArg === 'subscribe';

console.log('='.repeat(60));
console.log('  NFC Agent Raw Dump Test (JavaScript SDK)');
console.log('='.repeat(60));

if (runHttp) await testHttpDump(readerIndex);
if (runWs) await testWsDump(readerIndex);
if (runSubscribe) await testWsSubscribeRaw(readerIndex);

console.log('\n' + '='.repeat(60));
console.log('  Done');
console.log('='.repeat(60));
