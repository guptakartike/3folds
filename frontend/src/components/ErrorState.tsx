import React from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';

interface ErrorStateProps {
  title?: string;
  message: string;
  onRetry?: () => void;
}

export const ErrorState: React.FC<ErrorStateProps> = ({
  title = 'Unable to connect to backend',
  message,
  onRetry,
}) => {
  return (
    <div className="error-container">
      <AlertCircle className="error-icon" />
      <h3 className="error-title">{title}</h3>
      <p className="error-message">{message}</p>
      {onRetry && (
        <button onClick={onRetry} className="btn btn-secondary">
          <RefreshCw size={14} />
          Retry Request
        </button>
      )}
    </div>
  );
};
