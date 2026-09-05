import React, { useState, useEffect, useCallback } from 'react';
import { api } from './services/api';
import {
  OverviewResponse,
  ExceptionsResponse,
  AuditTrailResponse,
  RunResponse,
} from './types/api';
import { Navigation, TabKey } from './components/Navigation';
import { OverviewView } from './components/OverviewView';
import { ExceptionsView } from './components/ExceptionsView';
import { AuditTrailView } from './components/AuditTrailView';
import { UploadView } from './components/UploadView';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<TabKey>('overview');

  const [overviewData, setOverviewData] = useState<OverviewResponse | null>(null);
  const [overviewLoading, setOverviewLoading] = useState(true);
  const [overviewError, setOverviewError] = useState<string | null>(null);

  const [exceptionsData, setExceptionsData] = useState<ExceptionsResponse | null>(null);
  const [exceptionsLoading, setExceptionsLoading] = useState(false);
  const [exceptionsError, setExceptionsError] = useState<string | null>(null);

  const [auditData, setAuditData] = useState<AuditTrailResponse | null>(null);
  const [auditLoading, setAuditLoading] = useState(false);
  const [auditError, setAuditError] = useState<string | null>(null);

  const fetchOverview = useCallback(async () => {
    setOverviewLoading(true);
    setOverviewError(null);
    try {
      const data = await api.getOverview();
      setOverviewData(data);
      setOverviewLoading(false);
    } catch (err: unknown) {
      setOverviewLoading(false);
      const msg = err instanceof Error ? err.message : 'Failed to reach reconciliation backend';
      setOverviewError(msg);
    }
  }, []);

  const fetchExceptions = useCallback(async () => {
    setExceptionsLoading(true);
    setExceptionsError(null);
    try {
      const data = await api.getExceptions();
      setExceptionsData(data);
      setExceptionsLoading(false);
    } catch (err: unknown) {
      setExceptionsLoading(false);
      const msg = err instanceof Error ? err.message : 'Failed to reach reconciliation backend';
      setExceptionsError(msg);
    }
  }, []);

  const fetchAuditTrail = useCallback(async () => {
    setAuditLoading(true);
    setAuditError(null);
    try {
      const data = await api.getAuditTrail();
      setAuditData(data);
      setAuditLoading(false);
    } catch (err: unknown) {
      setAuditLoading(false);
      const msg = err instanceof Error ? err.message : 'Failed to reach reconciliation backend';
      setAuditError(msg);
    }
  }, []);

  // Initial fetch for overview and exceptions (to populate unresolved badge)
  useEffect(() => {
    fetchOverview();
    fetchExceptions();
  }, [fetchOverview, fetchExceptions]);

  // Lazy fetch tabs when switched
  useEffect(() => {
    if (activeTab === 'overview') {
      fetchOverview();
    } else if (activeTab === 'exceptions') {
      fetchExceptions();
    } else if (activeTab === 'audit') {
      fetchAuditTrail();
    }
  }, [activeTab, fetchOverview, fetchExceptions, fetchAuditTrail]);

  const handleRunSuccess = (res: RunResponse) => {
    setOverviewData(res.summary);
    fetchExceptions();
    fetchAuditTrail();
    setActiveTab('overview');
  };

  const handleResetSuccess = () => {
    fetchOverview();
    fetchExceptions();
    fetchAuditTrail();
  };

  const unresolvedCount =
    exceptionsData?.has_run && exceptionsData?.openCount !== undefined
      ? exceptionsData.openCount
      : overviewData?.has_run && overviewData?.needReview !== undefined
      ? overviewData.needReview
      : 0;

  const hasRun = !!overviewData?.has_run;

  return (
    <div className="app-container">
      <Navigation
        activeTab={activeTab}
        onSelectTab={setActiveTab}
        unresolvedCount={unresolvedCount}
        hasRun={hasRun}
      />

      <main className="main-content">
        {activeTab === 'overview' && (
          <OverviewView
            data={overviewData}
            loading={overviewLoading}
            error={overviewError}
            onRetry={fetchOverview}
            onNavigateToUpload={() => setActiveTab('upload')}
            onNavigateToExceptions={() => setActiveTab('exceptions')}
          />
        )}

        {activeTab === 'exceptions' && (
          <ExceptionsView
            data={exceptionsData}
            loading={exceptionsLoading}
            error={exceptionsError}
            onRetry={fetchExceptions}
            onNavigateToUpload={() => setActiveTab('upload')}
          />
        )}

        {activeTab === 'audit' && (
          <AuditTrailView
            data={auditData}
            loading={auditLoading}
            error={auditError}
            onRetry={fetchAuditTrail}
            onNavigateToUpload={() => setActiveTab('upload')}
          />
        )}

        {activeTab === 'upload' && (
          <UploadView
            onRunSuccess={handleRunSuccess}
            onResetSuccess={handleResetSuccess}
          />
        )}
      </main>

      <footer className="app-footer">
        <div className="footer-inner">
          <span>&copy; 2026 3folds Automated Reconciliation Engine. Continuous synchronization active.</span>
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
            <span className="status-dot" />
            Deterministic Latency: Sub-millisecond &bull; Multi-Way Gateway &amp; Bank Ledger
          </span>
        </div>
      </footer>
    </div>
  );
};
