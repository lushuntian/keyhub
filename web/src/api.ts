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

export interface UsageLogItem {
  createdAt: number
  type: number
  channelId: number
  modelName: string
  quota: number
  quotaUsd: number
  promptTokens: number
  completionTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  useTime: number
  firstResponseTimeMs: number
  isStream: boolean
  requestId?: string
  upstreamRequestId?: string
  content?: string
  streamStatus?: string
  streamEndReason?: string
}

export interface UsageLogStats {
  quota: number
  quotaUsd: number
  rpm: number
  tpm: number
}

export interface UsageLogPage {
  configured: boolean
  message?: string
  items: UsageLogItem[]
  total: number
  pageSize: number
  nextCursor?: string
  channels: number[]
  stats: UsageLogStats
}

export interface UsageLogConfiguration {
  configured: boolean
  message?: string
}

export interface UsageLogQuery {
  cursor?: string
  pageSize?: number
  type?: number
  modelName?: string
  startTimestamp?: number
  endTimestamp?: number
  channelId?: number
  requestId?: string
  upstreamRequestId?: string
}

export interface UsageSyncResult {
  synced: number
  baseline: number
  missing: number
  remoteStatusChecked?: number
  remoteStatusMissing?: number
  failedTargets?: number
  totalSyncedQuota?: number
  totalSyncedUsd?: number
  totalDeltaQuota: number
  totalDeltaUsd: number
  targetFailures?: Array<{
    connectionMode: AggregationTargetConnectionMode
    error: string
  }>
}

export interface ModelGap {
  id: number
  providerType: string
  modelName: string
  rpmGap: number
  tpmGap: number
  createdAt: string
  updatedAt: string
}

export interface ModelGapInput {
  providerType: string
  modelName: string
  rpmGap: number
  tpmGap: number
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

export interface AdminUser extends AuthUser {
  createdByUserId?: number
  createdByUsername?: string
  inheritsAggregationTargets: boolean
  disabled: boolean
  aggregationTargetCodes: string[]
  aggregationTargets: AdminUserAggregationTarget[]
  uploadedKeyCount: number
  totalUsageUsd: number
  lastLoginAt?: string
  createdAt: string
}

export interface AdminUserAggregationTarget {
  targetCode: string
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
  channelType: AggregationTargetChannelType
  models: string[]
  reverseGroupName?: string
  targetCount?: number
}

export type AggregationTargetConnectionMode = 'api' | 'new_api_reverse' | 'token_supply'
export type AggregationTargetChannelType =
  | 'anthropic'
  | 'aws_bedrock'
  | 'aws_bedrock_new'
  | 'openai'
  | 'azure_openai'
  | 'google_ai_studio'
export type AWSBedrockCredentialType = 'ak_sk' | 'api_key'

export interface AdminAggregationTarget extends AggregationTarget {
  enabled: boolean
  isSystemDefault: boolean
  source: 'database' | 'env'
  hasToken: boolean
  reverseUsername?: string
  hasReversePassword: boolean
  connectionMode: AggregationTargetConnectionMode
  createdAt?: string
  updatedAt?: string
}

export interface AggregationTargetInput {
  name: string
  baseUrl: string
  connectionMode: AggregationTargetConnectionMode
  channelType: AggregationTargetChannelType
  models: string[]
  token?: string
  reverseUsername?: string
  reverseGroupName?: string
  reversePassword?: string
  enabled: boolean
  isSystemDefault: boolean
}

export interface AggregationTargetContractCheckInput {
  code?: string
  baseUrl: string
  token?: string
}

export interface AggregationTargetContractCheckResult {
  success: boolean
  usageCount: number
}

export interface AggregationTargetReverseAdminCheckInput {
  code?: string
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

export interface AggregationTargetTokenSupplyCheckInput {
  code?: string
  baseUrl: string
  reverseUsername?: string
  reversePassword?: string
}

export interface AggregationTargetTokenSupplyCheckResult {
  success: boolean
  username: string
  purchaseProvider: string
  providerCount: number
  apiKeyCount: number
}

export interface KeyImportRequest {
  rawText: string
  channelTypes?: AggregationTargetChannelType[]
  awsCredentialType?: AWSBedrockCredentialType
  awsBedrockRegions?: string[]
  models?: string[]
  expectedTpm: number
}

export interface KeyImportResponse {
  batchId: number
  total: number
  imported: number
  duplicates: number
  failed: number
  importedKeyIds?: number[]
}

export interface APIKeyPushBinding {
  apiKeyId: number
  targetCode: string
  remoteChannelId: number
  status: string
  remoteStatus?: number
  remoteStatusCheckedAt?: string
}

export interface APIKeyRecord {
  id: number
  categoryCode: string
  awsCredentialType?: AWSBedrockCredentialType
  keyHint: string
  region: string
  baseUrl: string
  models: string[]
  tag: string
  note: string
  createdBy: string
  expectedTpm: number
  usageQuota30d: number
  usageUsd30d: number
  status: string
  newApiChannelId?: number
  pushBindings?: APIKeyPushBinding[]
  lastSyncStatus: string
  lastHealthStatus: string
  successCount: number
  errorCount: number
  lastError: string
  firstUsedAt?: string
  lastUsedAt?: string
  createdAt: string
}

export interface APIKeyListResponse {
  items: APIKeyRecord[]
  total: number
  totalUsageUsd: number
}

export interface APIKeyPlaintextResponse {
  id: number
  plaintext: string
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
  return getJSON('/dashboard/summary')
}

export async function getCategories(): Promise<Category[]> {
  const response = await getJSON<{ items?: Category[] | null }>('/categories')
  return itemsOrEmpty(response.items)
}

export async function getAggregationTargets(): Promise<AggregationTarget[]> {
  const response = await getJSON<{ items?: Array<Partial<AggregationTarget> & Pick<AggregationTarget, 'channelType'>> | null }>(
    '/aggregation-targets',
  )
  return itemsOrEmpty(response.items).map((target) => ({
    code: '',
    name: '',
    baseUrl: '',
    models: [],
    ...target,
  }))
}

export async function listAdminAggregationTargets(): Promise<AdminAggregationTarget[]> {
  const response = await getJSON<{ items?: AdminAggregationTarget[] | null }>('/admin/aggregation-targets')
  return itemsOrEmpty(response.items)
}

export async function createAggregationTarget(payload: AggregationTargetInput): Promise<{ success: boolean; code: string }> {
  return postJSON('/admin/aggregation-targets', payload)
}

export async function updateAggregationTarget(code: string, payload: AggregationTargetInput): Promise<{ success: boolean }> {
  return putJSON(`/admin/aggregation-targets/${encodeURIComponent(code)}`, payload)
}

export async function deleteAggregationTarget(code: string): Promise<{ deleted: boolean }> {
  return deleteJSON(`/admin/aggregation-targets/${encodeURIComponent(code)}`)
}

export async function checkAggregationTargetContract(
  payload: AggregationTargetContractCheckInput,
): Promise<AggregationTargetContractCheckResult> {
  return postJSON('/admin/aggregation-targets/check-contract', payload)
}

export async function checkAggregationTargetReverseAdmin(
  payload: AggregationTargetReverseAdminCheckInput,
): Promise<AggregationTargetReverseAdminCheckResult> {
  return postJSON('/admin/aggregation-targets/check-reverse-admin', payload)
}

export async function checkAggregationTargetTokenSupply(
  payload: AggregationTargetTokenSupplyCheckInput,
): Promise<AggregationTargetTokenSupplyCheckResult> {
  return postJSON('/admin/aggregation-targets/check-token-supply', payload)
}

export async function getChannelGroups(): Promise<ChannelGroup[]> {
  const response = await getJSON<{ items?: ChannelGroup[] | null }>('/channels')
  return itemsOrEmpty(response.items)
}

export async function listHealthChecks(): Promise<HealthCheckRecord[]> {
  const response = await getJSON<{ items?: HealthCheckRecord[] | null }>('/health-checks?limit=50')
  return itemsOrEmpty(response.items)
}

export async function runHealthChecks(payload: {
  autoDisable: boolean
  limit?: number
}): Promise<HealthRunResult[]> {
  const response = await postJSON<{ items?: HealthRunResult[] | null }>('/health-checks/run', payload)
  return itemsOrEmpty(response.items)
}

export async function getUsageSummary(days = 30): Promise<UsageSummary> {
  return getJSON(`/usage/summary?days=${days}`)
}

export async function getUsageLogs(query: UsageLogQuery = {}): Promise<UsageLogPage> {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, String(value))
    }
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : ''
  const response = await getJSON<Partial<UsageLogPage>>(`/usage-logs${suffix}`)
  return {
    configured: response.configured ?? false,
    message: response.message,
    items: itemsOrEmpty(response.items),
    total: response.total ?? 0,
    pageSize: response.pageSize ?? query.pageSize ?? 20,
    nextCursor: response.nextCursor,
    channels: itemsOrEmpty(response.channels),
    stats: response.stats ?? { quota: 0, quotaUsd: 0, rpm: 0, tpm: 0 },
  }
}

export async function getUsageLogConfiguration(): Promise<UsageLogConfiguration> {
  const response = await getJSON<Partial<UsageLogConfiguration>>('/usage-logs/configuration')
  return {
    configured: response.configured ?? false,
    message: response.message,
  }
}

export async function syncUsage(): Promise<UsageSyncResult> {
  return postJSON('/usage/sync', {})
}

export async function listModelGaps(): Promise<ModelGap[]> {
  const response = await getJSON<{ items?: ModelGap[] | null }>('/model-gaps')
  return itemsOrEmpty(response.items)
}

export async function createModelGap(payload: ModelGapInput): Promise<{ success: boolean; id: number }> {
  return postJSON('/admin/model-gaps', payload)
}

export async function updateModelGap(id: number, payload: ModelGapInput): Promise<{ success: boolean }> {
  return putJSON(`/admin/model-gaps/${id}`, payload)
}

export async function listWorkerRuns(): Promise<WorkerRunRecord[]> {
  const response = await getJSON<{ items?: WorkerRunRecord[] | null }>('/workers/runs?limit=30')
  return itemsOrEmpty(response.items)
}

export async function getSession(): Promise<AuthState> {
  return getJSON('/auth/me')
}

export async function login(payload: { username: string; password: string }): Promise<AuthState> {
  return postJSON('/auth/login', payload)
}

export async function register(payload: {
  username: string
  password: string
  displayName?: string
}): Promise<AuthState> {
  return postJSON('/auth/register', payload)
}

export async function logout(): Promise<{ success: boolean }> {
  return postJSON('/auth/logout', {})
}

export async function listAdminUsers(): Promise<AdminUser[]> {
  const response = await getJSON<{ items?: AdminUser[] | null }>('/admin/users')
  return itemsOrEmpty(response.items)
}

export async function createAdminUser(payload: {
  username: string
  password: string
  displayName?: string
}): Promise<{ user: AdminUser }> {
  return postJSON('/admin/users', payload)
}

export async function updateAdminUserAggregationTargets(
  userId: number,
  targets: AdminUserAggregationTarget[],
): Promise<{ success: boolean; targets: AdminUserAggregationTarget[] }> {
  return putJSON(`/admin/users/${userId}/aggregation-targets`, { targets })
}

export async function updateAdminUserStatus(
  userId: number,
  disabled: boolean,
): Promise<{ success: boolean; disabled: boolean }> {
  return putJSON(`/admin/users/${userId}/status`, { disabled })
}

export async function resetAdminUserPassword(
  userId: number,
  password: string,
): Promise<{ success: boolean }> {
  return putJSON(`/admin/users/${userId}/password`, { password })
}

export async function updateAdminUserRole(
  userId: number,
  role: 'user' | 'account_manager',
): Promise<{ success: boolean; role: string }> {
  return putJSON(`/admin/users/${userId}/role`, { role })
}

export async function listAuditLogs(): Promise<AuditLogRecord[]> {
  const response = await getJSON<{ items?: AuditLogRecord[] | null }>('/audit/logs?limit=100')
  return itemsOrEmpty(response.items)
}

export async function getOpsStatus(): Promise<OpsStatus> {
  return getJSON('/ops/status')
}

export async function exportKeyInventory(): Promise<KeyExportRecord[]> {
  const response = await getJSON<{ items?: KeyExportRecord[] | null }>('/keys/export?limit=5000')
  return itemsOrEmpty(response.items)
}

export async function importKeys(payload: KeyImportRequest): Promise<KeyImportResponse> {
  return postJSON('/keys/import', payload)
}

export async function listAPIKeys(): Promise<APIKeyListResponse> {
  const response = await getJSON<Partial<APIKeyListResponse> & { items?: APIKeyRecord[] | null }>('/keys?limit=100')
  return {
    items: itemsOrEmpty(response.items),
    total: response.total ?? 0,
    totalUsageUsd: response.totalUsageUsd ?? 0,
  }
}

export async function revealAPIKeyPlaintext(id: number): Promise<APIKeyPlaintextResponse> {
  return postJSON(`/keys/${id}/plaintext`, {})
}

export async function activateKey(id: number, payload: { targetCode: string }): Promise<{ newApiChannelId: number; targetCode: string }> {
  return postJSON(`/keys/${id}/activate`, payload)
}

export async function activateKeyToAssociatedTargets(
  id: number,
  payload: { channelTypes?: AggregationTargetChannelType[] } = {},
): Promise<{
  activated: boolean
  skipped: boolean
  total: number
  success: number
  failed: number
  newApiChannelIds?: number[]
}> {
  return postJSON(`/keys/${id}/activate-associated`, payload)
}

export interface KeyRetryPushResponse {
  activated: boolean
  skipped: boolean
  total: number
  success: number
  failed: number
  targetCodes: string[]
  newApiChannelIds?: number[]
  source: 'failed_events' | 'current_associations'
}

export async function retryKeyPush(id: number): Promise<KeyRetryPushResponse> {
  return postJSON(`/keys/${id}/retry-push`, {})
}

export async function disableKey(id: number): Promise<{ disabled: boolean }> {
  return postJSON(`/keys/${id}/disable`, {})
}

export async function deleteKey(id: number): Promise<{ deleted: boolean }> {
  return deleteJSON(`/keys/${id}`)
}

export async function listSyncEvents(): Promise<SyncEventRecord[]> {
  const response = await getJSON<{ items?: SyncEventRecord[] | null }>('/sync/events?limit=20')
  return itemsOrEmpty(response.items)
}

function itemsOrEmpty<T>(items?: T[] | null): T[] {
  return Array.isArray(items) ? items : []
}

const apiBaseURL = `${import.meta.env.BASE_URL.replace(/\/$/, '')}/api`

function resolveAPIURL(url: string) {
  return `${apiBaseURL}${url}`
}

async function getJSON<T>(url: string): Promise<T> {
  const response = await fetch(resolveAPIURL(url), { credentials: 'same-origin' })
  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }
  return response.json() as Promise<T>
}

async function postJSON<T>(url: string, payload: unknown): Promise<T> {
  const response = await fetch(resolveAPIURL(url), {
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
  const response = await fetch(resolveAPIURL(url), {
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
  const response = await fetch(resolveAPIURL(url), {
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
    const payload = JSON.parse(text) as { error?: string | { code?: string; message?: string }; message?: string }
    return (typeof payload.error === 'string' ? payload.error : payload.error?.message) || payload.message || text
  } catch {
    return text
  }
}

export interface UserAPIToken {
  id: number
  name: string
  prefix: string
  expiresAt: string
  revokedAt?: string
  lastUsedAt?: string
  createdAt: string
}

export async function listUserAPITokens(): Promise<UserAPIToken[]> {
  const result = await getJSON<{ items: UserAPIToken[] }>('/account/api-tokens')
  return result.items ?? []
}

export function createUserAPIToken(payload: { name: string; expiresInDays: number }) {
  return postJSON<{ id: number; token: string; expiresAt: string; scope: string }>('/account/api-tokens', payload)
}

export function revokeUserAPIToken(id: number) {
  return deleteJSON<{ revoked: boolean }>(`/account/api-tokens/${id}`)
}
