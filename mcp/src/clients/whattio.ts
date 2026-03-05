export interface WhattioMaterial {
  id: number;
  brand: string;
  name: string;
  origin_country?: string;
  code?: string;
  color_code?: string;
  recycled?: boolean;
  recyclability?: string;
}

export interface WhattioProduct {
  id: number;
  name: string;
  product_number: string;
  gtin_number?: string;
  category_id: number;
  description?: string;
}

export interface CreateMaterialOptions {
  brand: string;
  name: string;
  origin_country: string;
  code?: string;
  color_code?: string;
  recycled?: boolean;
  recyclability?: string;
}

export class WhattioClient {
  private readonly baseUrl = 'https://whatt.io';
  private readonly token: string;
  private readonly teamId: string | undefined;

  constructor(token: string, teamId?: string) {
    this.token = token;
    this.teamId = teamId;
  }

  private async request<T>(
    path: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseUrl}/api${path}`;
    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this.token}`,
        ...options.headers,
      },
    });

    if (!response.ok) {
      let message = `HTTP ${response.status}`;
      try {
        const data = await response.json() as Record<string, unknown>;
        if (typeof data['message'] === 'string') message = data['message'];
      } catch {
        // ignore
      }
      throw new Error(`whatt.io API error: ${message}`);
    }

    return response.json() as Promise<T>;
  }

  async setTeam(): Promise<void> {
    if (!this.teamId) return;
    await this.request('/set_team', {
      method: 'POST',
      body: JSON.stringify({ team_id: this.teamId }),
    });
  }

  async listMaterials(): Promise<WhattioMaterial[]> {
    return this.request('/materials');
  }

  async getMaterial(id: number): Promise<WhattioMaterial> {
    return this.request(`/material/${id}`);
  }

  async createMaterial(options: CreateMaterialOptions): Promise<WhattioMaterial> {
    return this.request('/material', {
      method: 'POST',
      body: JSON.stringify(options),
    });
  }

  async listProducts(): Promise<WhattioProduct[]> {
    return this.request('/products');
  }

  async getProduct(id: number): Promise<WhattioProduct> {
    return this.request(`/product/${id}`);
  }
}
