import React from 'react';
import { Activity, AlertTriangle, FileText, UploadCloud } from 'lucide-react';

export type TabKey = 'overview' | 'exceptions' | 'audit' | 'upload';

interface NavigationProps {
  activeTab: TabKey;
  onSelectTab: (tab: TabKey) => void;
  unresolvedCount: number;
  hasRun: boolean;
}

export const Navigation: React.FC<NavigationProps> = ({
  activeTab,
  onSelectTab,
  unresolvedCount,
  hasRun,
}) => {
  return (
    <nav className="navbar">
      <div className="navbar-inner">
        <div className="navbar-left">
          <div className="navbar-brand" onClick={() => onSelectTab('overview')}>
            {/* Authentic 3folds 3-stripe logo */}
            <svg
              className="navbar-logo-svg"
              viewBox="0 0 160 120"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
            >
              {/* Top stripe - Deep Petrol Blue */}
              <rect x="0" y="8" width="150" height="28" rx="14" fill="#0c5369" />
              {/* Middle stripe - Ocean Blue */}
              <rect x="18" y="46" width="132" height="28" rx="14" fill="#08758f" />
              {/* Bottom stripe - Emerald Green */}
              <rect x="36" y="84" width="114" height="28" rx="14" fill="#00b87a" />
            </svg>
            <span className="navbar-title">3folds</span>
          </div>

          <div className="navbar-tabs">
            <button
              className={`nav-tab ${activeTab === 'overview' ? 'active' : ''}`}
              onClick={() => onSelectTab('overview')}
            >
              <Activity size={15} />
              Overview
            </button>

            <button
              className={`nav-tab ${activeTab === 'exceptions' ? 'active' : ''}`}
              onClick={() => onSelectTab('exceptions')}
            >
              <AlertTriangle size={15} />
              Exceptions
              {hasRun && unresolvedCount > 0 && (
                <span className="nav-badge">{unresolvedCount}</span>
              )}
            </button>

            <button
              className={`nav-tab ${activeTab === 'audit' ? 'active' : ''}`}
              onClick={() => onSelectTab('audit')}
            >
              <FileText size={15} />
              Audit Trail
            </button>

            <button
              className={`nav-tab ${activeTab === 'upload' ? 'active' : ''}`}
              onClick={() => onSelectTab('upload')}
            >
              <UploadCloud size={15} />
              Upload
            </button>
          </div>
        </div>

        <div className="navbar-right">
          <div className="status-pill">
            <span className="status-dot" />
            <span>SYNTHETIC BENCHMARK</span>
          </div>

          <span
            style={{
              fontSize: '0.75rem',
              fontWeight: 600,
              padding: '0.25rem 0.5rem',
              backgroundColor: '#f1f5f9',
              borderRadius: '4px',
              color: '#475569',
            }}
          >
            DEMO ENVIRONMENT
          </span>
        </div>
      </div>
    </nav>
  );
};
