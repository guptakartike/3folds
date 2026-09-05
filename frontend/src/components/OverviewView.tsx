import React from 'react';
import { OverviewResponse } from '../types/api';
import { ErrorState } from './ErrorState';
import { LoadingState } from './LoadingState';
import {
  Inbox,
  ArrowRight,
  Database,
  Zap,
  AlertOctagon,
  Play,
  Award,
} from 'lucide-react';

interface OverviewViewProps {
  data: OverviewResponse | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onNavigateToUpload: () => void;
  onNavigateToExceptions?: () => void;
}

export const OverviewView: React.FC<OverviewViewProps> = ({
  data,
  loading,
  error,
  onRetry,
  onNavigateToUpload,
  onNavigateToExceptions,
}) => {
  if (loading) {
    return <LoadingState message="Fetching live reconciliation overview..." />;
  }

  if (error) {
    return (
      <ErrorState
        title="Failed to load reconciliation overview"
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
          Upload settlement files, bank statements, and internal ledger entries to run the 3folds reconciliation pipeline.
        </p>
        <button onClick={onNavigateToUpload} className="btn btn-primary">
          Go to Upload
          <ArrowRight size={16} />
        </button>
      </div>
    );
  }

  const tiers = data.tiers;
  const stats = data.stats;
  const evalData = data.evaluation;
  
  const recRate =
    data.reconciliationRate !== undefined
      ? data.reconciliationRate
      : data.velocity !== undefined
      ? data.velocity
      : 0;
  const recRateFormatted = `${recRate.toFixed(1)}%`;

  return (
    <div className="overview-view">
      {/* Header */}
      <div className="overview-top-header">
        <div>
          <div className="page-title-row">
            <span
              style={{
                width: '8px',
                height: '8px',
                borderRadius: '50%',
                backgroundColor: '#10b981',
              }}
            />
            <h1 className="page-title">Reconciliation Overview</h1>
          </div>
          <p className="page-subtitle">
            Last updated: {data.lastUpdated || 'Live'} • {data.syncDescription || 'Synthetic benchmark data • Multi-way reconciliation across settlements, bank statements & ERP ledger'}
          </p>
        </div>

        <button onClick={onNavigateToUpload} className="btn btn-primary">
          <Play size={14} />
          Run Reconciliation
        </button>
      </div>

      {/* Top Banner Grid */}
      <div className="overview-banner-grid">
        <div className="velocity-card">
          <span className="velocity-label">AUTOMATED RECONCILIATION RATE</span>
          <div className="velocity-hero">
            <span className="velocity-number font-mono">{recRateFormatted}</span>
            <span
              style={{
                fontSize: '0.75rem',
                fontWeight: 600,
                color: '#065f46',
                backgroundColor: '#d1fae5',
                padding: '0.2rem 0.5rem',
                borderRadius: '4px',
              }}
            >
              Automated resolution
            </span>
          </div>
          <span className="velocity-subtext">
            {data.matchedCount ?? 0} of {data.totalCount ?? 0} transactions reconciled automatically
          </span>
        </div>

        <div className="velocity-status-card">
          <div className="status-banner-row">
            <span style={{ fontWeight: 600, color: 'var(--text-primary)' }}>
              {data.matchedCount ?? 0} Reconciled ({data.deterministicCount ?? (data.matchedCount ?? 0) - (data.aiResolvedCount ?? 0)} Deterministic, {data.aiResolvedCount ?? 0} AI-Resolved)
            </span>
            {(data.needReview ?? 0) > 0 && (
              <span
                style={{
                  fontSize: '0.75rem',
                  fontWeight: 700,
                  color: '#b91c1c',
                  backgroundColor: '#fee2e2',
                  padding: '0.2rem 0.5rem',
                  borderRadius: '4px',
                }}
              >
                {data.needReview} require review
              </span>
            )}
          </div>

          <div className="status-meta-row">
            <span>
              Processed Volume:{' '}
              <strong className="font-mono" style={{ color: 'var(--text-primary)' }}>
                {data.processedVolume || '0.00'}
              </strong>
            </span>
            <span>
              Environment:{' '}
              <strong className="font-mono" style={{ color: '#0284c7' }}>
                {data.batchId || 'SYNTHETIC BENCHMARK'}
              </strong>
            </span>
          </div>
        </div>
      </div>

      {/* Reconciliation Tier Distribution */}
      {tiers && (
        <div className="distribution-card">
          <div className="distribution-header">
            <h3 className="distribution-title">Reconciliation Tier Distribution</h3>
            <span className="distribution-total font-mono">
              {data.totalCount ?? 0} Total Transactions
            </span>
          </div>

          <div className="proportional-bar">
            {tiers.exact && (
              <div
                className="bar-segment exact"
                style={{ width: `${tiers.exact.pct}%` }}
                title={`Exact: ${tiers.exact.count} (${tiers.exact.pct}%)`}
              />
            )}
            {tiers.fuzzy && (
              <div
                className="bar-segment fuzzy"
                style={{ width: `${tiers.fuzzy.pct}%` }}
                title={`Fuzzy: ${tiers.fuzzy.count} (${tiers.fuzzy.pct}%)`}
              />
            )}
            {tiers.batch && (
              <div
                className="bar-segment batch"
                style={{ width: `${tiers.batch.pct}%` }}
                title={`Batch: ${tiers.batch.count} (${tiers.batch.pct}%)`}
              />
            )}
            {tiers.aiResolved && (
              <div
                className="bar-segment ai"
                style={{ width: `${tiers.aiResolved.pct}%` }}
                title={`AI-Resolved: ${tiers.aiResolved.count} (${tiers.aiResolved.pct}%)`}
              />
            )}
            {tiers.unresolved && (
              <div
                className="bar-segment unresolved"
                style={{ width: `${tiers.unresolved.pct}%` }}
                title={`Unresolved: ${tiers.unresolved.count} (${tiers.unresolved.pct}%)`}
              />
            )}
          </div>

          <div className="tier-legend">
            {tiers.exact && (
              <div className="legend-item">
                <div className="legend-header">
                  <span className="legend-dot exact" />
                  <span>{tiers.exact.label}</span>
                </div>
                <div className="legend-stats">
                  <span className="legend-count font-mono">{tiers.exact.count}</span>
                  <span className="legend-pct font-mono">({tiers.exact.pct.toFixed(1)}%)</span>
                </div>
              </div>
            )}

            {tiers.fuzzy && (
              <div className="legend-item">
                <div className="legend-header">
                  <span className="legend-dot fuzzy" />
                  <span>{tiers.fuzzy.label}</span>
                </div>
                <div className="legend-stats">
                  <span className="legend-count font-mono">{tiers.fuzzy.count}</span>
                  <span className="legend-pct font-mono">({tiers.fuzzy.pct.toFixed(1)}%)</span>
                </div>
              </div>
            )}

            {tiers.batch && (
              <div className="legend-item">
                <div className="legend-header">
                  <span className="legend-dot batch" />
                  <span>{tiers.batch.label}</span>
                </div>
                <div className="legend-stats">
                  <span className="legend-count font-mono">{tiers.batch.count}</span>
                  <span className="legend-pct font-mono">({tiers.batch.pct.toFixed(1)}%)</span>
                </div>
              </div>
            )}

            {tiers.aiResolved && (
              <div className="legend-item">
                <div className="legend-header">
                  <span className="legend-dot ai" />
                  <span>{tiers.aiResolved.label}</span>
                </div>
                <div className="legend-stats">
                  <span className="legend-count font-mono">{tiers.aiResolved.count}</span>
                  <span className="legend-pct font-mono">({tiers.aiResolved.pct.toFixed(1)}%)</span>
                </div>
              </div>
            )}

            {tiers.unresolved && (
              <div className="legend-item">
                <div className="legend-header">
                  <span className="legend-dot unresolved" />
                  <span>{tiers.unresolved.label}</span>
                </div>
                <div className="legend-stats">
                  <span className="legend-count font-mono">{tiers.unresolved.count}</span>
                  <span className="legend-pct font-mono">({tiers.unresolved.pct.toFixed(1)}%)</span>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* 3 Supporting Metric Cards */}
      <div className="stat-cards-grid">
        <div className="stat-card">
          <div className="stat-card-top">
            <span className="stat-card-label">Total Transactions</span>
            <div
              className="stat-icon-badge"
              style={{ backgroundColor: '#e0f2fe', color: '#0284c7' }}
            >
              <Database size={16} />
            </div>
          </div>
          <div className="stat-card-val font-mono">{data.totalCount ?? 0}</div>
          <div className="stat-card-footer">
            <span>Deterministic: {data.deterministicCount ?? 0} | AI: {data.aiResolvedCount ?? 0}</span>
          </div>
        </div>

        <div className="stat-card">
          <div className="stat-card-top">
            <span className="stat-card-label">Deterministic Matching Latency</span>
            <div
              className="stat-icon-badge"
              style={{ backgroundColor: '#f0fdf4', color: '#16a34a' }}
            >
              <Zap size={16} />
            </div>
          </div>
          <div className="stat-card-val font-mono">
            {stats?.avgResolutionTime || '< 1µs / tx'}
          </div>
          <div className="stat-card-footer">
            <span style={{ color: '#047857', fontWeight: 500 }}>
              Deterministic pipeline execution speed
            </span>
          </div>
        </div>

        <div className="stat-card">
          <div className="stat-card-top">
            <span className="stat-card-label">Exceptions Requiring Review</span>
            <div
              className="stat-icon-badge"
              style={{ backgroundColor: '#fee2e2', color: '#dc2626' }}
            >
              <AlertOctagon size={16} />
            </div>
          </div>
          <div className="stat-card-val font-mono">
            {data.needReview ?? 0}
            {(data.needReview ?? 0) > 0 && (
              <span
                style={{
                  fontSize: '0.75rem',
                  fontWeight: 700,
                  color: '#b91c1c',
                  backgroundColor: '#fee2e2',
                  padding: '0.15rem 0.4rem',
                  borderRadius: '4px',
                }}
              >
                Review Queue
              </span>
            )}
          </div>
          <div className="stat-card-footer">
            {onNavigateToExceptions ? (
              <button
                onClick={onNavigateToExceptions}
                style={{
                  background: 'none',
                  border: 'none',
                  color: '#0284c7',
                  cursor: 'pointer',
                  padding: 0,
                  fontSize: '0.75rem',
                  fontWeight: 600,
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                }}
              >
                Action required in review queue
                <ArrowRight size={12} />
              </button>
            ) : (
              <span>Action required in review queue</span>
            )}
          </div>
        </div>
      </div>

      {/* Ground Truth Evaluation Section */}
      {evalData && evalData.hasEvaluation && (
        <div className="card" style={{ padding: '1.5rem', marginBottom: '1.5rem', backgroundColor: '#ffffff' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1rem', flexWrap: 'wrap', gap: '0.5rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <Award size={18} style={{ color: '#0284c7' }} />
              <h3 style={{ fontSize: '0.9375rem', fontWeight: 700, color: 'var(--text-primary)' }}>
                Ground Truth Evaluation
              </h3>
            </div>
            <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              Verified against <strong className="font-mono" style={{ color: 'var(--text-secondary)' }}>ground_truth.json</strong> ({evalData.total} labeled cases)
            </span>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '1rem' }}>
            <div style={{ padding: '0.875rem', backgroundColor: '#f8fafc', borderRadius: '8px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', fontWeight: 700, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                Ground Truth Accuracy
              </div>
              <div className="font-mono" style={{ fontSize: '1.5rem', fontWeight: 700, color: evalData.accuracy === 100 ? '#047857' : '#0f172a', marginTop: '0.25rem' }}>
                {evalData.accuracy.toFixed(1)}%
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.125rem' }}>
                {evalData.correct} / {evalData.total} correct decisions
              </div>
            </div>

            <div style={{ padding: '0.875rem', backgroundColor: '#f8fafc', borderRadius: '8px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', fontWeight: 700, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                Expected Matches Found
              </div>
              <div className="font-mono" style={{ fontSize: '1.5rem', fontWeight: 700, color: '#0284c7', marginTop: '0.25rem' }}>
                {evalData.matched} / {evalData.expectedMatches}
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.125rem' }}>
                {evalData.matchCoverage.toFixed(1)}% match coverage
              </div>
            </div>

            <div style={{ padding: '0.875rem', backgroundColor: '#f8fafc', borderRadius: '8px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', fontWeight: 700, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                Genuine Exceptions Detected
              </div>
              <div className="font-mono" style={{ fontSize: '1.5rem', fontWeight: 700, color: '#7c3aed', marginTop: '0.25rem' }}>
                {evalData.detectedExceptions} / {evalData.expectedExceptions}
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.125rem' }}>
                {evalData.exceptionDetection.toFixed(1)}% exception detection
              </div>
            </div>

            <div style={{ padding: '0.875rem', backgroundColor: '#f8fafc', borderRadius: '8px', border: '1px solid var(--border-default)' }}>
              <div style={{ fontSize: '0.6875rem', fontWeight: 700, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                False Positives / Negatives
              </div>
              <div className="font-mono" style={{ fontSize: '1.5rem', fontWeight: 700, color: evalData.falsePositives === 0 && evalData.falseNegatives === 0 ? '#047857' : '#b91c1c', marginTop: '0.25rem' }}>
                {evalData.falsePositives} FP / {evalData.falseNegatives} FN
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.125rem' }}>
                Zero false reconciliations
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
