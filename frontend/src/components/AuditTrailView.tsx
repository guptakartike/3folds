import React, { useState, useMemo } from 'react';
import { AuditTrailResponse, AuditRow } from '../types/api';
import { ErrorState } from './ErrorState';
import { LoadingState } from './LoadingState';
import {
  Search,
  ArrowUpDown,
  Inbox,
  ArrowRight,
  ShieldCheck,
  CheckCircle,
  AlertCircle,
  Sparkles,
  Award,
} from 'lucide-react';

interface AuditTrailViewProps {
  data: AuditTrailResponse | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onNavigateToUpload: () => void;
}

type SortField = 'none' | 'tier' | 'amountDiff';
type SortDirection = 'asc' | 'desc';

export const AuditTrailView: React.FC<AuditTrailViewProps> = ({
  data,
  loading,
  error,
  onRetry,
  onNavigateToUpload,
}) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [tierFilter, setTierFilter] = useState<string>('ALL');
  const [sortField, setSortField] = useState<SortField>('none');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      if (sortDirection === 'asc') {
        setSortDirection('desc');
      } else {
        setSortField('none');
        setSortDirection('asc');
      }
    } else {
      setSortField(field);
      setSortDirection('asc');
    }
  };

  const processedRows = useMemo(() => {
    if (!data?.rows) return [];

    let result = [...data.rows];

    // Filter by tier
    if (tierFilter !== 'ALL') {
      result = result.filter((row) => row.tier.toUpperCase() === tierFilter);
    }

    // Filter by search
    if (searchTerm.trim()) {
      const term = searchTerm.toLowerCase().trim();
      result = result.filter(
        (row) =>
          row.orderId.toLowerCase().includes(term) ||
          row.settlementId.toLowerCase().includes(term) ||
          row.bankRef.toLowerCase().includes(term) ||
          row.reason.toLowerCase().includes(term)
      );
    }

    // Sort
    if (sortField === 'tier') {
      result.sort((a, b) => {
        const cmp = a.tier.localeCompare(b.tier);
        return sortDirection === 'asc' ? cmp : -cmp;
      });
    } else if (sortField === 'amountDiff') {
      result.sort((a, b) => {
        const parseAmount = (s: string) => {
          const clean = s.replace(/[^0-9.-]+/g, '');
          return parseFloat(clean) || 0;
        };
        const valA = parseAmount(a.amountDiff);
        const valB = parseAmount(b.amountDiff);
        return sortDirection === 'asc' ? valA - valB : valB - valA;
      });
    }

    return result;
  }, [data?.rows, tierFilter, searchTerm, sortField, sortDirection]);

  if (loading) {
    return <LoadingState message="Fetching reconciliation audit trail..." />;
  }

  if (error) {
    return (
      <ErrorState
        title="Failed to load audit trail"
        message={error}
        onRetry={onRetry}
      />
    );
  }

  if (!data || !data.has_run || data.status === 'idle') {
    return (
      <div className="idle-container">
        <Inbox className="idle-icon" />
        <h2 className="idle-title">No audit trail records yet</h2>
        <p className="idle-description">
          Reconciled transaction records will appear here once the reconciliation pipeline has been executed.
        </p>
        <button onClick={onNavigateToUpload} className="btn btn-primary">
          Go to Upload
          <ArrowRight size={16} />
        </button>
      </div>
    );
  }

  const getBadgeClass = (tier: string) => {
    switch (tier.toUpperCase()) {
      case 'EXACT':
        return 'tier-badge tier-exact';
      case 'FUZZY':
        return 'tier-badge tier-fuzzy';
      case 'BATCH':
        return 'tier-badge tier-batch';
      case 'AI-RESOLVED':
        return 'tier-badge tier-ai';
      default:
        return 'tier-badge tier-unresolved';
    }
  };

  const total = data.totalCount || 1;
  const exactCount = data.exactCount || 0;
  const fuzzyCount = data.fuzzyCount || 0;
  const batchCount = data.batchCount || 0;
  const deterministicCount = data.deterministicCount ?? (exactCount + fuzzyCount + batchCount);
  const aiCount = data.aiCount || 0;
  const unresolvedCount = data.unresolvedCount || 0;
  const reconciledCount = data.reconciledCount ?? (deterministicCount + aiCount);
  
  const recRate = data.reconciliationRate !== undefined
    ? data.reconciliationRate
    : Math.round((reconciledCount / total) * 1000) / 10;
  const recRatePct = `${recRate.toFixed(1)}%`;

  const evalData = data.evaluation;

  return (
    <div className="audit-trail-view">
      {/* Top Header */}
      <div style={{ marginBottom: '1.5rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.25rem' }}>
          <h1 className="page-title">Reconciliation Audit Trail</h1>
          <span
            style={{
              fontSize: '0.75rem',
              fontWeight: 700,
              backgroundColor: '#f1f5f9',
              color: '#475569',
              padding: '0.2rem 0.6rem',
              borderRadius: '9999px',
            }}
          >
            {data.totalCount} transactions
          </span>
          <span
            style={{
              fontSize: '0.75rem',
              fontWeight: 600,
              backgroundColor: '#f0fdf4',
              border: '1px solid #bbf7d0',
              color: '#15803d',
              padding: '0.15rem 0.5rem',
              borderRadius: '4px',
            }}
          >
            Auditable Reconciliation Record
          </span>
        </div>
        <p className="page-subtitle">
          Auditable reconciliation record tracking exact matches, fuzzy variances, batch groupings, AI-assisted resolutions, and exceptions.
        </p>
      </div>

      {/* Structured Tier Classification Summary Grid */}
      <div className="audit-summary-grid">
        <div className="audit-summary-card">
          <div className="audit-summary-label">
            <span>Deterministic Resolutions</span>
            <CheckCircle size={16} style={{ color: '#16a34a' }} />
          </div>
          <div className="audit-summary-val font-mono">
            {deterministicCount} <span style={{ fontSize: '1rem', color: 'var(--text-muted)' }}>/ {total}</span>
          </div>
          <div className="audit-summary-sub font-mono" style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
            <span>{exactCount} Exact</span>
            <span>•</span>
            <span>{fuzzyCount} Fuzzy</span>
            <span>•</span>
            <span>{batchCount} Batch</span>
          </div>
        </div>

        <div className="audit-summary-card">
          <div className="audit-summary-label">
            <span>AI-Assisted Resolutions</span>
            <Sparkles size={16} style={{ color: '#7c3aed' }} />
          </div>
          <div className="audit-summary-val font-mono">
            {aiCount} <span style={{ fontSize: '1rem', color: 'var(--text-muted)' }}>/ {total}</span>
          </div>
          <div className="audit-summary-sub">
            {aiCount > 0 ? `${aiCount} resolved by AI agent` : '0 AI-Resolved (Rules resolved all valid cases)'}
          </div>
        </div>

        <div className="audit-summary-card">
          <div className="audit-summary-label">
            <span>Exceptions (Unresolved)</span>
            <AlertCircle size={16} style={{ color: '#dc2626' }} />
          </div>
          <div className="audit-summary-val font-mono" style={{ color: unresolvedCount > 0 ? '#b91c1c' : 'inherit' }}>
            {unresolvedCount}
          </div>
          <div className="audit-summary-sub">
            {unresolvedCount > 0 ? 'Requires human investigation' : 'Zero unhandled exceptions'}
          </div>
        </div>

        <div className="audit-summary-card">
          <div className="audit-summary-label">
            <span>Automated Rate</span>
            <ShieldCheck size={16} style={{ color: '#0284c7' }} />
          </div>
          <div className="audit-summary-val font-mono">{recRatePct}</div>
          <div className="audit-summary-sub font-mono">
            {reconciledCount} of {total} Reconciled
          </div>
        </div>
      </div>

      {/* Ground Truth Evaluation Card in Audit Trail */}
      {evalData && evalData.hasEvaluation && (
        <div className="card" style={{ padding: '1.25rem 1.5rem', marginBottom: '1.5rem', backgroundColor: '#ffffff' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.75rem', flexWrap: 'wrap', gap: '0.5rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <Award size={16} style={{ color: '#0284c7' }} />
              <strong style={{ fontSize: '0.875rem', color: 'var(--text-primary)' }}>GROUND TRUTH EVALUATION</strong>
            </div>
            <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              Accuracy is verified against labeled ground truth benchmark
            </span>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: '0.75rem' }}>
            <div style={{ padding: '0.5rem 0.75rem', backgroundColor: '#f8fafc', borderRadius: '6px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', color: 'var(--text-muted)', fontWeight: 600, textTransform: 'uppercase' }}>Accuracy</div>
              <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: evalData.accuracy === 100 ? '#047857' : '#0f172a' }}>
                {evalData.accuracy.toFixed(1)}%
              </div>
            </div>

            <div style={{ padding: '0.5rem 0.75rem', backgroundColor: '#f8fafc', borderRadius: '6px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', color: 'var(--text-muted)', fontWeight: 600, textTransform: 'uppercase' }}>Expected Matches</div>
              <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: '#0284c7' }}>
                {evalData.matched} / {evalData.expectedMatches}
              </div>
            </div>

            <div style={{ padding: '0.5rem 0.75rem', backgroundColor: '#f8fafc', borderRadius: '6px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', color: 'var(--text-muted)', fontWeight: 600, textTransform: 'uppercase' }}>Genuine Exceptions</div>
              <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: '#7c3aed' }}>
                {evalData.detectedExceptions} / {evalData.expectedExceptions}
              </div>
            </div>

            <div style={{ padding: '0.5rem 0.75rem', backgroundColor: '#f8fafc', borderRadius: '6px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', color: 'var(--text-muted)', fontWeight: 600, textTransform: 'uppercase' }}>False Positives</div>
              <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: evalData.falsePositives === 0 ? '#047857' : '#b91c1c' }}>
                {evalData.falsePositives}
              </div>
            </div>

            <div style={{ padding: '0.5rem 0.75rem', backgroundColor: '#f8fafc', borderRadius: '6px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', color: 'var(--text-muted)', fontWeight: 600, textTransform: 'uppercase' }}>False Negatives</div>
              <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: evalData.falseNegatives === 0 ? '#047857' : '#b91c1c' }}>
                {evalData.falseNegatives}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Toolbar & Filters */}
      <div className="audit-toolbar">
        <div className="filter-group">
          {[
            { key: 'ALL', label: `All (${total})` },
            { key: 'EXACT', label: `Exact (${exactCount})` },
            { key: 'FUZZY', label: `Fuzzy (${fuzzyCount})` },
            { key: 'BATCH', label: `Batch (${batchCount})` },
            { key: 'AI-RESOLVED', label: `AI-Resolved (${aiCount})` },
            { key: 'UNRESOLVED', label: `Unresolved (${unresolvedCount})` },
          ].map((item) => (
            <button
              key={item.key}
              className={`filter-btn ${tierFilter === item.key ? 'active' : ''}`}
              onClick={() => setTierFilter(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>

        <div style={{ position: 'relative', width: '280px' }}>
          <Search
            size={16}
            style={{
              position: 'absolute',
              left: '10px',
              top: '50%',
              transform: 'translateY(-50%)',
              color: 'var(--text-muted)',
            }}
          />
          <input
            type="text"
            className="input-text"
            style={{ width: '100%', paddingLeft: '32px' }}
            placeholder="Search order, settlement, UTR..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </div>

      {/* Data Table */}
      <div className="table-wrapper">
        <table className="data-table">
          <thead>
            <tr>
              <th>Order ID</th>
              <th>Settlement ID</th>
              <th className="sortable" onClick={() => handleSort('tier')}>
                <div style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                  Tier
                  <ArrowUpDown size={12} />
                </div>
              </th>
              <th>Bank Reference</th>
              <th>Amount Diff</th>
              <th>Date Diff</th>
              <th>Resolution / Auditor Reason</th>
            </tr>
          </thead>
          <tbody>
            {processedRows.map((row: AuditRow, idx: number) => (
              <tr key={idx}>
                <td className="font-mono" style={{ fontWeight: 600, color: '#0369a1' }}>
                  {row.orderId}
                </td>
                <td className="font-mono" style={{ color: 'var(--text-secondary)' }}>
                  {row.settlementId}
                </td>
                <td>
                  <span className={getBadgeClass(row.tier)}>{row.tier}</span>
                </td>
                <td className="font-mono" style={{ color: row.bankRef === '-' ? 'var(--text-muted)' : 'var(--text-primary)' }}>
                  {row.bankRef}
                </td>
                <td
                  className="font-mono"
                  style={{
                    fontWeight: 600,
                    color: row.amountDiffColor === 'red' ? '#b91c1c' : '#15803d',
                  }}
                >
                  {row.amountDiff}
                </td>
                <td
                  className="font-mono"
                  style={{
                    color: row.dateDiffColor === 'red' ? '#b91c1c' : 'var(--text-secondary)',
                  }}
                >
                  {row.dateDiff}
                </td>
                <td style={{ maxWidth: '340px', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                  {row.reason}
                </td>
              </tr>
            ))}

            {processedRows.length === 0 && (
              <tr>
                <td colSpan={7} style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-muted)' }}>
                  No transaction records matched the active filter criteria.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '1rem', fontSize: '0.75rem', color: 'var(--text-muted)' }}>
        <span>Showing {processedRows.length} of {total} transactions</span>
        <span>Reconciliation Run: Audited</span>
      </div>
    </div>
  );
};
