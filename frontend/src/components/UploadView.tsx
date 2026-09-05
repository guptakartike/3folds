import React, { useState, useRef } from 'react';
import { api } from '../services/api';
import { UploadResponse, RunResponse } from '../types/api';
import { ErrorState } from './ErrorState';
import {
  UploadCloud,
  CheckCircle2,
  FileSpreadsheet,
  AlertCircle,
  Play,
  RotateCcw,
  Download,
  Loader2,
} from 'lucide-react';

interface UploadViewProps {
  onRunSuccess: (res: RunResponse) => void;
  onResetSuccess: () => void;
}

interface UploadSlotState {
  file: File | null;
  uploading: boolean;
  uploaded: boolean;
  response: UploadResponse | null;
  error: string | null;
}

export const UploadView: React.FC<UploadViewProps> = ({
  onRunSuccess,
  onResetSuccess,
}) => {
  const [settlements, setSettlements] = useState<UploadSlotState>({
    file: null,
    uploading: false,
    uploaded: false,
    response: null,
    error: null,
  });

  const [bankStatements, setBankStatements] = useState<UploadSlotState>({
    file: null,
    uploading: false,
    uploaded: false,
    response: null,
    error: null,
  });

  const [ledger, setLedger] = useState<UploadSlotState>({
    file: null,
    uploading: false,
    uploaded: false,
    response: null,
    error: null,
  });

  const [running, setRunning] = useState(false);
  const [runError, setRunError] = useState<string | null>(null);
  const [resetting, setResetting] = useState(false);
  const [resetMessage, setResetMessage] = useState<string | null>(null);

  const settlementInputRef = useRef<HTMLInputElement>(null);
  const bankInputRef = useRef<HTMLInputElement>(null);
  const ledgerInputRef = useRef<HTMLInputElement>(null);

  const handleFileUpload = async (
    kind: 'settlements' | 'bank-statements' | 'ledger',
    file: File,
    setter: React.Dispatch<React.SetStateAction<UploadSlotState>>
  ) => {
    setter({
      file,
      uploading: true,
      uploaded: false,
      response: null,
      error: null,
    });
    setRunError(null);
    setResetMessage(null);

    try {
      const resp = await api.uploadFile(kind, file);
      setter({
        file,
        uploading: false,
        uploaded: true,
        response: resp,
        error: null,
      });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Upload failed';
      setter({
        file,
        uploading: false,
        uploaded: false,
        response: null,
        error: msg,
      });
    }
  };

  const uploadedCount =
    (settlements.uploaded ? 1 : 0) +
    (bankStatements.uploaded ? 1 : 0) +
    (ledger.uploaded ? 1 : 0);

  const allUploaded = uploadedCount === 3;
  const neededCount = 3 - uploadedCount;

  const handleRunReconciliation = async () => {
    if (!allUploaded || running) return;

    setRunning(true);
    setRunError(null);
    setResetMessage(null);

    try {
      const res = await api.runReconciliation();
      setRunning(false);
      onRunSuccess(res);
    } catch (err: unknown) {
      setRunning(false);
      const msg = err instanceof Error ? err.message : 'Reconciliation execution failed';
      setRunError(msg);
    }
  };

  const handleReset = async () => {
    if (resetting) return;

    setResetting(true);
    setRunError(null);
    try {
      const resp = await api.resetState();
      setResetting(false);
      setSettlements({ file: null, uploading: false, uploaded: false, response: null, error: null });
      setBankStatements({ file: null, uploading: false, uploaded: false, response: null, error: null });
      setLedger({ file: null, uploading: false, uploaded: false, response: null, error: null });
      setResetMessage(resp.message || 'Reconciliation state reset to idle.');
      onResetSuccess();
    } catch (err: unknown) {
      setResetting(false);
      const msg = err instanceof Error ? err.message : 'Reset failed';
      setRunError(msg);
    }
  };

  const renderSlot = (
    title: string,
    kind: 'settlements' | 'bank-statements' | 'ledger',
    slot: UploadSlotState,
    setter: React.Dispatch<React.SetStateAction<UploadSlotState>>,
    inputRef: React.RefObject<HTMLInputElement>
  ) => {
    return (
      <div className={`upload-card ${slot.uploaded ? 'success' : ''}`}>
        <div className="upload-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <FileSpreadsheet size={18} style={{ color: 'var(--accent-primary)' }} />
            <h3 className="upload-title">{title}</h3>
          </div>
          {slot.uploaded && (
            <span
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: '4px',
                fontSize: '0.75rem',
                color: 'var(--tier-exact)',
                fontWeight: 600,
              }}
            >
              <CheckCircle2 size={14} />
              {slot.response?.rows} rows parsed
            </span>
          )}
        </div>

        <input
          type="file"
          accept=".csv,.json"
          ref={inputRef}
          style={{ display: 'none' }}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) handleFileUpload(kind, f, setter);
          }}
        />

        <div
          className="dropzone"
          onClick={() => inputRef.current?.click()}
        >
          <UploadCloud size={24} style={{ color: 'var(--text-muted)', marginBottom: '8px' }} />
          {slot.uploading ? (
            <p className="dropzone-text">Uploading and validating {kind}...</p>
          ) : slot.file ? (
            <p className="dropzone-text font-mono" style={{ color: 'var(--text-primary)' }}>
              {slot.file.name}
            </p>
          ) : (
            <p className="dropzone-text">Click to select CSV or JSON file</p>
          )}
          <p className="dropzone-hint">Auto-detected schema format</p>
        </div>

        {slot.error && (
          <div
            style={{
              padding: '0.75rem',
              backgroundColor: 'var(--tier-unresolved-bg)',
              border: '1px solid var(--tier-unresolved-border)',
              borderRadius: 'var(--radius-sm)',
              fontSize: '0.75rem',
              color: 'var(--tier-unresolved)',
              display: 'flex',
              alignItems: 'flex-start',
              gap: '6px',
            }}
          >
            <AlertCircle size={14} style={{ flexShrink: 0, marginTop: '2px' }} />
            <span>{slot.error}</span>
          </div>
        )}

        {slot.uploaded && slot.response?.preview && slot.response.preview.length > 0 && (
          <div className="preview-box">
            <div className="preview-title">Parsed Records Sample:</div>
            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.6875rem' }}>
                <tbody>
                  {slot.response.preview.map((row, rIdx) => (
                    <tr key={rIdx} style={{ borderBottom: '1px solid rgba(255, 255, 255, 0.05)' }}>
                      {Object.entries(row).map(([k, v], cIdx) => (
                        <td key={cIdx} style={{ padding: '3px 6px' }}>
                          <span style={{ color: 'var(--text-muted)' }}>{k}: </span>
                          <span className="font-mono" style={{ color: 'var(--text-primary)' }}>
                            {String(v)}
                          </span>
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="upload-view">
      <div className="samples-bar">
        <div>
          <h4 style={{ fontSize: '0.875rem', fontWeight: 600, marginBottom: '2px' }}>
            Download Sample Test Datasets
          </h4>
          <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
            Representative data generated by the backend reconciliation engine.
          </p>
        </div>

        <div className="samples-links">
          <a
            href={api.getSampleDownloadUrl('settlements', 'csv')}
            download="sample_settlements.csv"
            className="btn btn-secondary"
            style={{ fontSize: '0.75rem', padding: '0.375rem 0.625rem' }}
          >
            <Download size={12} />
            Settlements (CSV)
          </a>
          <a
            href={api.getSampleDownloadUrl('bank-statements', 'csv')}
            download="sample_bank_statements.csv"
            className="btn btn-secondary"
            style={{ fontSize: '0.75rem', padding: '0.375rem 0.625rem' }}
          >
            <Download size={12} />
            Bank Statements (CSV)
          </a>
          <a
            href={api.getSampleDownloadUrl('ledger', 'csv')}
            download="sample_ledger_entries.csv"
            className="btn btn-secondary"
            style={{ fontSize: '0.75rem', padding: '0.375rem 0.625rem' }}
          >
            <Download size={12} />
            Ledger (CSV)
          </a>
        </div>
      </div>

      <div className="upload-grid">
        {renderSlot('1. Settlement Data', 'settlements', settlements, setSettlements, settlementInputRef)}
        {renderSlot('2. Bank Statements', 'bank-statements', bankStatements, setBankStatements, bankInputRef)}
        {renderSlot('3. Internal Ledger', 'ledger', ledger, setLedger, ledgerInputRef)}
      </div>

      {runError && (
        <ErrorState
          title="Reconciliation Execution Error"
          message={runError}
        />
      )}

      {resetMessage && (
        <div
          style={{
            padding: '1rem',
            backgroundColor: 'var(--bg-surface)',
            border: '1px solid var(--border-default)',
            borderRadius: 'var(--radius-md)',
            marginBottom: '1.5rem',
            fontSize: '0.875rem',
            color: 'var(--text-primary)',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <CheckCircle2 size={16} style={{ color: 'var(--tier-exact)' }} />
          <span>{resetMessage}</span>
        </div>
      )}

      <div className="actions-bar">
        <button
          onClick={handleReset}
          disabled={resetting || running}
          className="btn btn-danger"
        >
          <RotateCcw size={14} />
          {resetting ? 'Resetting...' : 'Reset to Empty State'}
        </button>

        <button
          onClick={handleRunReconciliation}
          disabled={!allUploaded || running}
          className="btn btn-primary"
          style={{ minWidth: '240px' }}
        >
          {running ? (
            <>
              <Loader2 size={16} className="spinner" style={{ width: '16px', height: '16px', margin: 0, borderWidth: '2px' }} />
              Running Reconciliation Pipeline...
            </>
          ) : allUploaded ? (
            <>
              <Play size={16} />
              Run Reconciliation Pipeline
            </>
          ) : (
            `Run Reconciliation (${neededCount} of 3 files needed)`
          )}
        </button>
      </div>
    </div>
  );
};
