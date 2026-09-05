import React, { useState } from 'react';
import { ExceptionsResponse, ExceptionRow } from '../types/api';
import { ErrorState } from './ErrorState';
import { LoadingState } from './LoadingState';
import {
  Search,
  ChevronDown,
  CheckCircle2,
  Inbox,
  ArrowRight,
  Landmark,
  Calendar,
  Layers,
  CheckSquare,
  Square,
  AlertTriangle,
  GitPullRequest,
} from 'lucide-react';

interface ExceptionsViewProps {
  data: ExceptionsResponse | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onNavigateToUpload: () => void;
}

type ReviewQueueFilter = 'ALL' | 'FUZZY' | 'UNRESOLVED';

export const ExceptionsView: React.FC<ExceptionsViewProps> = ({
  data,
  loading,
  error,
  onRetry,
  onNavigateToUpload,
}) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [filter, setFilter] = useState<ReviewQueueFilter>('ALL');
  const [expandedIds, setExpandedIds] = useState<Record<number, boolean>>({ 1: true });
  const [selectedIds, setSelectedIds] = useState<Record<number, boolean>>({});

  if (loading) {
    return <LoadingState message="Fetching exception review queue..." />;
  }

  if (error) {
    return (
      <ErrorState
        title="Failed to load exception queue"
        message={error}
        onRetry={onRetry}
      />
    );
  }

  if (!data || !data.has_run || data.status === 'idle') {
    return (
      <div className="idle-container">
        <Inbox className="idle-icon" />
        <h2 className="idle-title">No reconciliation run yet</h2>
        <p className="idle-description">
          Run reconciliation from the Upload tab to populate the review queue.
        </p>
        <button onClick={onNavigateToUpload} className="btn btn-primary">
          Go to Upload
          <ArrowRight size={16} />
        </button>
      </div>
    );
  }

  if (!data.rows || data.rows.length === 0) {
    return (
      <div className="idle-container">
        <CheckCircle2 className="idle-icon" style={{ color: '#16a34a' }} />
        <h2 className="idle-title">Zero review items</h2>
        <p className="idle-description">
          All transactions were successfully reconciled with exact deterministic matches. No items require review.
        </p>
      </div>
    );
  }

  const fuzzyCount = data.fuzzyCount ?? data.rows.filter(r => r.reviewType === 'fuzzy' || r.tier.includes('FUZZY')).length;
  const unresolvedCount = data.unresolvedCount ?? data.rows.filter(r => r.reviewType === 'unresolved' || r.tier.includes('UNRESOLVED')).length;
  const totalReviewItems = data.openCount || data.rows.length;

  const toggleExpand = (id: number) => {
    setExpandedIds((prev) => ({
      ...prev,
      [id]: !prev[id],
    }));
  };

  const toggleSelect = (id: number, e: React.MouseEvent) => {
    e.stopPropagation();
    setSelectedIds((prev) => ({
      ...prev,
      [id]: !prev[id],
    }));
  };

  const filteredRows = data.rows.filter((row: ExceptionRow) => {
    // Filter by category
    if (filter === 'FUZZY') {
      const isFuzzy = row.reviewType === 'fuzzy' || row.tier.toUpperCase().includes('FUZZY');
      if (!isFuzzy) return false;
    } else if (filter === 'UNRESOLVED') {
      const isUnresolved = row.reviewType === 'unresolved' || row.tier.toUpperCase().includes('UNRESOLVED');
      if (!isUnresolved) return false;
    }

    // Filter by search
    const term = searchTerm.toLowerCase().trim();
    if (!term) return true;
    return (
      row.orderId.toLowerCase().includes(term) ||
      row.settlementId.toLowerCase().includes(term) ||
      row.description.toLowerCase().includes(term)
    );
  });

  return (
    <div className="exceptions-view">
      {/* Header */}
      <div style={{ marginBottom: '1.5rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '1rem', marginBottom: '0.25rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <h1 className="page-title">{totalReviewItems} items requiring review</h1>
            <span
              style={{
                fontSize: '0.75rem',
                fontWeight: 700,
                backgroundColor: '#fee2e2',
                color: '#b91c1c',
                padding: '0.2rem 0.6rem',
                borderRadius: '9999px',
                display: 'inline-flex',
                alignItems: 'center',
                gap: '4px',
              }}
            >
              <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: '#b91c1c' }} />
              Review Queue
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '1.25rem', fontSize: '0.8125rem' }}>
            <div>
              <span style={{ color: 'var(--text-muted)', textTransform: 'uppercase', fontSize: '0.6875rem', fontWeight: 700 }}>
                Review Volume:{' '}
              </span>
              <strong className="font-mono" style={{ color: 'var(--text-primary)' }}>
                {data.disputedVolume}
              </strong>
            </div>

            {data.criticalCount > 0 && (
              <div>
                <span style={{ color: 'var(--text-muted)', textTransform: 'uppercase', fontSize: '0.6875rem', fontWeight: 700 }}>
                  High Priority:{' '}
                </span>
                <strong className="font-mono" style={{ color: '#b91c1c' }}>
                  {data.criticalCount} Unresolved
                </strong>
              </div>
            )}
          </div>
        </div>
        <p className="page-subtitle">
          Separate queues for fuzzy matches within tolerance requiring verification and genuine exceptions missing bank evidence.
        </p>
      </div>

      {/* Review Queue Split Navigation & Search */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '1rem', marginBottom: '1.25rem' }}>
        <div className="filter-group">
          <button
            className={`filter-btn ${filter === 'ALL' ? 'active' : ''}`}
            onClick={() => setFilter('ALL')}
          >
            All Items ({totalReviewItems})
          </button>
          <button
            className={`filter-btn ${filter === 'FUZZY' ? 'active' : ''}`}
            onClick={() => setFilter('FUZZY')}
            style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}
          >
            <GitPullRequest size={13} />
            Fuzzy Reviews ({fuzzyCount})
          </button>
          <button
            className={`filter-btn ${filter === 'UNRESOLVED' ? 'active' : ''}`}
            onClick={() => setFilter('UNRESOLVED')}
            style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}
          >
            <AlertTriangle size={13} />
            Unresolved Exceptions ({unresolvedCount})
          </button>
        </div>

        <div style={{ maxWidth: '320px', width: '100%', position: 'relative' }}>
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
            placeholder="Search by order ID or settlement ID..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </div>

      {/* Exceptions List */}
      <div className="exceptions-list">
        {filteredRows.map((row) => {
          const isExpanded = !!expandedIds[row.id];
          const isSelected = !!selectedIds[row.id];
          const isFuzzy = row.reviewType === 'fuzzy' || row.tier.toUpperCase().includes('FUZZY');
          const badgeClass = isFuzzy ? 'tier-badge tier-fuzzy' : 'tier-badge tier-unresolved';

          return (
            <div key={row.id} className={`exception-item ${isExpanded ? 'expanded' : ''}`}>
              <div className="exception-summary" onClick={() => toggleExpand(row.id)}>
                <div className="exception-main">
                  <div
                    onClick={(e) => toggleSelect(row.id, e)}
                    style={{ color: isSelected ? '#0284c7' : 'var(--text-subtle)', cursor: 'pointer', display: 'flex', alignItems: 'center' }}
                  >
                    {isSelected ? <CheckSquare size={18} /> : <Square size={18} />}
                  </div>

                  <div className="exception-ids">
                    <span className="exception-order-id font-mono">{row.orderId}</span>
                    <span className="exception-settlement-id font-mono">{row.settlementId}</span>
                  </div>

                  <span className={badgeClass}>{row.tier}</span>

                  <span className="exception-reason">
                    <strong>{row.label} </strong>
                    {row.description}
                  </span>
                </div>

                <div className="exception-meta">
                  <div>
                    <div className="exception-amount font-mono">{row.amount}</div>
                    {row.delta && <div className="exception-delta font-mono">{row.delta}</div>}
                  </div>

                  <ChevronDown size={18} className="exception-expand-btn" />
                </div>
              </div>

              {isExpanded && row.detail && (
                <div className="exception-detail-panel">
                  <div className="payload-card">
                    <div className="payload-title">
                      <span>{row.detail.left.header}</span>
                      <span className="font-mono">{row.detail.left.timestamp}</span>
                    </div>
                    {row.detail.left.rows.map((r, idx) => (
                      <div key={idx} className="payload-row">
                        <span className="payload-label">{r.label}</span>
                        <span className="payload-val font-mono">{r.value}</span>
                      </div>
                    ))}
                  </div>

                  <div className="payload-card">
                    <div className="payload-title">
                      <span>{row.detail.right.header}</span>
                      <span className="font-mono">{row.detail.right.timestamp}</span>
                    </div>
                    {row.detail.right.rows.map((r, idx) => (
                      <div key={idx} className="payload-row">
                        <span className="payload-label">{r.label}</span>
                        <span className="payload-val font-mono">{r.value}</span>
                      </div>
                    ))}

                    {row.detail.chips && row.detail.chips.length > 0 && (
                      <div className="chips-row">
                        {row.detail.chips.map((chip, cIdx) => (
                          <span key={cIdx} className="info-chip font-mono">
                            {chip.icon === 'amount' && <Layers size={12} />}
                            {chip.icon === 'time' && <Calendar size={12} />}
                            {chip.icon === 'bank' && <Landmark size={12} />}
                            {chip.text}
                          </span>
                        ))}
                      </div>
                    )}

                    <div style={{ marginTop: '0.75rem', padding: '0.5rem 0.75rem', backgroundColor: '#f0fdf4', border: '1px solid #bbf7d0', borderRadius: '4px', fontSize: '0.75rem', color: '#166534' }}>
                      <strong>Audit Action: </strong>
                      {isFuzzy
                        ? 'Variance is within configured tolerance parameters (₹2.00 / 72h). Sign off to confirm match.'
                        : 'No bank settlement record found for this transaction ID. Review pending clearing or contact payment gateway.'}
                    </div>
                  </div>
                </div>
              )}
            </div>
          );
        })}

        {filteredRows.length === 0 && (
          <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-muted)', backgroundColor: '#ffffff', borderRadius: '8px', border: '1px solid var(--border-default)' }}>
            No review items matched the active search or category filter.
          </div>
        )}
      </div>
    </div>
  );
};
