export interface TierInfo {
  label: string;
  count: number;
  pct: number;
  color: string;
}

export interface OverviewTiers {
  exact: TierInfo;
  fuzzy: TierInfo;
  batch: TierInfo;
  aiResolved: TierInfo;
  unresolved: TierInfo;
}

export interface OverviewStats {
  totalTransactions: number;
  avgResolutionTime: string;
  exceptionsThisWeek: number;
}

export interface EvaluationInfo {
  hasEvaluation: boolean;
  total: number;
  correct: number;
  wrong: number;
  accuracy: number;
  expectedMatches: number;
  matched: number;
  matchCoverage: number;
  expectedExceptions: number;
  detectedExceptions: number;
  exceptionDetection: number;
  falsePositives: number;
  falseNegatives: number;
}

export interface OverviewResponse {
  status: 'idle' | 'populated' | string;
  has_run: boolean;
  lastUpdated?: string;
  syncDescription?: string;
  reconciliationRate?: number;
  velocity?: number;
  velocityDelta?: number;
  matchedCount?: number;
  deterministicCount?: number;
  aiResolvedCount?: number;
  totalCount?: number;
  needReview?: number;
  processedVolume?: string;
  batchId?: string;
  tiers?: OverviewTiers;
  stats?: OverviewStats;
  evaluation?: EvaluationInfo;
}

export interface LabelVal {
  label: string;
  value: string;
}

export interface DetailPanel {
  header: string;
  timestamp: string;
  rows: LabelVal[];
}

export interface Chip {
  icon: string;
  text: string;
}

export interface ExceptionDetail {
  left: DetailPanel;
  right: DetailPanel;
  chips: Chip[];
  aiSuggestion: string;
}

export interface ExceptionRow {
  id: number;
  orderId: string;
  settlementId: string;
  tier: string;
  tierColor: string;
  reviewType?: string;
  label: string;
  description: string;
  amount: string;
  delta: string | null;
  deltaNote: string | null;
  deltaNoteColor: string | null;
  expanded: boolean;
  detail?: ExceptionDetail;
}

export interface ExceptionsResponse {
  status: 'idle' | 'populated' | string;
  has_run: boolean;
  disputedVolume: string;
  criticalCount: number;
  openCount: number;
  fuzzyCount?: number;
  unresolvedCount?: number;
  rows: ExceptionRow[];
}

export interface AuditRow {
  orderId: string;
  settlementId: string;
  tier: string;
  tierColor: string;
  bankRef: string;
  amount: string;
  amountDiff: string;
  amountDiffColor: string | null;
  dateDiff: string;
  dateDiffColor: string | null;
  reason: string;
  reasonHighlight: unknown;
}

export interface AuditTrailResponse {
  status: 'idle' | 'populated' | string;
  has_run: boolean;
  reconciledVolume: string;
  parityIndex: number;
  reconciliationRate?: number;
  exactCount: number;
  fuzzyCount: number;
  batchCount?: number;
  aiCount: number;
  deterministicCount?: number;
  unresolvedCount: number;
  reconciledCount?: number;
  totalCount: number;
  rows: AuditRow[];
  evaluation?: EvaluationInfo;
}

export interface UploadResponse {
  rows: number;
  preview: Array<Record<string, unknown>>;
  warnings: string[];
  expectedHeaders?: string[];
  foundHeaders?: string[];
}

export interface RunResponse {
  summary: OverviewResponse;
  llmSkipped: boolean;
  message: string;
}

export interface ResetResponse {
  status: string;
  has_run?: boolean;
  message: string;
}
