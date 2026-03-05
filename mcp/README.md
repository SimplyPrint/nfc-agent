# NFC Agent MCP Server

MCP (Model Context Protocol) server for the [NFC Agent](https://github.com/SimplyPrint/nfc-agent). Allows Claude Code and other MCP clients to read/write NFC cards, manage filament spools via SimplyPrint, and interact with whatt.io materials/products.

## Quick Start

```bash
cd mcp
npm install
npm run build
```

Then add to your Claude Code MCP config (`~/.claude/config.json`):

```json
{
  "mcpServers": {
    "nfc-agent": {
      "command": "node",
      "args": ["/path/to/nfc-agent/mcp/dist/index.js"],
      "env": {
        "NFC_AGENT_URL": "http://127.0.0.1:32145",
        "SIMPLYPRINT_API_KEY": "your-key-here",
        "SIMPLYPRINT_BASE_URL": "https://api.simplyprint.io/{org_id}",
        "WHATTIO_TOKEN": "your-sanctum-token"
      }
    }
  }
}
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `NFC_AGENT_URL` | `http://127.0.0.1:32145` | NFC Agent URL |
| `NFC_AGENT_REPO_PATH` | — | Path to nfc-agent repo (enables dev tools) |
| `SIMPLYPRINT_API_KEY` | — | SimplyPrint API key (enables `sp_*` tools) |
| `SIMPLYPRINT_BASE_URL` | `https://api.simplyprint.io/0` | SimplyPrint API base URL including org ID |
| `WHATTIO_TOKEN` | — | whatt.io Sanctum bearer token (enables `whattio_*` tools) |
| `WHATTIO_TEAM_ID` | — | whatt.io team ID (optional) |

## Tools

### Always Available

| Tool | Description |
|------|-------------|
| `nfc_agent_status` | Check agent connection and version |
| `nfc_agent_stop` | Shutdown the agent |
| `nfc_list_readers` | List connected readers |
| `nfc_list_supported_readers` | List supported hardware |
| `nfc_read_card` | Read card UID, type, NDEF data, and full raw memory in one call |
| `nfc_write_card` | Write NDEF data (text/json/url/binary) |
| `nfc_write_records` | Write multiple NDEF records |
| `nfc_erase_card` | Erase NDEF data |
| `nfc_lock_card` | Permanently lock card |
| `nfc_set_password` | Set NTAG/Ultralight password |
| `nfc_remove_password` | Remove card password |
| `nfc_poll_card` | Wait for card to be presented |
| `nfc_read_mifare_block` | Read MIFARE Classic block |
| `nfc_write_mifare_block` | Write MIFARE Classic block |
| `nfc_write_mifare_blocks` | Batch write MIFARE blocks |
| `nfc_derive_uid_key` | Derive MIFARE key from UID via AES |
| `nfc_read_ultralight_page` | Read Ultralight/NTAG page |
| `nfc_write_ultralight_page` | Write Ultralight/NTAG page |
| `nfc_write_ultralight_pages` | Batch write Ultralight/NTAG pages |
| `nfc_dump_card` | Dump full raw card memory (WS, 30s timeout) |
| `nfc_dump_card_http` | Dump full raw card memory (HTTP) |

### Dev Tools (`NFC_AGENT_REPO_PATH` required)

| Tool | Description |
|------|-------------|
| `nfc_agent_build` | Build agent binary (`go build`) |
| `nfc_agent_test` | Run Go tests |
| `nfc_agent_start` | Start agent from source |
| `nfc_agent_logs` | Fetch agent logs |

### SimplyPrint (`SIMPLYPRINT_API_KEY` required)

| Tool | Description |
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

| Tool | Description |
|------|-------------|
| `whattio_list_materials` | List all materials |
| `whattio_get_material` | Get material by ID |
| `whattio_create_material` | Create a new material |
| `whattio_list_products` | List all products |
| `whattio_get_product` | Get product by ID |
| `whattio_write_product_to_card` | Write product info to NFC card |

## Getting a whatt.io Token

whatt.io uses Sanctum token authentication. Generate a token once:

```bash
curl -X POST "https://whatt.io/api/sanctum/token" \
  -d "email=you@example.com&password=yourpassword&device_name=nfc-agent-mcp"
```

Store the returned token as `WHATTIO_TOKEN`.

## Example Usage

```
"Flash spool #1234 to the NTAG216 card on reader 0 using the OpenSpool standard"
→ sp_flash_spool_to_card(fid=1234, standard="openspool")

"What card is on reader 0?"
→ nfc_read_card(reader=0)

"Write 'Hello World' to the card"
→ nfc_write_card(reader=0, data="Hello World", dataType="text")

"Run the NFC agent tests for the core package"
→ nfc_agent_test(pkg="./internal/core/...")
```
