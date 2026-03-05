# NFC Agent MCP Server

The `mcp/` directory contains an MCP (Model Context Protocol) server that exposes all NFC Agent capabilities as Claude Code tools. This is the **preferred way to interact with the agent during development** — no curl, no Python scripts, structured JSON output, and composite tools for common workflows.

## Setup

```bash
cd mcp
npm install
npm run build
```

Add to `~/.claude.json` under `mcpServers`:

```json
"nfc-agent": {
  "type": "stdio",
  "command": "node",
  "args": ["/path/to/nfc-agent/mcp/dist/index.js"],
  "env": {
    "NFC_AGENT_URL": "http://127.0.0.1:32145",
    "NFC_AGENT_REPO_PATH": "/path/to/nfc-agent",
    "SIMPLYPRINT_API_KEY": "...",
    "SIMPLYPRINT_BASE_URL": "https://api.simplyprint.io/{org_id}",
    "WHATTIO_TOKEN": "..."
  }
}
```

Restart Claude Code after editing the config.

## Tool Groups

### Always Available

| Tool | What it does |
|------|-------------|
| `nfc_agent_status` | Check agent connection + version |
| `nfc_agent_stop` | Shutdown the running agent |
| `nfc_list_readers` | List connected NFC readers |
| `nfc_list_supported_readers` | List all supported hardware models |
| `nfc_read_card` | Read card: UID, type, NDEF data, and full raw memory in one call |
| `nfc_write_card` | Write NDEF (text / json / url / binary) |
| `nfc_write_records` | Write multiple NDEF records |
| `nfc_erase_card` | Erase NDEF data |
| `nfc_lock_card` | Permanently lock card |
| `nfc_set_password` | Set NTAG/Ultralight password |
| `nfc_remove_password` | Remove card password |
| `nfc_poll_card` | Wait for a card to be presented |
| `nfc_read_mifare_block` | Read MIFARE Classic block |
| `nfc_write_mifare_block` | Write MIFARE Classic block |
| `nfc_write_mifare_blocks` | Batch write MIFARE blocks |
| `nfc_derive_uid_key` | Derive MIFARE key from UID via AES |
| `nfc_read_ultralight_page` | Read Ultralight/NTAG page |
| `nfc_write_ultralight_page` | Write Ultralight/NTAG page |
| `nfc_write_ultralight_pages` | Batch write Ultralight/NTAG pages |
| `nfc_dump_card` | Dump full raw card memory (WebSocket, 30s timeout) |
| `nfc_dump_card_http` | Dump full raw card memory (HTTP) |

### Dev Tools (`NFC_AGENT_REPO_PATH` required)

| Tool | What it does |
|------|-------------|
| `nfc_agent_build` | Build agent binary (`go build`) |
| `nfc_agent_test` | Run Go tests (optionally filter by package/pattern) |
| `nfc_agent_start` | Start agent from source |
| `nfc_agent_logs` | Fetch recent agent logs |

### SimplyPrint (`SIMPLYPRINT_API_KEY` required)

| Tool | What it does |
|------|-------------|
| `sp_identify_card` | **All-in-one:** read card on reader → resolve against SimplyPrint |
| `sp_resolve` | Find spools/printers by search term, NFC UID, or NFC content |
| `sp_list_filaments` | List all filament spools |
| `sp_get_supported_standards` | Get NFC standards/tag types |
| `sp_get_spool_flashing_data` | Get NDEF data for a spool |
| `sp_assign_nfc` | Assign NFC card to spool |
| `sp_create_filament` | Create a new filament spool |
| `sp_flash_spool_to_card` | **All-in-one:** read card → fetch data → write → assign |
| `sp_db_brands` | List all FilamentDB brands |
| `sp_db_brand` | Get brand details + material types |
| `sp_db_material_types` | Get material types (optionally for a brand) |
| `sp_db_filaments` | Get filament profiles for a brand |
| `sp_db_colors` | Get color variants for a filament profile |
| `sp_db_stores` | List known filament stores/retailers |

### whatt.io (`WHATTIO_TOKEN` required)

| Tool | What it does |
|------|-------------|
| `whattio_list_materials` | List all materials |
| `whattio_get_material` | Get material by ID |
| `whattio_create_material` | Create a new material |
| `whattio_list_products` | List all products |
| `whattio_get_product` | Get product by ID |
| `whattio_write_product_to_card` | Write product info to NFC card |

## Typical Dev Workflows

**After changing HTTP API code:**
```
nfc_agent_build → nfc_agent_status → nfc_read_card(reader=0)
```

**Debugging a card read issue:**
```
nfc_agent_logs          → check recent logs for errors
nfc_read_card(reader=0) → inspect raw output
nfc_dump_card(reader=0) → full memory if needed
```

**Running tests:**
```
nfc_agent_test                            → run all tests
nfc_agent_test(pkg="./internal/core/...") → run core tests only
nfc_agent_test(pattern="TestNTAG")        → run matching tests
```

**Flash a filament spool to a card:**
```
sp_flash_spool_to_card(fid=1234, standard="openspool", reader=0)
```

**Identify an unknown card:**
```
sp_identify_card(reader=0)   → UID + NDEF + SimplyPrint spool match
```

## Source Layout

```
mcp/
├── src/
│   ├── index.ts              # Entry: build server, register tools
│   ├── config.ts             # Env var parsing + feature flags
│   ├── tools/
│   │   ├── agent.ts          # Agent lifecycle + reader management
│   │   ├── card.ts           # Core NFC read/write/erase/lock/password
│   │   ├── advanced.ts       # MIFARE blocks, Ultralight pages, AES ops
│   │   ├── dev.ts            # Build, test, logs (gated on REPO_PATH)
│   │   ├── simplyprint.ts    # SimplyPrint filament + flash tools
│   │   └── whattio.ts        # whatt.io materials/products tools
│   └── clients/
│       ├── simplyprint.ts    # SimplyPrint HTTP client
│       └── whattio.ts        # whatt.io HTTP client
├── package.json
└── tsconfig.json
```

## When to Use curl / Python Scripts Instead

- The agent binary is not running and you need to test startup behaviour
- You're testing a raw HTTP response format not exposed through MCP tools
- You're writing a regression test that needs exact byte-level output
- MCP server is not configured in the current environment (CI, remote machine)
