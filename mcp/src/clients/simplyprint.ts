export interface FilamentSpool {
  id: number;
  uniqId: string;
  colorName: string;
  colorHex: string;
  width: number;
  cost: number | null;
  brandName: string;
  nfcId: string | string[] | null;
  nfcWrittenAt: string | null;
  nfcStandardWritten: string | null;
  nfcStandardVersionWritten: string | null;
  nfcTagType: string | null;
  nfcHasDynamicData: boolean;
  nfcWriteMethod: string | null;
  mmLength: number;
  lengthUsed: number;
  location: { id: number; name: string } | null;
}

export interface NfcStandards {
  standards: string[];
  nfc_tag_types: string[];
  nfc_color_modes: string[];
  nfc_transport_modes: string[];
  nfc_url_block_writing_rule: string[];
}

export interface NdefRecord {
  type: string;
  data: string;
  mimeType?: string;
}

export interface SpoolFlashingData {
  spool_id: number;
  spool_uid: string;
  material_standard_data: unknown;
  flashing_data: {
    ndef_records: NdefRecord[];
    estimated_bytes: number;
    tag_max_bytes: number;
    fits_on_tag: boolean;
  };
  bin_file?: string;
}

export interface GetSpoolFlashingDataOptions {
  nfc_tag_type: string;
  standard: string;
  uid?: string;
  write_url_first_block?: boolean;
  include_current_state?: boolean;
  overrides?: Record<string, { material_code?: number; color_code?: number }>;
}

export interface AssignNfcOptions {
  nfc_id: string | string[];
  standard?: string;
  standard_version?: string;
  tag_type?: string;
  include_url?: boolean;
  include_state?: boolean;
  write_method?: string;
}

export interface CreateFilamentOptions {
  color_name: string;
  color_hex: string;
  width: 1.75 | 2.85 | 3.0;
  brand: string;
  filament_type: number;
  total_length_type: 'kg' | 'g' | 'meter' | 'mm';
  total_length: number;
  left_length_type: 'kg' | 'g' | 'meter' | 'mm' | 'percent';
  length_used: number;
  amount?: number;
  cost?: number;
  brand_id?: number;
  nfc_id?: string;
  custom_note?: string;
}

export interface ResolveParams {
  search?: string;
  nfc_id?: string;
  nfc_content?: string;
  filament?: boolean;
  filamentdb?: boolean;
  printer?: boolean;
  printergroup?: boolean;
}

export class SimplyPrintClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;

  constructor(baseUrl: string, apiKey: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.apiKey = apiKey;
  }

  private async request<T>(
    path: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        'X-API-KEY': this.apiKey,
        ...options.headers,
      },
    });

    const data = await response.json() as Record<string, unknown>;

    if (!response.ok || data['status'] === false) {
      const message = typeof data['message'] === 'string'
        ? data['message']
        : `HTTP ${response.status}`;
      throw new Error(`SimplyPrint API error: ${message}`);
    }

    return data as T;
  }

  async testConnection(): Promise<{ status: boolean; message: string }> {
    return this.request('/account/Test');
  }

  async getSupportedStandards(): Promise<NfcStandards> {
    return this.request('/nfc/GetSupportedStandards');
  }

  async getSpoolFlashingData(
    fid: number,
    options: GetSpoolFlashingDataOptions
  ): Promise<{ flashingData: SpoolFlashingData[] }> {
    return this.request(`/nfc/GetSpoolFlashingData?fid=${fid}`, {
      method: 'POST',
      body: JSON.stringify(options),
    });
  }

  async getFilaments(): Promise<FilamentSpool[]> {
    const data = await this.request<{ filaments?: FilamentSpool[] }>('/filament/GetFilament');
    return data.filaments ?? (data as unknown as FilamentSpool[]);
  }

  async assignNfc(fid: number, options: AssignNfcOptions): Promise<{ spool: FilamentSpool }> {
    return this.request(`/filament/AssignNfc?fid=${fid}`, {
      method: 'POST',
      body: JSON.stringify(options),
    });
  }

  async createFilament(options: CreateFilamentOptions): Promise<{
    filament_ids: string[];
    created_spools: FilamentSpool[];
  }> {
    return this.request('/filament/Create', {
      method: 'POST',
      body: JSON.stringify({ amount: 1, ...options }),
    });
  }

  /** Search/resolve by text, NFC UID, or NFC content (URL/JSON from tag) */
  async resolve(params: ResolveParams): Promise<unknown> {
    const qs = new URLSearchParams();
    if (params.search) qs.set('search', params.search);
    if (params.nfc_id) qs.set('nfc_id', params.nfc_id);
    if (params.nfc_content) qs.set('nfc_content', params.nfc_content);
    if (params.filament) qs.set('filament', 'true');
    if (params.filamentdb) qs.set('filamentdb', 'true');
    if (params.printer) qs.set('printer', 'true');
    if (params.printergroup) qs.set('printergroup', 'true');
    return this.request(`/resolve/FindBySearch?${qs.toString()}`);
  }

  async getDbBrands(onlyOfficials = false): Promise<unknown> {
    const qs = onlyOfficials ? '?only_officials=true' : '';
    return this.request(`/filament/db/GetBrands${qs}`);
  }

  async getDbBrand(params: { brandId?: number; brandName?: string }): Promise<unknown> {
    const qs = new URLSearchParams();
    if (params.brandId !== undefined) qs.set('brandId', String(params.brandId));
    if (params.brandName) qs.set('brandName', params.brandName);
    return this.request(`/filament/db/GetBrand?${qs.toString()}`);
  }

  async getDbMaterialTypes(params: { brandId?: number; brandName?: string } = {}): Promise<unknown> {
    const qs = new URLSearchParams();
    if (params.brandId !== undefined) qs.set('brandId', String(params.brandId));
    if (params.brandName) qs.set('brandName', params.brandName);
    return this.request(`/filament/db/GetMaterialTypes?${qs.toString()}`);
  }

  async getDbFilaments(params: { brandId?: number; brandName?: string; materialTypeId?: number }): Promise<unknown> {
    const qs = new URLSearchParams();
    if (params.brandId !== undefined) qs.set('brandId', String(params.brandId));
    if (params.brandName) qs.set('brandName', params.brandName);
    if (params.materialTypeId !== undefined) qs.set('materialTypeId', String(params.materialTypeId));
    return this.request(`/filament/db/GetFilaments?${qs.toString()}`);
  }

  async getDbColors(filamentId: number): Promise<unknown> {
    return this.request(`/filament/db/GetColors?filamentId=${filamentId}`);
  }

  async getDbStores(withCustom = true): Promise<unknown> {
    return this.request(`/filament/db/GetStores?withCustom=${withCustom}`);
  }
}
