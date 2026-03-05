import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { NFCAgentClient } from '@simplyprint/nfc-agent';
import { SimplyPrintClient } from '../clients/simplyprint.js';

/** Map NFC Agent card type string to SimplyPrint nfc_tag_type enum value, or null if unrecognized. */
function cardTypeToTagType(cardType: string): string | null {
  const t = cardType.toLowerCase();
  if (t.includes('ntag213')) return 'ntag213';
  if (t.includes('ntag215')) return 'ntag215';
  if (t.includes('ntag216')) return 'ntag216';
  if (t.includes('ntag424')) return 'ntag424';
  if (t.includes('mifare classic') || t.includes('mifare 1k')) return 'mifare_1k';
  if (t.includes('mifare 4k')) return 'mifare_4k';
  if (t.includes('ultralight')) return 'ultralight';
  return null;
}

export function registerSimplyPrintTools(
  server: McpServer,
  sp: SimplyPrintClient,
  nfc: NFCAgentClient,
  agentUrl: string
): void {
  server.registerTool(
    'sp_list_filaments',
    {
      title: 'SimplyPrint: List Filaments',
      description: 'List all filament spools in your SimplyPrint account',
    },
    async () => {
      const filaments = await sp.getFilaments();
      if (!filaments || (Array.isArray(filaments) && filaments.length === 0)) {
        return { content: [{ type: 'text' as const, text: 'No filament spools found' }] };
      }
      return { content: [{ type: 'text' as const, text: JSON.stringify(filaments, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_get_supported_standards',
    {
      title: 'SimplyPrint: Get NFC Standards',
      description: 'Get supported NFC standards, tag types, and color/transport modes from SimplyPrint',
    },
    async () => {
      const standards = await sp.getSupportedStandards();
      return { content: [{ type: 'text' as const, text: JSON.stringify(standards, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_get_spool_flashing_data',
    {
      title: 'SimplyPrint: Get Spool Flashing Data',
      description: 'Get the NDEF flashing data for a filament spool — returns the records to write to the NFC card',
      inputSchema: z.object({
        fid: z.number().int().positive().describe('Filament spool ID'),
        nfc_tag_type: z.string().describe('NFC tag type (e.g. "ntag216", "mifare_1k"). Use sp_get_supported_standards for valid values.'),
        standard: z.string().describe('Material standard (e.g. "openspool", "bambulab", "simple_url"). Use sp_get_supported_standards for valid values.'),
        uid: z.string().optional().describe('Card UID (required for some standards like Creality that use AES encryption)'),
        write_url_first_block: z.boolean().default(false).describe('Embed a SimplyPrint URL in the first NDEF block'),
        include_current_state: z.boolean().default(false).describe('Include remaining weight/length in tag data'),
      }),
    },
    async ({ fid, nfc_tag_type, standard, uid, write_url_first_block, include_current_state }) => {
      const result = await sp.getSpoolFlashingData(fid, {
        nfc_tag_type,
        standard,
        uid,
        write_url_first_block,
        include_current_state,
      });
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_assign_nfc',
    {
      title: 'SimplyPrint: Assign NFC to Spool',
      description: 'Record that an NFC card has been assigned to a filament spool in SimplyPrint',
      inputSchema: z.object({
        fid: z.number().int().positive().describe('Filament spool ID'),
        nfc_id: z.string().describe('NFC card UID (from nfc_read_card)'),
        standard: z.string().optional().describe('Material standard that was written'),
        tag_type: z.string().optional().describe('NFC tag type that was used'),
        include_url: z.boolean().default(false).describe('Whether a URL was embedded'),
        include_state: z.boolean().default(false).describe('Whether current state was embedded'),
      }),
    },
    async ({ fid, nfc_id, standard, tag_type, include_url, include_state }) => {
      const result = await sp.assignNfc(fid, {
        nfc_id,
        standard,
        tag_type,
        include_url,
        include_state,
        write_method: 'nfc-agent',
      });
      return { content: [{ type: 'text' as const, text: JSON.stringify(result.spool, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_create_filament',
    {
      title: 'SimplyPrint: Create Filament Spool',
      description: 'Create a new filament spool in SimplyPrint',
      inputSchema: z.object({
        color_name: z.string().describe('Color name (e.g. "Galaxy Black")'),
        color_hex: z.string().describe('Hex color code (e.g. "#1A1A2E")'),
        width: z.union([z.literal(1.75), z.literal(2.85), z.literal(3.0)]).describe('Filament diameter in mm'),
        brand: z.string().describe('Brand name (e.g. "Polymaker")'),
        filament_type: z.number().int().describe('FilamentProfile ID for material type'),
        total_length_type: z.enum(['kg', 'g', 'meter', 'mm']).describe('Unit for total length'),
        total_length: z.number().positive().describe('Total filament amount'),
        left_length_type: z.enum(['kg', 'g', 'meter', 'mm', 'percent']).describe('Unit for remaining length'),
        length_used: z.number().min(0).describe('Amount used'),
        amount: z.number().int().min(1).max(500).default(1).describe('Number of identical spools to create'),
        cost: z.number().optional().describe('Purchase price'),
        nfc_id: z.string().optional().describe('Pre-assign an NFC ID'),
        custom_note: z.string().optional().describe('Custom notes'),
      }),
    },
    async (opts) => {
      const result = await sp.createFilament(opts as Parameters<typeof sp.createFilament>[0]);
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  // ── Resolve / search ─────────────────────────────────────────────────────

  server.registerTool(
    'sp_identify_card',
    {
      title: 'SimplyPrint: Identify Card on Reader',
      description:
        'Read the card currently on a reader, then resolve it against SimplyPrint — ' +
        'returns the assigned spool (if any), FilamentDB matches, and full card info in one call.',
      inputSchema: z.object({
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
      }),
    },
    async ({ reader }) => {
      // 1. Read the card (unified endpoint — metadata + NDEF + raw memory)
      const res = await fetch(`${agentUrl}/v1/readers/${reader}/read`);
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as Record<string, unknown>;
        throw new Error(typeof err['error'] === 'string' ? err['error'] : `HTTP ${res.status}`);
      }
      const card = await res.json() as Record<string, unknown>;

      // 2. Build resolve params from what we got
      const nfc_id = typeof card['uid'] === 'string' ? card['uid'] : undefined;

      // Extract NDEF text/URL content to pass as nfc_content
      const ndefData = card['data'];
      const ndefUrl = card['url'];
      const nfc_content = typeof ndefUrl === 'string' ? ndefUrl
        : typeof ndefData === 'string' ? ndefData
        : undefined;

      // 3. Resolve against SimplyPrint
      const resolved = await sp.resolve({ nfc_id, nfc_content });

      return {
        content: [{
          type: 'text' as const,
          text: JSON.stringify({ card, resolved }, null, 2),
        }],
      };
    }
  );

  server.registerTool(
    'sp_resolve',
    {
      title: 'SimplyPrint: Resolve / Identify',
      description:
        'Find spools, printers, or FilamentDB entries by search term, NFC card UID, or NFC card content (URL/JSON read from a tag). ' +
        'Pass nfc_id to look up which spool is assigned to a card. ' +
        'Pass nfc_content to resolve a URL or JSON payload read from an NFC tag. ' +
        'Pass search to do a general text search.',
      inputSchema: z.object({
        search: z.string().optional().describe('Free-text search (spool name, color, UID, EAN, …)'),
        nfc_id: z.string().optional().describe('NFC card UID (from nfc_read_card) — finds the spool assigned to this card'),
        nfc_content: z.string().optional().describe('Content read from an NFC tag (URL or JSON) — resolved to a spool/printer'),
        filament: z.boolean().default(false).describe('Restrict results to filament spools only'),
        filamentdb: z.boolean().default(false).describe('Restrict results to FilamentDB entries only'),
        printer: z.boolean().default(false).describe('Restrict results to printers only'),
      }),
    },
    async ({ search, nfc_id, nfc_content, filament, filamentdb, printer }) => {
      const result = await sp.resolve({ search, nfc_id, nfc_content, filament, filamentdb, printer });
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  // ── FilamentDB ────────────────────────────────────────────────────────────

  server.registerTool(
    'sp_db_brands',
    {
      title: 'SimplyPrint: FilamentDB — List Brands',
      description: 'List all filament brands in the SimplyPrint FilamentDB (community + official)',
      inputSchema: z.object({
        only_officials: z.boolean().default(false).describe('Only return official/verified brands'),
      }),
    },
    async ({ only_officials }) => {
      const result = await sp.getDbBrands(only_officials);
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_db_brand',
    {
      title: 'SimplyPrint: FilamentDB — Get Brand',
      description: 'Get a FilamentDB brand and its material types (PLA, PETG, ABS, …)',
      inputSchema: z.object({
        brand_id: z.number().int().positive().optional().describe('Brand ID'),
        brand_name: z.string().optional().describe('Brand name (e.g. "Polymaker")'),
      }),
    },
    async ({ brand_id, brand_name }) => {
      const result = await sp.getDbBrand({ brandId: brand_id, brandName: brand_name });
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_db_material_types',
    {
      title: 'SimplyPrint: FilamentDB — Get Material Types',
      description: 'Get material types from FilamentDB. Optionally filtered to a specific brand.',
      inputSchema: z.object({
        brand_id: z.number().int().positive().optional().describe('Filter to a specific brand ID'),
        brand_name: z.string().optional().describe('Filter to a specific brand name'),
      }),
    },
    async ({ brand_id, brand_name }) => {
      const result = await sp.getDbMaterialTypes({ brandId: brand_id, brandName: brand_name });
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_db_filaments',
    {
      title: 'SimplyPrint: FilamentDB — Get Filaments',
      description: 'Get filament profiles for a brand in FilamentDB, optionally filtered by material type',
      inputSchema: z.object({
        brand_id: z.number().int().positive().optional().describe('Brand ID'),
        brand_name: z.string().optional().describe('Brand name'),
        material_type_id: z.number().int().positive().optional().describe('Filter to a specific material type ID'),
      }),
    },
    async ({ brand_id, brand_name, material_type_id }) => {
      const result = await sp.getDbFilaments({
        brandId: brand_id,
        brandName: brand_name,
        materialTypeId: material_type_id,
      });
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_db_colors',
    {
      title: 'SimplyPrint: FilamentDB — Get Colors',
      description: 'Get all color variants for a specific filament profile in FilamentDB',
      inputSchema: z.object({
        filament_id: z.number().int().positive().describe('FilamentDB filament profile ID'),
      }),
    },
    async ({ filament_id }) => {
      const result = await sp.getDbColors(filament_id);
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_db_stores',
    {
      title: 'SimplyPrint: FilamentDB — Get Stores',
      description: 'List known filament stores/retailers from FilamentDB',
    },
    async () => {
      const result = await sp.getDbStores();
      return { content: [{ type: 'text' as const, text: JSON.stringify(result, null, 2) }] };
    }
  );

  server.registerTool(
    'sp_flash_spool_to_card',
    {
      title: 'SimplyPrint: Flash Spool to NFC Card',
      description:
        'All-in-one: reads card UID → fetches flashing data from SimplyPrint → writes to card → assigns NFC in SimplyPrint. ' +
        'Place the card on the reader before calling this tool.',
      inputSchema: z.object({
        fid: z.number().int().positive().describe('Filament spool ID to flash'),
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        standard: z.string().describe('Material standard to use (e.g. "openspool", "bambulab"). Use sp_get_supported_standards to list options.'),
        write_url_first_block: z.boolean().default(false).describe('Embed SimplyPrint URL in first NDEF block'),
        include_current_state: z.boolean().default(false).describe('Include remaining weight/length in tag data'),
      }),
    },
    async ({ fid, reader, standard, write_url_first_block, include_current_state }) => {
      const steps: string[] = [];

      // 1. Read card to get UID and type
      const card = await nfc.readCard(reader, { refresh: true });
      steps.push(`Card detected: ${card.type} (UID: ${card.uid})`);

      const nfc_tag_type = cardTypeToTagType(card.type ?? '');
      if (!nfc_tag_type) {
        throw new Error(`Unrecognized card type "${card.type}" — cannot determine SimplyPrint tag type. Flash the card manually using sp_get_spool_flashing_data with an explicit nfc_tag_type.`);
      }
      steps.push(`Mapped to tag type: ${nfc_tag_type}`);

      // 2. Fetch flashing data from SimplyPrint
      const flashResult = await sp.getSpoolFlashingData(fid, {
        nfc_tag_type,
        standard,
        uid: card.uid,
        write_url_first_block,
        include_current_state,
      });

      const flashingData = flashResult.flashingData[0];
      if (!flashingData) {
        throw new Error('No flashing data returned for this spool');
      }

      const { ndef_records, estimated_bytes, tag_max_bytes, fits_on_tag } = flashingData.flashing_data;

      if (!fits_on_tag) {
        throw new Error(
          `Data (${estimated_bytes} bytes) does not fit on this tag (max ${tag_max_bytes} bytes). ` +
          `Try a different standard or a larger tag type.`
        );
      }

      steps.push(`Got ${ndef_records.length} NDEF records (${estimated_bytes}/${tag_max_bytes} bytes)`);

      // 3. Write NDEF records to card
      const res = await fetch(`${agentUrl}/v1/readers/${reader}/records`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ records: ndef_records }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({})) as Record<string, unknown>;
        throw new Error(`Write failed: ${typeof err['error'] === 'string' ? err['error'] : `HTTP ${res.status}`}`);
      }
      steps.push('NDEF records written to card');

      // 4. Assign NFC in SimplyPrint
      const assignResult = await sp.assignNfc(fid, {
        nfc_id: card.uid,
        standard,
        tag_type: nfc_tag_type ?? undefined,
        include_url: write_url_first_block,
        include_state: include_current_state,
        write_method: 'nfc-agent',
      });
      steps.push(`NFC assigned to spool (ID: ${assignResult.spool.id})`);

      return {
        content: [
          {
            type: 'text' as const,
            text: [
              '✓ Spool flashed successfully',
              '',
              ...steps.map((s) => `  ${s}`),
              '',
              `Spool: ${assignResult.spool.brandName} ${assignResult.spool.colorName}`,
              `Standard: ${standard}`,
              `Card UID: ${card.uid}`,
            ].join('\n'),
          },
        ],
      };
    }
  );
}
