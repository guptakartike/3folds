import {
  OverviewResponse,
  ExceptionsResponse,
  AuditTrailResponse,
  UploadResponse,
  RunResponse,
  ResetResponse,
} from '../types/api';

const API_BASE = '/api';

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let errorMsg = `Server error (${res.status} ${res.statusText})`;
    try {
      const data = await res.json();
      if (data && data.error) {
        errorMsg = data.error;
      }
    } catch {
      // Body not JSON
    }
    throw new Error(errorMsg);
  }
  return res.json();
}

export const api = {
  async getOverview(): Promise<OverviewResponse> {
    const res = await fetch(`${API_BASE}/overview`);
    return handleResponse<OverviewResponse>(res);
  },

  async getExceptions(): Promise<ExceptionsResponse> {
    const res = await fetch(`${API_BASE}/exceptions`);
    return handleResponse<ExceptionsResponse>(res);
  },

  async getAuditTrail(): Promise<AuditTrailResponse> {
    const res = await fetch(`${API_BASE}/audit-trail`);
    return handleResponse<AuditTrailResponse>(res);
  },

  async uploadFile(kind: 'settlements' | 'bank-statements' | 'ledger', file: File): Promise<UploadResponse> {
    const formData = new FormData();
    formData.append('file', file);
    const res = await fetch(`${API_BASE}/upload/${kind}`, {
      method: 'POST',
      body: formData,
    });
    return handleResponse<UploadResponse>(res);
  },

  async runReconciliation(): Promise<RunResponse> {
    const res = await fetch(`${API_BASE}/run`, {
      method: 'POST',
    });
    return handleResponse<RunResponse>(res);
  },

  async resetState(): Promise<ResetResponse> {
    const res = await fetch(`${API_BASE}/reset`, {
      method: 'POST',
    });
    return handleResponse<ResetResponse>(res);
  },

  getSampleDownloadUrl(kind: 'settlements' | 'bank-statements' | 'ledger', format: 'csv' | 'json' = 'csv'): string {
    return `${API_BASE}/sample/${kind}?format=${format}`;
  },
};
