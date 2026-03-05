import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { NFCAgentClient } from '@simplyprint/nfc-agent';
import { WhattioClient } from '../clients/whattio.js';

export function registerWhattioTools(
  server: McpServer,
  whattio: WhattioClient,
  nfc: NFCAgentClient
): void {
  server.registerTool(
    'whattio_list_materials',
    {
      title: 'whatt.io: List Materials',
      description: 'List all materials in your whatt.io team',
    },
    async () => {
      const materials = await whattio.listMaterials();
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(materials, null, 2) }],
      };
    }
  );

  server.registerTool(
    'whattio_get_material',
    {
      title: 'whatt.io: Get Material',
      description: 'Get details for a specific material by ID',
      inputSchema: z.object({
        id: z.number().int().positive().describe('Material ID'),
      }),
    },
    async ({ id }) => {
      const material = await whattio.getMaterial(id);
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(material, null, 2) }],
      };
    }
  );

  server.registerTool(
    'whattio_create_material',
    {
      title: 'whatt.io: Create Material',
      description: 'Create a new material in whatt.io',
      inputSchema: z.object({
        brand: z.string().describe('Material brand name'),
        name: z.string().describe('Material name'),
        origin_country: z.string().describe('Country of origin (ISO code, e.g. "US")'),
        code: z.string().optional().describe('Material code or SKU'),
        color_code: z.string().optional().describe('Color code'),
        recycled: z.boolean().optional().describe('Whether the material is recycled'),
        recyclability: z.string().optional().describe('Recyclability information'),
      }),
    },
    async (opts) => {
      const material = await whattio.createMaterial(opts);
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(material, null, 2) }],
      };
    }
  );

  server.registerTool(
    'whattio_list_products',
    {
      title: 'whatt.io: List Products',
      description: 'List all products in your whatt.io team',
    },
    async () => {
      const products = await whattio.listProducts();
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(products, null, 2) }],
      };
    }
  );

  server.registerTool(
    'whattio_get_product',
    {
      title: 'whatt.io: Get Product',
      description: 'Get details for a specific product by ID',
      inputSchema: z.object({
        id: z.number().int().positive().describe('Product ID'),
      }),
    },
    async ({ id }) => {
      const product = await whattio.getProduct(id);
      return {
        content: [{ type: 'text' as const, text: JSON.stringify(product, null, 2) }],
      };
    }
  );

  server.registerTool(
    'whattio_write_product_to_card',
    {
      title: 'whatt.io: Write Product to NFC Card',
      description:
        'Write product information from whatt.io as NDEF data to an NFC card. ' +
        'Place the card on the reader first.',
      inputSchema: z.object({
        product_id: z.number().int().positive().describe('whatt.io product ID'),
        reader: z.number().int().min(0).default(0).describe('Reader index (0-based)'),
        format: z.enum(['url', 'text', 'json']).default('json').describe(
          'How to encode product data: url = write product URL, text = human-readable summary, json = full product JSON'
        ),
      }),
    },
    async ({ product_id, reader, format }) => {
      const product = await whattio.getProduct(product_id);
      const steps: string[] = [`Fetched product: ${product.name} (${product.product_number})`];

      let data: string;
      let dataType: 'url' | 'text' | 'json';

      if (format === 'url') {
        data = `https://whatt.io/product/${product.id}`;
        dataType = 'url';
      } else if (format === 'text') {
        data = [
          `Product: ${product.name}`,
          `Number: ${product.product_number}`,
          product.gtin_number ? `GTIN: ${product.gtin_number}` : '',
          product.description ? `Info: ${product.description}` : '',
        ]
          .filter(Boolean)
          .join('\n');
        dataType = 'text';
      } else {
        data = JSON.stringify({
          id: product.id,
          name: product.name,
          product_number: product.product_number,
          gtin_number: product.gtin_number,
        });
        dataType = 'json';
      }

      await nfc.writeCard(reader, { data, dataType });
      steps.push(`Wrote ${dataType} data to card on reader ${reader}`);

      return {
        content: [
          {
            type: 'text' as const,
            text: [
              '✓ Product written to card',
              '',
              ...steps.map((s) => `  ${s}`),
              `  Format: ${dataType}`,
              `  Data: ${data.slice(0, 100)}${data.length > 100 ? '...' : ''}`,
            ].join('\n'),
          },
        ],
      };
    }
  );
}
