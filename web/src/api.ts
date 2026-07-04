export interface CategorySummary {
  code: string
  label: string
  newApiType: number
  totalKeys: number
  activeKeys: number
  inventoryKeys: number
}

export interface DashboardSummary {
  totalKeys: number
  activeKeys: number
  inventoryKeys: number
  disabledKeys: number
  totalUsageUsd: number
  categories: CategorySummary[]
}

export interface ChannelGroup {
  categoryCode: string
  tag: string
  keyCount: number
  activeCount: number
  disabledCount: number
  usedUsd: number
}

export interface HealthCheckRecord {
  id: number
  apiKeyId: number
  categoryCode: string
  keyHint: string
  status: string
  latencyMs: number
  errorCode: string
  errorMessage: string
  checkedAt: string
}

export interface HealthRunResult {
  apiKeyId: number
  newApiChannelId: number
  success: boolean
  latencyMs: number
  message: string
  errorCode?: string
  autoDisabled: boolean
  disableError?: string
  durationSeconds: number
}

export interface UsageDaySummary {
  statDate: string
  quota: number
  usd: number
}

export interface UsageCategorySummary {
  categoryCode: string
  categoryLabel: string
  quota: number
  usd: number
}

export interface UsageChannelSummary {
  apiKeyId: number
  targetCode: string
  newApiChannelId: number
  categoryCode: string
  keyHint: string
  tag: string
  quota: number
  usd: number
}

export interface UsageSummary {
  days: number
  totalQuota: number
  totalUsd: number
  byDay: UsageDaySummary[]
  categories: UsageCategorySummary[]
  channels: UsageChannelSummary[]
}

export interface UsageSyncResult {
  synced: number
  baseline: number
  missing: number
  totalDeltaQuota: number
  totalDeltaUsd: number
}

export interface WorkerRunRecord {
  id: number
  workerName: string
  status: string
  startedAt: string
  finishedAt: string
  durationMs: number
  errorMessage: string
}

export interface AuthUser {
  id: number
  username: string
  displayName: string
  role: string
}

export interface AuthState {
  authEnabled: boolean
  registrationEnabled: boolean
  authenticated: boolean
  user?: AuthUser
}

export interface AuditLogRecord {
  id: number
  actor: string
  action: string
  targetType: string
  targetId?: number
  detailJson: string
  createdAt: string
}

export interface ComponentHealth {
  status: string
  message: string
  latencyMs: number
}

export interface SystemHealth {
  status: string
  components: Record<string, ComponentHealth>
  newApiBase: string
  newApiUser: number
  serverTime: string
  staticDir: string
  autoMigrate: boolean
  worker: boolean
}

export interface TableStat {
  tableName: string
  rows: number
  dataMb: number
  indexMb: number
}

export interface WorkerStat {
  workerName: string
  totalRuns: number
  successRuns: number
  failedRuns: number
  successRate: number
  lastStatus: string
  lastFinishedAt?: string
}

export interface RecentError {
  source: string
  message: string
  createdAt: string
}

export interface OpsStatus {
  health: SystemHealth
  tableStats: TableStat[]
  workerStats: WorkerStat[]
  recentErrors: RecentError[]
  generatedAt: string
}

export interface KeyExportRecord {
  id: number
  categoryCode: string
  categoryLabel: string
  keyHint: string
  region: string
  baseUrl: string
  models: string[]
  tag: string
  expectedTpm: number
  status: string
  newApiChannelId?: number
  lastSyncStatus: string
  lastHealthStatus: string
  successCount: number
  errorCount: number
  lastError: string
  createdAt: string
}

export interface Category {
  code: string
  label: string
  newApiType: number
  defaultModels: string[]
  keyFormat: string
}

export interface AggregationTarget {
  code: string
  name: string
  baseUrl: string
  connectionMode?: AggregationTargetConnectionMode
  default: boolean
}

export type AggregationTargetConnectionMode = 'api' | 'new_api_reverse'

export interface AdminAggregationTarget extends AggregationTarget {
  enabled: boolean
  source: 'database' | 'env'
  hasToken: boolean
  reverseUsername?: string
  hasReversePassword: boolean
  connectionMode: AggregationTargetConnectionMode
  createdAt?: string
  updatedAt?: string
}

export interface AggregationTargetInput {
  code: string
  name: string
  baseUrl: string
  connectionMode: AggregationTargetConnectionMode
  token?: string
  reverseUsername?: string
  reversePassword?: string
  enabled: boolean
  default: boolean
}

export interface AggregationTargetContractCheckInput {
  code: string
  baseUrl: string
  token?: string
}

export interface AggregationTargetContractCheckResult {
  success: boolean
  usageCount: number
}

export interface AggregationTargetReverseAdminCheckInput {
  code: string
  baseUrl: string
  reverseUsername?: string
  reversePassword?: string
}

export interface AggregationTargetReverseAdminCheckResult {
  success: boolean
  userId: number
  role: number
  channelTotal: number
}

export interface KeyImportRequest {
  categoryCode: string
  endpointUrl: string
  rawText: string
  models: string[]
  expectedTpm: number
  note: string
}

export interface KeyImportResponse {
  batchId: number
  total: number
  imported: number
  duplicates: number
  failed: number
}

export interface APIKeyRecord {
  id: number
  categoryCode: string
  keyHint: string
  region: string
  baseUrl: string
  models: string[]
  tag: string
  note: string
  expectedTpm: number
  usageQuota30d: number
  usageUsd30d: number
  status: string
  newApiChannelId?: number
  lastSyncStatus: string
  lastHealthStatus: string
  successCount: number
  errorCount: number
  lastError: string
  createdAt: string
}

export interface APIKeyListResponse {
  items: APIKeyRecord[]
  total: number
}

export interface SyncEventRecord {
  id: number
  apiKeyId?: number
  action: string
  status: string
  requestPayload?: Record<string, unknown>
  response?: Record<string, unknown>
  errorMessage: string
  createdAt: string
}

export async function getDashboardSummary(): Promise<DashboardSummary> {
  return getJSON('/api/dashboard/summary')
}

export async function getCategories(): Promise<Category[]> {
  const response = await getJSON<{ items?: Category[] | null }>('/api/categories')
  return itemsOrEmpty(response.items)
}

export async function getAggregationTargets(): Promise<AggregationTarget[]> {
  const response = await getJSON<{ items?: AggregationTarget[] | null }>('/api/aggregation-targets')
  return itemsOrEmpty(response.items)
}

export async function listAdminAggregationTargets(): Promise<AdminAggregationTarget[]> {
  const response = await getJSON<{ items?: AdminAggregationTarget[] | null }>('/api/admin/aggregation-targets')
  return itemsOrEmpty(response.items)
}

export async function createAggregationTarget(payload: AggregationTargetInput): Promise<{ success: boolean }> {
  return postJSON('/api/admin/aggregation-targets', payload)
}

export async function updateAggregationTarget(code: string, payload: AggregationTargetInput): Promise<{ success: boolean }> {
  return putJSON(`/api/admin/aggregation-targets/${encodeURIComponent(code)}`, payload)
}

export async function deleteAggregationTarget(code: string): Promise<{ deleted: boolean }> {
  return deleteJSON(`/api/admin/aggregation-targets/${encodeURIComponent(code)}`)
}

export async function checkAggregationTargetContract(
  payload: AggregationTargetContractCheckInput,
): Promise<AggregationTargetContractCheckResult> {
  return postJSON('/api/admin/aggregation-targets/check-contract', payload)
}

export async function checkAggregationTargetReverseAdmin(
  payload: AggregationTargetReverseAdminCheckInput,
): Promise<AggregationTargetReverseAdminCheckResult> {
  return postJSON('/api/admin/aggregation-targets/check-reverse-admin', payload)
}

export async function getChannelGroups(): Promise<ChannelGroup[]> {
  const response = await getJSON<{ items?: ChannelGroup[] | null }>('/api/channels')
  return itemsOrEmpty(response.items)
}

export async function listHealthChecks(): Promise<HealthCheckRecord[]> {
  const response = await getJSON<{ items?: HealthCheckRecord[] | null }>('/api/health-checks?limit=50')
  return itemsOrEmpty(response.items)
}

export async function runHealthChecks(payload: {
  autoDisable: boolean
  limit?: number
}): Promise<HealthRunResult[]> {
  const response = await postJSON<{ items?: HealthRunResult[] | null }>('/api/health-checks/run', payload)
  return itemsOrEmpty(response.items)
}

export async function getUsageSummary(days = 30): Promise<UsageSummary> {
  return getJSON(`/api/usage/summary?days=${days}`)
}

export async function syncUsage(): Promise<UsageSyncResult> {
  return postJSON('/api/usage/sync', {})
}

export async function listWorkerRuns(): Promise<WorkerRunRecord[]> {
  const response = await getJSON<{ items?: WorkerRunRecord[] | null }>('/api/workers/runs?limit=30')
  return itemsOrEmpty(response.items)
}

export async function getSession(): Promise<AuthState> {
  return getJSON('/api/auth/me')
}

export async function login(payload: { username: string; password: string }): Promise<AuthState> {
  return postJSON('/api/auth/login', payload)
}

export async function register(payload: {
  username: string
  password: string
  displayName?: string
}): Promise<AuthState> {
  return postJSON('/api/auth/register', payload)
}

export async function logout(): Promise<{ success: boolean }> {
  return postJSON('/api/auth/logout', {})
}

export async function listAuditLogs(): Promise<AuditLogRecord[]> {
  const response = await getJSON<{ items?: AuditLogRecord[] | null }>('/api/audit/logs?limit=100')
  return itemsOrEmpty(response.items)
}

export async function getOpsStatus(): Promise<OpsStatus> {
  return getJSON('/api/ops/status')
}

export async function exportKeyInventory(): Promise<KeyExportRecord[]> {
  const response = await getJSON<{ items?: KeyExportRecord[] | null }>('/api/keys/export?limit=5000')
  return itemsOrEmpty(response.items)
}

export async function importKeys(payload: KeyImportRequest): Promise<KeyImportResponse> {
  return postJSON('/api/keys/import', payload)
}

export async function listAPIKeys(): Promise<APIKeyListResponse> {
  const response = await getJSON<Partial<APIKeyListResponse> & { items?: APIKeyRecord[] | null }>('/api/keys?limit=100')
  return {
    items: itemsOrEmpty(response.items),
    total: response.total ?? 0,
  }
}

export async function activateKey(id: number, payload: { targetCode: string }): Promise<{ newApiChannelId: number; targetCode: string }> {
  return postJSON(`/api/keys/${id}/activate`, payload)
}

export async function disableKey(id: number): Promise<{ disabled: boolean }> {
  return postJSON(`/api/keys/${id}/disable`, {})
}

export async function deleteKey(id: number): Promise<{ deleted: boolean }> {
  return deleteJSON(`/api/keys/${id}`)
}

export async function listSyncEvents(): Promise<SyncEventRecord[]> {
  const response = await getJSON<{ items?: SyncEventRecord[] | null }>('/api/sync/events?limit=20')
  return itemsOrEmpty(response.items)
}

function itemsOrEmpty<T>(items?: T[] | null): T[] {
  return Array.isArray(items) ? items : []
}

async function getJSON<T>(url: string): Promise<T> {
  const response = await fetch(url, { credentials: 'same-origin' })
  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }
  return response.json() as Promise<T>
}

async function postJSON<T>(url: string, payload: unknown): Promise<T> {
  const response = await fetch(url, {
    method: 'POST',
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }
  return response.json() as Promise<T>
}

async function putJSON<T>(url: string, payload: unknown): Promise<T> {
  const response = await fetch(url, {
    method: 'PUT',
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }
  return response.json() as Promise<T>
}

async function deleteJSON<T>(url: string): Promise<T> {
  const response = await fetch(url, {
    method: 'DELETE',
    credentials: 'same-origin',
  })
  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }
  return response.json() as Promise<T>
}

async function readErrorMessage(response: Response) {
  const text = await response.text()
  if (!text) {
    return `${response.status} ${response.statusText}`
  }
  try {
    const payload = JSON.parse(text) as { error?: string; message?: string }
    return payload.error || payload.message || text
  } catch {
    return text
  }
}
