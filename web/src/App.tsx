import {
  Alert,
  Button,
  Card,
  Checkbox,
  ConfigProvider,
  Dropdown,
  Empty,
  Form,
  Input,
  InputNumber,
  Layout,
  Menu,
  Modal,
  Select,
  Segmented,
  Space,
  Statistic,
  Switch,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  message,
  theme,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { MenuProps } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import {
  Activity,
  AlertTriangle,
  BarChart3,
  CircleDollarSign,
  Clock,
  ClipboardPaste,
  Copy,
  Database,
  Download,
  FileText,
  HardDrive,
  HeartPulse,
  KeyRound,
  ListTree,
  LogOut,
  MoreHorizontal,
  Pencil,
  Plus,
  Power,
  PowerOff,
  RefreshCw,
  ServerCog,
  ShieldCheck,
  Trash2,
  Upload,
  UserPlus,
  Zap,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ClipboardEvent, type KeyboardEvent, type ReactNode } from 'react'
import {
  activateKey,
  checkAggregationTargetContract,
  checkAggregationTargetReverseAdmin,
  createAggregationTarget,
  deleteAggregationTarget,
  deleteKey,
  disableKey,
  exportKeyInventory,
  getAggregationTargets,
  getCategories,
  getChannelGroups,
  getDashboardSummary,
  getOpsStatus,
  getSession,
  getUsageSummary,
  importKeys,
  listAuditLogs,
  listAdminAggregationTargets,
  listAPIKeys,
  listHealthChecks,
  listSyncEvents,
  listWorkerRuns,
  login,
  logout,
  register,
  runHealthChecks,
  syncUsage,
  updateAggregationTarget,
  type AdminAggregationTarget,
  type AggregationTarget,
  type AggregationTargetConnectionMode,
  type AggregationTargetInput,
  type APIKeyRecord,
  type AuditLogRecord,
  type AuthUser,
  type Category,
  type ChannelGroup,
  type DashboardSummary,
  type HealthCheckRecord,
  type KeyExportRecord,
  type OpsStatus,
  type RecentError,
  type TableStat,
  type SyncEventRecord,
  type UsageSummary,
  type WorkerStat,
  type WorkerRunRecord,
} from './api'
import receiverPlatformDoc from './docs/receiver-platform.md?raw'

const { Sider, Content } = Layout
const { Text, Title } = Typography
const { TextArea } = Input

type PageKey = 'dashboard' | 'upload' | 'channels' | 'usage' | 'audit' | 'ops' | 'targets'

interface UploadFormValues {
  categoryCode: string
  endpointUrl?: string
  rawText: string
  models: string[]
  expectedTpm?: number
  note: string
}

interface BatchBedrockKeyFormValues {
  accessKey: string
  secretKey: string
  regions: string[]
}

interface PasteKeyFormValues {
  rawText: string
}

const AWS_BEDROCK_REGIONS = [
  'us-east-1',
  'us-east-2',
  'us-west-1',
  'us-west-2',
  'ca-central-1',
  'ca-west-1',
  'eu-central-1',
  'eu-central-2',
  'eu-north-1',
  'eu-south-1',
  'eu-south-2',
  'eu-west-1',
  'eu-west-2',
  'eu-west-3',
  'ap-east-2',
  'ap-northeast-1',
  'ap-northeast-2',
  'ap-northeast-3',
  'ap-south-1',
  'ap-south-2',
  'ap-southeast-1',
  'ap-southeast-2',
  'ap-southeast-3',
  'ap-southeast-4',
  'ap-southeast-5',
  'ap-southeast-6',
  'ap-southeast-7',
  'il-central-1',
  'me-central-1',
  'me-south-1',
  'af-south-1',
  'sa-east-1',
]

function endpointForCategory(category?: Category) {
  switch (category?.code) {
    case 'aws_bedrock':
      return 'https://bedrock-runtime.{region}.amazonaws.com'
    case 'anthropic':
      return 'https://api.anthropic.com'
    case 'openai':
      return 'https://api.openai.com/v1'
    case 'azure_openai':
      return 'https://{resource-name}.openai.azure.com'
    case 'google_ai_studio':
      return 'https://generativelanguage.googleapis.com/v1beta'
    default:
      return ''
  }
}

const emptySummary: DashboardSummary = {
  totalKeys: 0,
  activeKeys: 0,
  inventoryKeys: 0,
  disabledKeys: 0,
  totalUsageUsd: 0,
  categories: [],
}

const baseNavigationItems = [
  { key: 'dashboard', icon: <Activity size={16} />, label: '控制台' },
  { key: 'channels', icon: <ListTree size={16} />, label: '我的秘钥' },
  { key: 'usage', icon: <BarChart3 size={16} />, label: '消费快照' },
]

const adminNavigationItems = [
  { key: 'audit', icon: <FileText size={16} />, label: '审计日志' },
  { key: 'ops', icon: <ServerCog size={16} />, label: '运维状态' },
  { key: 'targets', icon: <HardDrive size={16} />, label: '接收平台' },
]

export default function App() {
  const [page, setPage] = useState<PageKey>('dashboard')
  const [summary, setSummary] = useState<DashboardSummary>(emptySummary)
  const [groups, setGroups] = useState<ChannelGroup[]>([])
  const [categories, setCategories] = useState<Category[]>([])
  const [aggregationTargets, setAggregationTargets] = useState<AggregationTarget[]>([])
  const [adminAggregationTargets, setAdminAggregationTargets] = useState<AdminAggregationTarget[]>([])
  const [keyRecords, setKeyRecords] = useState<APIKeyRecord[]>([])
  const [syncEvents, setSyncEvents] = useState<SyncEventRecord[]>([])
  const [healthChecks, setHealthChecks] = useState<HealthCheckRecord[]>([])
  const [usageSummary, setUsageSummary] = useState<UsageSummary | null>(null)
  const [workerRuns, setWorkerRuns] = useState<WorkerRunRecord[]>([])
  const [auditLogs, setAuditLogs] = useState<AuditLogRecord[]>([])
  const [opsStatus, setOpsStatus] = useState<OpsStatus | null>(null)
  const [keyExportRows, setKeyExportRows] = useState<KeyExportRecord[]>([])
  const [authChecked, setAuthChecked] = useState(false)
  const [authEnabled, setAuthEnabled] = useState(true)
  const [registrationEnabled, setRegistrationEnabled] = useState(false)
  const [currentUser, setCurrentUser] = useState<AuthUser | null>(null)
  const [loading, setLoading] = useState(false)
  const [operationLoading, setOperationLoading] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const canManageKeys = !authEnabled || currentUser?.role === 'admin' || currentUser?.role === 'root'
  const navigationItems = useMemo(
    () => (canManageKeys ? [...baseNavigationItems, ...adminNavigationItems] : baseNavigationItems),
    [canManageKeys],
  )

  const bootstrapSession = async () => {
    try {
      const session = await getSession()
      setAuthEnabled(session.authEnabled)
      setRegistrationEnabled(session.registrationEnabled)
      setCurrentUser(session.user ?? null)
    } catch {
      setAuthEnabled(true)
      setRegistrationEnabled(false)
      setCurrentUser(null)
    } finally {
      setAuthChecked(true)
    }
  }

  const refresh = async () => {
    setLoading(true)
    setError(null)
    try {
      const [
        summaryData,
        groupData,
        categoryData,
        aggregationTargetData,
        keyData,
        syncEventData,
        healthData,
        usageData,
        workerRunData,
        auditData,
        opsData,
        adminAggregationTargetData,
      ] = await Promise.all([
        getDashboardSummary(),
        getChannelGroups(),
        getCategories(),
        getAggregationTargets(),
        listAPIKeys(),
        listSyncEvents(),
        listHealthChecks(),
        getUsageSummary(30),
        canManageKeys ? listWorkerRuns() : Promise.resolve([]),
        canManageKeys ? listAuditLogs() : Promise.resolve([]),
        canManageKeys ? getOpsStatus() : Promise.resolve(null),
        canManageKeys ? listAdminAggregationTargets() : Promise.resolve([]),
      ])
      setSummary(summaryData)
      setGroups(groupData)
      setCategories(categoryData)
      setAggregationTargets(aggregationTargetData)
      setKeyRecords(keyData.items)
      setSyncEvents(syncEventData)
      setHealthChecks(healthData)
      setUsageSummary(usageData)
      setWorkerRuns(workerRunData)
      setAuditLogs(auditData)
      setOpsStatus(opsData)
      setAdminAggregationTargets(adminAggregationTargetData)
    } catch (err) {
      if (err instanceof Error && err.message.includes('unauthorized')) {
        setCurrentUser(null)
      }
      setError(err instanceof Error ? err.message : '请求失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void bootstrapSession()
  }, [])

  useEffect(() => {
    if (authChecked && (!authEnabled || currentUser)) {
      void refresh()
    }
  }, [authChecked, authEnabled, currentUser])

  useEffect(() => {
    if (!canManageKeys && ['audit', 'ops', 'targets'].includes(page)) {
      setPage('dashboard')
    }
  }, [canManageKeys, page])

  const handleLogin = async (values: { username: string; password: string }) => {
    const session = await login(values)
    setAuthEnabled(session.authEnabled)
    setRegistrationEnabled(session.registrationEnabled)
    setCurrentUser(session.user ?? null)
    message.success('登录成功')
  }

  const handleRegister = async (values: { username: string; password: string; displayName?: string }) => {
    const session = await register(values)
    setAuthEnabled(session.authEnabled)
    setRegistrationEnabled(session.registrationEnabled)
    setCurrentUser(session.user ?? null)
    message.success('注册成功')
  }

  const handleLogout = async () => {
    await logout()
    setCurrentUser(null)
    setPage('dashboard')
    message.success('已退出')
  }

  const handleActivateKey = async (id: number, targetCode: string) => {
    const result = await activateKey(id, { targetCode })
    const target = aggregationTargets.find((item) => item.code === result.targetCode) ?? aggregationTargets.find((item) => item.code === targetCode)
    message.success(`已上线到 ${target?.name || result.targetCode || targetCode}，远端渠道 #${result.newApiChannelId}`)
    await refresh()
  }

  const handleDisableKey = async (id: number) => {
    const confirmed = await confirmDanger('停用渠道', `确认停用 Key #${id} 并同步禁用 new-api 渠道？`)
    if (!confirmed) {
      return
    }
    await disableKey(id)
    message.success('已停用渠道')
    await refresh()
  }

  const handleDeleteKey = async (row: APIKeyRecord) => {
    if (row.status === 'active') {
      message.warning('请先下线该秘钥，再删除')
      return
    }
    const confirmed = await confirmDanger('删除秘钥', `确认删除 Key #${row.id}？删除后本地库存记录不可恢复。`)
    if (!confirmed) {
      return
    }
    await deleteKey(row.id)
    message.success('秘钥已删除')
    await refresh()
  }

  const handleHealthRun = async () => {
    const confirmed = await confirmDanger('运行健康检查', '健康检查失败的渠道会被自动停用，确认继续？')
    if (!confirmed) {
      return
    }
    setOperationLoading('health')
    try {
      const results = await runHealthChecks({ autoDisable: true, limit: 200 })
      const failed = results.filter((item) => !item.success).length
      const disabled = results.filter((item) => item.autoDisabled).length
      if (failed > 0) {
        message.warning(`健康检查完成：${failed} 个失败，自动停用 ${disabled} 个`)
      } else {
        message.success(`健康检查完成：${results.length} 个渠道正常`)
      }
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '健康检查失败')
    } finally {
      setOperationLoading(null)
    }
  }

  const handleUsageSync = async () => {
    setOperationLoading('usage')
    try {
      const result = await syncUsage()
      if (result.totalDeltaQuota > 0) {
        message.success(`消费同步完成：新增 $${result.totalDeltaUsd.toFixed(4)}`)
      } else if (result.baseline > 0) {
        message.info(`已建立 ${result.baseline} 个渠道的用量基线`)
      } else {
        message.info('消费同步完成，暂无新增用量')
      }
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '消费同步失败')
    } finally {
      setOperationLoading(null)
    }
  }

  const handleKeyInventoryExport = async () => {
    setOperationLoading('key-export')
    try {
      const rows = await exportKeyInventory()
      setKeyExportRows(rows)

      const blob = new Blob([JSON.stringify(rows, null, 2)], { type: 'application/json;charset=utf-8' })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `keyhub-inventory-${new Date().toISOString().slice(0, 10)}.json`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)

      message.success(`已导出 ${rows.length} 条脱敏 Key 清单`)
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '导出失败')
    } finally {
      setOperationLoading(null)
    }
  }

  const handleSaveAggregationTarget = async (values: AggregationTargetInput, originalCode?: string) => {
    setOperationLoading('target-save')
    try {
      if (originalCode) {
        await updateAggregationTarget(originalCode, values)
      } else {
        await createAggregationTarget(values)
      }
      message.success('接收平台已保存')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setOperationLoading(null)
    }
  }

  const handleDeleteAggregationTarget = async (code: string) => {
    const confirmed = await confirmDanger('删除接收平台', `确认删除接收平台 ${code}？`)
    if (!confirmed) {
      return
    }
    setOperationLoading(`target-delete:${code}`)
    try {
      await deleteAggregationTarget(code)
      message.success('接收平台已删除')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    } finally {
      setOperationLoading(null)
    }
  }

  if (!authChecked) {
    return (
      <ConfigProvider locale={zhCN} theme={{ algorithm: theme.defaultAlgorithm }}>
        <div className="login-shell">
          <Card loading />
        </div>
      </ConfigProvider>
    )
  }

  if (authEnabled && !currentUser) {
    return (
      <ConfigProvider
        locale={zhCN}
        theme={{
          algorithm: theme.defaultAlgorithm,
          token: {
            colorPrimary: '#1677ff',
            borderRadius: 8,
            fontFamily:
              '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
          },
        }}
      >
        <LoginPage registrationEnabled={registrationEnabled} onLogin={handleLogin} onRegister={handleRegister} />
      </ConfigProvider>
    )
  }

  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#1677ff',
          borderRadius: 8,
          fontFamily:
            '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
        },
      }}
    >
      <Layout className="app-shell">
        <Sider className="sidebar" width={184} theme="light">
          <div className="brand">
            <KeyRound size={22} />
            <span>KeyHub</span>
          </div>
          <Menu
            className="sidebar-nav"
            mode="inline"
            selectedKeys={[page]}
            onSelect={({ key }) => setPage(key as PageKey)}
            items={navigationItems}
          />
          <div className="sidebar-account">
            <Tag className="sidebar-user" icon={<ShieldCheck size={13} />}>
              {currentUser?.displayName || currentUser?.username || 'local'}
            </Tag>
            <div className="sidebar-actions">
              {authEnabled ? (
                <Button className="sidebar-action-button" icon={<LogOut size={16} />} onClick={handleLogout}>
                  退出
                </Button>
              ) : null}
            </div>
          </div>
        </Sider>
        <Layout className="main-layout">
          <Content className="content">
            {error ? (
              <Alert className="error-alert" type="error" showIcon message="数据加载失败" description={error} />
            ) : null}
            {page === 'dashboard' ? (
              <Dashboard
                summary={summary}
                groups={groups}
                healthChecks={healthChecks}
                loading={loading}
                operationLoading={operationLoading}
                canManageKeys={canManageKeys}
                onUpload={() => setPage('upload')}
                onRunHealthChecks={handleHealthRun}
              />
            ) : null}
            {page === 'upload' ? (
              <UploadPage
                categories={categories}
                onImported={refresh}
              />
            ) : null}
            {page === 'channels' ? (
              <KeyListPage
                categories={categories}
                keyRecords={keyRecords}
                aggregationTargets={aggregationTargets}
                syncEvents={syncEvents}
                healthChecks={healthChecks}
                loading={loading}
                canManageKeys={canManageKeys}
                onImported={refresh}
                onActivate={handleActivateKey}
                onDisable={handleDisableKey}
                onDelete={handleDeleteKey}
                showHeading={false}
              />
            ) : null}
            {page === 'usage' ? (
              <UsagePage
                usageSummary={usageSummary}
                workerRuns={workerRuns}
                loading={loading}
                canManageKeys={canManageKeys}
                syncing={operationLoading === 'usage'}
                onSync={handleUsageSync}
              />
            ) : null}
            {page === 'audit' && canManageKeys ? <AuditPage auditLogs={auditLogs} loading={loading} /> : null}
            {page === 'ops' && canManageKeys ? (
              <OpsPage
                opsStatus={opsStatus}
                keyExportRows={keyExportRows}
                loading={loading}
                exporting={operationLoading === 'key-export'}
                onExport={handleKeyInventoryExport}
              />
            ) : null}
            {page === 'targets' && canManageKeys ? (
              <AggregationTargetsPage
                targets={adminAggregationTargets}
                loading={loading}
                operationLoading={operationLoading}
                onSave={handleSaveAggregationTarget}
                onDelete={handleDeleteAggregationTarget}
              />
            ) : null}
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}

type AuthMode = 'login' | 'register'

interface LoginPageProps {
  registrationEnabled: boolean
  onLogin: (values: { username: string; password: string }) => Promise<void>
  onRegister: (values: { username: string; password: string; displayName?: string }) => Promise<void>
}

interface RegisterFormValues {
  username: string
  password: string
  confirmPassword: string
  displayName?: string
}

function LoginPage({ registrationEnabled, onLogin, onRegister }: LoginPageProps) {
  const [loginForm] = Form.useForm<{ username: string; password: string }>()
  const [registerForm] = Form.useForm<RegisterFormValues>()
  const [mode, setMode] = useState<AuthMode>('login')
  const [loading, setLoading] = useState(false)

  const submitLogin = async (values: { username: string; password: string }) => {
    setLoading(true)
    try {
      await onLogin(values)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  const submitRegister = async ({ confirmPassword, ...values }: RegisterFormValues) => {
    void confirmPassword
    setLoading(true)
    try {
      await onRegister(values)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '注册失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-shell">
      <Card className="login-card">
        <div className="login-brand">
          <span className="login-icon">
            <KeyRound size={24} />
          </span>
          <div>
            <Title level={3}>KeyHub</Title>
            <Text type="secondary">供应商 Key 管理系统</Text>
          </div>
        </div>
        {registrationEnabled ? (
          <Segmented
            block
            className="auth-mode"
            value={mode}
            onChange={(value) => setMode(value as AuthMode)}
            options={[
              { label: '登录', value: 'login' },
              { label: '注册', value: 'register' },
            ]}
          />
        ) : null}
        {mode === 'login' ? (
          <Form form={loginForm} layout="vertical" initialValues={{ username: 'admin' }} onFinish={submitLogin}>
            <Form.Item label="账号" name="username" rules={[{ required: true, message: '请输入账号' }]}>
              <Input autoComplete="username" />
            </Form.Item>
            <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}>
              <Input.Password autoComplete="current-password" />
            </Form.Item>
            <Button type="primary" block htmlType="submit" icon={<KeyRound size={16} />} loading={loading}>
              登录
            </Button>
          </Form>
        ) : (
          <Form form={registerForm} layout="vertical" onFinish={submitRegister}>
            <Form.Item
              label="账号"
              name="username"
              rules={[
                { required: true, message: '请输入账号' },
                {
                  pattern: /^[A-Za-z0-9_.-]{3,64}$/,
                  message: '账号需为 3-64 位字母、数字、点、下划线或连字符',
                },
              ]}
            >
              <Input autoComplete="username" />
            </Form.Item>
            <Form.Item label="显示名称" name="displayName" rules={[{ max: 128, message: '显示名称最多 128 个字符' }]}>
              <Input autoComplete="name" />
            </Form.Item>
            <Form.Item
              label="密码"
              name="password"
              rules={[
                { required: true, message: '请输入密码' },
                { min: 8, message: '密码至少 8 个字符' },
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Form.Item
              label="确认密码"
              name="confirmPassword"
              dependencies={['password']}
              rules={[
                { required: true, message: '请再次输入密码' },
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    if (!value || getFieldValue('password') === value) {
                      return Promise.resolve()
                    }
                    return Promise.reject(new Error('两次密码不一致'))
                  },
                }),
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" block htmlType="submit" icon={<UserPlus size={16} />} loading={loading}>
              注册
            </Button>
          </Form>
        )}
      </Card>
    </div>
  )
}

function LegacyLoginPage({ onLogin }: { onLogin: (values: { username: string; password: string }) => Promise<void> }) {
  const [form] = Form.useForm<{ username: string; password: string }>()
  const [loading, setLoading] = useState(false)

  const submit = async () => {
    const values = await form.validateFields()
    setLoading(true)
    try {
      await onLogin(values)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-shell">
      <Card className="login-card">
        <div className="login-brand">
          <span className="login-icon">
            <KeyRound size={24} />
          </span>
          <div>
            <Title level={3}>KeyHub</Title>
            <Text type="secondary">供应商 Key 管理系统</Text>
          </div>
        </div>
        <Form form={form} layout="vertical" initialValues={{ username: 'admin' }} onFinish={submit}>
          <Form.Item label="账号" name="username" rules={[{ required: true, message: '请输入账号' }]}>
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" block loading={loading} onClick={submit}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  )
}

function Dashboard({
  summary,
  groups,
  healthChecks,
  loading,
  operationLoading,
  canManageKeys,
  onUpload,
  onRunHealthChecks,
}: {
  summary: DashboardSummary
  groups: ChannelGroup[]
  healthChecks: HealthCheckRecord[]
  loading: boolean
  operationLoading: string | null
  canManageKeys: boolean
  onUpload: () => void
  onRunHealthChecks: () => Promise<void>
}) {
  const channelColumns = useMemo<ColumnsType<ChannelGroup>>(
    () => [
      {
        title: '分类',
        dataIndex: 'categoryCode',
        width: 180,
        render: (value: string) => <Tag color="blue">{value}</Tag>,
      },
      { title: '标签', dataIndex: 'tag' },
      { title: 'Key 数量', dataIndex: 'keyCount', width: 120 },
      {
        title: '启用 / 停用',
        width: 150,
        render: (_, row) => (
          <Space>
            <Tag color="green">{row.activeCount} 启用</Tag>
            <Tag>{row.disabledCount} 停用</Tag>
          </Space>
        ),
      },
      {
        title: '累计消费',
        dataIndex: 'usedUsd',
        width: 140,
        render: (value: number) => `$${value.toFixed(4)}`,
      },
    ],
    [],
  )

  return (
    <div className="page-stack">
      <section className="dashboard-banner">
        <div>
          <Title level={2}>供应商 Key 管理控制台</Title>
          <Text>库存、上线、健康检查集中在这里处理。</Text>
        </div>
        <Button type="primary" icon={<Upload size={16} />} onClick={onUpload}>
          上传密钥
        </Button>
      </section>

      <section className="stats-grid">
        <MetricCard icon={<Database size={22} />} label="我的秘钥" value={summary.totalKeys} color="blue" loading={loading} />
        <MetricCard
          icon={<CircleDollarSign size={22} />}
          label="累计消费"
          value={`$${summary.totalUsageUsd.toFixed(2)}`}
          color="teal"
          loading={loading}
        />
        <MetricCard icon={<Activity size={22} />} label="库存" value={summary.inventoryKeys} color="orange" loading={loading} />
        <MetricCard icon={<Zap size={22} />} label="启用中" value={summary.activeKeys} color="green" loading={loading} />
      </section>

      <section>
        <Card title="分类概况" loading={loading}>
          <div className="category-list">
            {summary.categories.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无分类数据" />
            ) : (
              summary.categories.map((category) => (
                <div className="category-row" key={category.code}>
                  <div>
                    <Text strong>{category.label}</Text>
                    <div>
                      <Text type="secondary">type {category.newApiType}</Text>
                    </div>
                  </div>
                  <Space wrap>
                    <Tag color="green">{category.activeKeys} 活跃</Tag>
                    <Tag color="orange">{category.inventoryKeys} 库存</Tag>
                    <Tag>{category.totalKeys} 总数</Tag>
                  </Space>
                </div>
              ))
            )}
          </div>
        </Card>
      </section>

      <section>
        <Card title="我的秘钥">
          <Table
            rowKey={(row) => `${row.categoryCode}-${row.tag}`}
            dataSource={groups}
            columns={channelColumns}
            loading={loading}
            pagination={false}
            locale={tableEmpty('暂无渠道')}
            scroll={{ x: 760 }}
          />
        </Card>
      </section>

      <HealthChecksPanel
        healthChecks={healthChecks}
        loading={loading}
        operationLoading={operationLoading}
        canManageKeys={canManageKeys}
        onRunHealthChecks={onRunHealthChecks}
      />
    </div>
  )
}

function UsageLogsTable({ syncEvents, loading }: { syncEvents: SyncEventRecord[]; loading: boolean }) {
  const columns = useMemo<ColumnsType<SyncEventRecord>>(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      {
        title: 'Key',
        dataIndex: 'apiKeyId',
        width: 90,
        render: (value?: number) => (value ? `#${value}` : '-'),
      },
      { title: '动作', dataIndex: 'action', width: 110 },
      {
        title: '状态',
        dataIndex: 'status',
        width: 110,
        render: (value: string) => <Tag color={syncStatusColor(value)}>{value}</Tag>,
      },
      { title: '错误', dataIndex: 'errorMessage' },
      {
        title: '时间',
        dataIndex: 'createdAt',
        width: 180,
        render: (value: string) => formatDateTime(value),
      },
    ],
    [],
  )

  return (
    <Table
      rowKey="id"
      dataSource={syncEvents}
      columns={columns}
      loading={loading}
      pagination={false}
      size="small"
      locale={tableEmpty('暂无使用日志')}
      scroll={{ x: 760 }}
    />
  )
}

function HealthChecksTable({ healthChecks, loading }: { healthChecks: HealthCheckRecord[]; loading: boolean }) {
  const columns = useMemo<ColumnsType<HealthCheckRecord>>(
    () => [
      {
        title: 'Key',
        width: 190,
        render: (_, row) => (
          <Space wrap>
            <Tag>#{row.apiKeyId}</Tag>
            <Text>{row.keyHint}</Text>
          </Space>
        ),
      },
      {
        title: '分类',
        dataIndex: 'categoryCode',
        width: 150,
        render: (value: string) => <Tag color="blue">{value}</Tag>,
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (value: string) => <Tag color={healthStatusColor(value)}>{healthStatusText(value)}</Tag>,
      },
      {
        title: '延迟',
        dataIndex: 'latencyMs',
        width: 100,
        render: (value: number) => `${value} ms`,
      },
      { title: '错误', dataIndex: 'errorMessage' },
      {
        title: '检查时间',
        dataIndex: 'checkedAt',
        width: 180,
        render: (value: string) => formatDateTime(value),
      },
    ],
    [],
  )

  return (
    <Table
      rowKey="id"
      dataSource={healthChecks}
      columns={columns}
      loading={loading}
      pagination={{ pageSize: 8, size: 'small' }}
      size="small"
      locale={tableEmpty('暂无健康检查记录')}
      scroll={{ x: 920 }}
    />
  )
}

function HealthChecksPanel({
  healthChecks,
  loading,
  operationLoading,
  canManageKeys,
  onRunHealthChecks,
}: {
  healthChecks: HealthCheckRecord[]
  loading: boolean
  operationLoading: string | null
  canManageKeys: boolean
  onRunHealthChecks: () => Promise<void>
}) {
  return (
    <section>
      <Card
        title="最近健康检查"
        extra={
          canManageKeys ? (
            <Button icon={<HeartPulse size={15} />} onClick={onRunHealthChecks} loading={operationLoading === 'health'}>
              健康检查
            </Button>
          ) : null
        }
      >
        <HealthChecksTable healthChecks={healthChecks} loading={loading} />
      </Card>
    </section>
  )
}

function UploadPage({
  categories,
  onImported,
}: {
  categories: Category[]
  onImported: () => Promise<void>
}) {
  const [form] = Form.useForm<UploadFormValues>()
  const [batchForm] = Form.useForm<BatchBedrockKeyFormValues>()
  const [pasteForm] = Form.useForm<PasteKeyFormValues>()
  const [importLoading, setImportLoading] = useState(false)
  const [pasteModalOpen, setPasteModalOpen] = useState(false)
  const [batchModalOpen, setBatchModalOpen] = useState(false)
  const [pasteLoading, setPasteLoading] = useState(false)
  const [batchImportLoading, setBatchImportLoading] = useState(false)
  const selectedCategoryCode = Form.useWatch('categoryCode', form)
  const endpointUrl = Form.useWatch('endpointUrl', form)
  const selectedBatchRegions = Form.useWatch('regions', batchForm) ?? []

  const selectedCategory = useMemo(
    () => categories.find((category) => category.code === selectedCategoryCode) ?? categories[0],
    [categories, selectedCategoryCode],
  )

  const isAWSKeyCategory = selectedCategory?.code === 'aws_bedrock'

  useEffect(() => {
    if (!selectedCategory) {
      return
    }
    const currentCategory = form.getFieldValue('categoryCode')
    if (!currentCategory) {
      form.setFieldsValue({
        categoryCode: selectedCategory.code,
        endpointUrl: endpointForCategory(selectedCategory),
        models: selectedCategory.defaultModels,
      })
      return
    }
  }, [form, selectedCategory])

  const handleImport = async () => {
    const values = await form.validateFields()
    setImportLoading(true)
    try {
      const result = await importKeys({
        categoryCode: values.categoryCode,
        endpointUrl: values.endpointUrl ?? '',
        rawText: values.rawText,
        models: values.models,
        expectedTpm: Number(values.expectedTpm ?? 0),
        note: values.note ?? '',
      })
      message.success(`导入 ${result.imported} 个，跳过 ${result.duplicates} 个，失败 ${result.failed} 个`)
      await onImported()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '导入失败')
    } finally {
      setImportLoading(false)
    }
  }

  const openBatchModal = () => {
    if (!isAWSKeyCategory) {
      message.warning('批量上 Key 仅支持 AWS Bedrock 渠道')
      return
    }
    batchForm.setFieldsValue({ accessKey: '', secretKey: '', regions: [] })
    setBatchModalOpen(true)
  }

  const openPasteModal = () => {
    if (!isAWSKeyCategory) {
      message.warning('粘贴解析仅支持 AWS Bedrock 渠道')
      return
    }
    pasteForm.setFieldsValue({ rawText: '' })
    setPasteModalOpen(true)
  }

  const closePasteModal = () => {
    if (pasteLoading) {
      return
    }
    setPasteModalOpen(false)
  }

  const closeBatchModal = () => {
    if (batchImportLoading) {
      return
    }
    setBatchModalOpen(false)
  }

  const handlePasteAppend = async () => {
    if (!isAWSKeyCategory) {
      message.warning('请先选择 AWS Bedrock 渠道')
      return
    }

    setPasteLoading(true)
    try {
      const values = await pasteForm.validateFields()
      const parsedText = normalizePastedAWSKeys(values.rawText)
      const currentRawText = String(form.getFieldValue('rawText') ?? '').trim()

      form.setFieldValue('rawText', currentRawText ? `${currentRawText}\n${parsedText.rawText}` : parsedText.rawText)
      setPasteModalOpen(false)
      pasteForm.resetFields()
      message.success(`已解析 ${parsedText.count} 行到密钥列表`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '解析失败')
    } finally {
      setPasteLoading(false)
    }
  }

  const handleBatchUpload = async () => {
    if (!isAWSKeyCategory) {
      message.warning('请先选择 AWS Bedrock 渠道')
      return
    }

    setBatchImportLoading(true)
    try {
      const batchValues = await batchForm.validateFields()
      const rawText = batchValues.regions
        .map((region) => `${batchValues.accessKey.trim()}|${batchValues.secretKey.trim()}|${region}`)
        .join('\n')
      const currentRawText = String(form.getFieldValue('rawText') ?? '').trim()

      form.setFieldValue('rawText', currentRawText ? `${currentRawText}\n${rawText}` : rawText)
      setBatchModalOpen(false)
      batchForm.resetFields()
      message.success(`已添加 ${batchValues.regions.length} 行到密钥列表`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '添加失败')
    } finally {
      setBatchImportLoading(false)
    }
  }

  return (
    <div className="upload-waterfall">
      <section className="upload-panel upload-intake-panel">
        <div className="upload-panel-head">
          <div className="upload-panel-title">
            <span className="upload-panel-icon">
              <KeyRound size={18} />
            </span>
            <div>
              <Title level={4}>密钥入库</Title>
              <Text type="secondary">{selectedCategory ? `格式：${selectedCategory.keyFormat}` : '等待渠道分类'}</Text>
            </div>
          </div>
          <div className="upload-panel-reminder">
            <ShieldCheck size={14} />
            <span>在秘钥上线前，系统不会使用这个秘钥</span>
          </div>
        </div>
        <Form form={form} layout="vertical" initialValues={{ expectedTpm: 0 }}>
          <div className="upload-fields-grid">
            <Form.Item label="渠道分类" name="categoryCode" rules={[{ required: true, message: '请选择渠道分类' }]}>
              <Select
                optionLabelProp="label"
                classNames={{ popup: { root: 'category-select-dropdown' } }}
                options={categories.map((category) => ({
                  value: category.code,
                  label: category.label,
                }))}
                onChange={(categoryCode) => {
                  const nextCategory = categories.find((category) => category.code === categoryCode)
                  form.setFieldsValue({
                    endpointUrl: endpointForCategory(nextCategory),
                    models: nextCategory?.defaultModels ?? [],
                  })
                }}
                optionRender={(option) => (
                  <div className="category-select-option">
                    <span className="category-select-option-icon">
                      <KeyRound size={14} />
                    </span>
                    <span className="category-select-option-name">{option.label}</span>
                  </div>
                )}
              />
            </Form.Item>
            <Form.Item name="endpointUrl" hidden>
              <Input />
            </Form.Item>
            <Form.Item label="端点地址" className="endpoint-info-item">
              <div className="endpoint-system-text">
                {endpointUrl || endpointForCategory(selectedCategory) || '未配置'}
              </div>
            </Form.Item>
            <Form.Item
              label="模型范围"
              name="models"
              rules={[
                {
                  validator: (_, value: string[]) =>
                    Array.isArray(value) && value.length > 0 ? Promise.resolve() : Promise.reject(new Error('请输入模型')),
                },
              ]}
            >
              <ModelTagsInput placeholder="claude-opus-4-6, claude-opus-4-7" />
            </Form.Item>
            <Form.Item
              label="预期 TPM"
              name="expectedTpm"
              rules={[{ type: 'number', min: 0, message: '预期 TPM 不能小于 0' }]}
            >
              <InputNumber min={0} precision={0} placeholder="例如 200000" style={{ width: '100%' }} />
            </Form.Item>
          </div>
          <div className="upload-key-heading">
            <span className="upload-required-label">密钥列表</span>
            <Space size={8}>
              <Tooltip title={isAWSKeyCategory ? '粘贴 AK|SK|Region 格式文本并解析到列表' : '仅 AWS Bedrock 渠道支持'}>
                <Button size="small" icon={<ClipboardPaste size={14} />} onClick={openPasteModal} disabled={!isAWSKeyCategory}>
                  粘贴
                </Button>
              </Tooltip>
              <Tooltip title={isAWSKeyCategory ? '按 AK/SK 批量生成 32 个 AWS Bedrock 区域 Key' : '仅 AWS Bedrock 渠道支持'}>
                <Button size="small" icon={<Upload size={14} />} onClick={openBatchModal} disabled={!isAWSKeyCategory}>
                  批量上 Key
                </Button>
              </Tooltip>
            </Space>
          </div>
          <Form.Item
            className="upload-key-field"
            name="rawText"
            rules={[
              {
                validator: (_, value: string) =>
                  hasKeyRowsContent(value) ? Promise.resolve() : Promise.reject(new Error('请至少填写一行密钥')),
              },
            ]}
          >
            <KeyRowsInput category={selectedCategory} />
          </Form.Item>
          <Form.Item label="备注" name="note">
            <Input placeholder="来源、用途、结算说明" />
          </Form.Item>
          <div className="upload-action-row">
            <Space>
              <Button type="primary" icon={<Upload size={16} />} onClick={handleImport} loading={importLoading}>
                入库
              </Button>
            </Space>
          </div>
        </Form>
      </section>

      <Modal
        title="粘贴密钥"
        open={pasteModalOpen}
        okText="解析到列表"
        cancelText="取消"
        width={760}
        className="paste-key-modal"
        confirmLoading={pasteLoading}
        onOk={handlePasteAppend}
        onCancel={closePasteModal}
      >
        <Form form={pasteForm} layout="vertical" className="paste-key-form">
          <Form.Item
            label="密钥文本"
            name="rawText"
            rules={[{ required: true, whitespace: true, message: '请粘贴密钥文本' }]}
          >
            <TextArea
              autoFocus
              className="paste-key-textarea"
              placeholder={'AKIAxxxx|secretxxxx|us-east-1\nAKIAyyyy|secretyyyy|us-west-2'}
              rows={12}
            />
          </Form.Item>
          <Text type="secondary">每行一条，格式为 AccessKey|SecretKey|Region；空行会自动忽略。</Text>
        </Form>
      </Modal>

      <Modal
        title="批量上 Key"
        open={batchModalOpen}
        okText="添加到列表"
        cancelText="取消"
        width={860}
        className="batch-key-modal"
        confirmLoading={batchImportLoading}
        onOk={handleBatchUpload}
        onCancel={closeBatchModal}
      >
        <Form form={batchForm} layout="vertical" className="batch-key-form" initialValues={{ regions: [] }}>
          <div className="batch-aksk-grid">
            <Form.Item
              label="AK"
              name="accessKey"
              rules={[{ required: true, whitespace: true, message: '请输入 AK' }]}
            >
              <Input autoComplete="off" placeholder="AKIA..." />
            </Form.Item>
            <Form.Item
              label="SK"
              name="secretKey"
              rules={[{ required: true, whitespace: true, message: '请输入 SK' }]}
            >
              <Input.Password autoComplete="new-password" placeholder="Secret Access Key" />
            </Form.Item>
          </div>
          <div className="batch-region-head">
            <Text strong>区域</Text>
            <Space size={8} wrap>
              <Text type="secondary">
                已选择 {selectedBatchRegions.length}/{AWS_BEDROCK_REGIONS.length}
              </Text>
              <Button size="small" onClick={() => batchForm.setFieldValue('regions', AWS_BEDROCK_REGIONS)}>
                全选
              </Button>
              <Button size="small" onClick={() => batchForm.setFieldValue('regions', [])}>
                清空
              </Button>
            </Space>
          </div>
          <Form.Item
            name="regions"
            rules={[
              {
                validator: (_, value: string[]) =>
                  Array.isArray(value) && value.length > 0 ? Promise.resolve() : Promise.reject(new Error('请选择区域')),
              },
            ]}
          >
            <Checkbox.Group
              className="batch-region-grid"
              options={AWS_BEDROCK_REGIONS.map((region) => ({ label: region, value: region }))}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

function KeyListPage({
  categories,
  keyRecords,
  aggregationTargets,
  syncEvents = [],
  healthChecks = [],
  loading,
  canManageKeys,
  compact = false,
  onImported,
  onActivate,
  onDisable,
  onDelete,
  title = '我的秘钥',
  showHeading = true,
  showDetails = true,
}: {
  categories: Category[]
  keyRecords: APIKeyRecord[]
  aggregationTargets: AggregationTarget[]
  syncEvents?: SyncEventRecord[]
  healthChecks?: HealthCheckRecord[]
  loading: boolean
  canManageKeys: boolean
  compact?: boolean
  onImported: () => Promise<void>
  onActivate?: (id: number, targetCode: string) => Promise<void>
  onDisable?: (id: number) => Promise<void>
  onDelete?: (row: APIKeyRecord) => Promise<void>
  title?: string
  showHeading?: boolean
  showDetails?: boolean
}) {
  const [actionID, setActionID] = useState<number | null>(null)
  const [searchText, setSearchText] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [uploadModalOpen, setUploadModalOpen] = useState(false)
  const [detailModal, setDetailModal] = useState<'usage' | 'health' | null>(null)
  const [activateTargetCode, setActivateTargetCode] = useState('')
  const [activatingRecord, setActivatingRecord] = useState<APIKeyRecord | null>(null)

  const openActivateModal = (row: APIKeyRecord) => {
    if (!canManageKeys) {
      message.warning('仅管理员可以上线密钥')
      return
    }
    if (!onActivate) {
      return
    }
    if (aggregationTargets.length === 0) {
      message.error('未配置接收平台地址')
      return
    }
    setActivatingRecord(row)
    setActivateTargetCode(aggregationTargets.find((target) => target.default)?.code || aggregationTargets[0].code)
  }

  const runActivate = async () => {
    if (!activatingRecord || !activateTargetCode || !onActivate) {
      return
    }
    setActionID(activatingRecord.id)
    try {
      await onActivate(activatingRecord.id, activateTargetCode)
      setActivatingRecord(null)
      setActivateTargetCode('')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActionID(null)
    }
  }

  const runAction = async (row: APIKeyRecord, action: KeyRecordAction) => {
    if (action === 'activate') {
      openActivateModal(row)
      return
    }
    if (action === 'delete') {
      if (!onDelete) {
        return
      }
      setActionID(row.id)
      try {
        await onDelete(row)
      } catch (err) {
        message.error(err instanceof Error ? err.message : '操作失败')
      } finally {
        setActionID(null)
      }
      return
    }
    if (!onDisable) {
      return
    }
    setActionID(row.id)
    try {
      await onDisable(row.id)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActionID(null)
    }
  }

  const handleModalImported = async () => {
    await onImported()
    setUploadModalOpen(false)
  }

  const visibleKeyRecords = useMemo(() => {
    if (compact) {
      return keyRecords
    }
    const keyword = searchText.trim().toLowerCase()
    return keyRecords.filter((row) => {
      const matchesStatus = statusFilter === 'all' || row.status === statusFilter
      const haystack = [
        row.id,
        row.categoryCode,
        row.keyHint,
        row.tag,
        row.status,
        row.newApiChannelId,
        row.expectedTpm,
        row.models.join(' '),
      ]
        .filter((value) => value !== undefined && value !== null)
        .join(' ')
        .toLowerCase()
      return matchesStatus && (!keyword || haystack.includes(keyword))
    })
  }, [compact, keyRecords, searchText, statusFilter])

  return (
    <div className="page-stack">
      {!compact && showHeading ? <Title level={2}>{title}</Title> : null}
      <Card
        className="key-list-card"
        title={compact ? title : null}
        extra={
          compact ? null : (
            <Space className="table-toolbar" wrap>
              <Input.Search
                allowClear
                placeholder="搜索 Key、标签"
                value={searchText}
                onChange={(event) => setSearchText(event.target.value)}
                style={{ width: 360, maxWidth: '100%' }}
              />
              <Select
                value={statusFilter}
                onChange={setStatusFilter}
                style={{ width: 128 }}
                options={[
                  { value: 'all', label: '全部状态' },
                  { value: 'active', label: '活跃' },
                  { value: 'inventory', label: '库存' },
                  { value: 'disabled', label: '停用' },
                  { value: 'error', label: '异常' },
                  { value: 'revoked', label: '吊销' },
                ]}
              />
              {showDetails ? (
                <>
                  <Button className="key-detail-trigger" icon={<FileText size={16} />} onClick={() => setDetailModal('usage')}>
                    使用日志
                  </Button>
                  <Button className="key-detail-trigger" icon={<HeartPulse size={16} />} onClick={() => setDetailModal('health')}>
                    健康检查
                  </Button>
                </>
              ) : null}
              <Button
                type="primary"
                className="key-upload-trigger"
                icon={<Upload size={16} />}
                onClick={() => setUploadModalOpen(true)}
              >
                上传秘钥
              </Button>
            </Space>
          )
        }
      >
        <KeyRecordList
          records={visibleKeyRecords}
          loading={loading}
          compact={compact}
          actionID={actionID}
          canManageKeys={canManageKeys}
          onAction={runAction}
        />
      </Card>
      <Modal
        title="上线密钥"
        open={!!activatingRecord}
        okText="上线"
        cancelText="取消"
        confirmLoading={actionID === activatingRecord?.id}
        onOk={runActivate}
        onCancel={() => {
          setActivatingRecord(null)
          setActivateTargetCode('')
        }}
      >
        <Form layout="vertical">
          <Form.Item label="接收平台" required>
            <Select
              value={activateTargetCode}
              onChange={setActivateTargetCode}
              options={aggregationTargets.map((target) => ({
                value: target.code,
                label: (
                  <Space size={8}>
                    <span>{`${target.name} (${target.baseUrl})`}</span>
                    <Tag color={target.connectionMode === 'new_api_reverse' ? 'purple' : 'blue'}>
                      {target.connectionMode === 'new_api_reverse' ? 'new-api逆向' : 'API方式'}
                    </Tag>
                  </Space>
                ),
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title="上传秘钥"
        open={uploadModalOpen}
        footer={null}
        width={1080}
        className="key-upload-modal"
        onCancel={() => setUploadModalOpen(false)}
      >
        <UploadPage categories={categories} onImported={handleModalImported} />
      </Modal>
      <Modal
        title="使用日志"
        open={detailModal === 'usage'}
        footer={null}
        width={960}
        className="key-detail-modal"
        onCancel={() => setDetailModal(null)}
      >
        <UsageLogsTable syncEvents={syncEvents} loading={loading} />
      </Modal>
      <Modal
        title="最近健康检查"
        open={detailModal === 'health'}
        footer={null}
        width={1080}
        className="key-detail-modal"
        onCancel={() => setDetailModal(null)}
      >
        <HealthChecksTable healthChecks={healthChecks} loading={loading} />
      </Modal>
    </div>
  )
}

type KeyRecordAction = 'activate' | 'disable' | 'delete'

interface KeyRecordListProps {
  records: APIKeyRecord[]
  loading: boolean
  compact: boolean
  actionID: number | null
  canManageKeys: boolean
  onAction: (row: APIKeyRecord, action: KeyRecordAction) => void
}

const keyRecordListHeaders = [
  { key: 'category', label: '分类' },
  { key: 'channel', label: '发布渠道id' },
  { key: 'key', label: '密钥' },
  { key: 'tag', label: '标签' },
  { key: 'models', label: '模型' },
  { key: 'tpm', label: '预期 TPM' },
  { key: 'usage', label: '消耗额度' },
  { key: 'status', label: '状态' },
  { key: 'health', label: '健康' },
  { key: 'created', label: '创建时间' },
  { key: 'action', label: '操作' },
]

function KeyRecordList({ records, loading, compact, actionID, canManageKeys, onAction }: KeyRecordListProps) {
  const pageSize = compact ? 6 : 12
  const [pageIndex, setPageIndex] = useState(1)
  const pageCount = Math.max(1, Math.ceil(records.length / pageSize))
  const currentPage = Math.min(pageIndex, pageCount)
  const pageStart = (currentPage - 1) * pageSize
  const pagedRecords = records.slice(pageStart, pageStart + pageSize)
  const startLabel = records.length === 0 ? 0 : pageStart + 1
  const endLabel = Math.min(records.length, pageStart + pagedRecords.length)
  const showSkeleton = loading && records.length === 0

  useEffect(() => {
    setPageIndex(1)
  }, [records])

  useEffect(() => {
    if (pageIndex > pageCount) {
      setPageIndex(pageCount)
    }
  }, [pageCount, pageIndex])

  return (
    <div className={`key-record-list ${loading ? 'is-loading' : ''}`} aria-busy={loading}>
      <div className="key-record-list-scroll">
        <div className="key-record-list-grid" role="table" aria-label="我的秘钥列表">
          <div className="key-record-list-header" role="row">
            {keyRecordListHeaders.map((column) => (
              <div className={`key-record-list-head-cell is-${column.key}`} role="columnheader" key={column.key}>
                {column.label}
              </div>
            ))}
          </div>
          <div className="key-record-list-body">
            {showSkeleton
              ? Array.from({ length: 5 }, (_, index) => (
                  <div className="key-record-list-row is-skeleton" role="row" key={`key-record-skeleton-${index}`}>
                    {keyRecordListHeaders.map((column) => (
                      <div className={`key-record-list-cell is-${column.key}`} role="cell" key={column.key}>
                        <span className="key-record-skeleton-bar" />
                      </div>
                    ))}
                  </div>
                ))
              : null}

            {!showSkeleton && records.length === 0 ? (
              <div className="key-record-list-empty">
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无 Key 记录" />
              </div>
            ) : null}

            {!showSkeleton
              ? pagedRecords.map((row) => (
                  <div className="key-record-list-row" role="row" key={row.id}>
                    <div className="key-record-list-cell is-category" role="cell">
                      <Tag color="blue">{row.categoryCode}</Tag>
                    </div>
                    <div className="key-record-list-cell is-channel" role="cell">
                      {row.newApiChannelId ? <Tag color="geekblue">#{row.newApiChannelId}</Tag> : <Text type="secondary">-</Text>}
                    </div>
                    <div className="key-record-list-cell is-key" role="cell">
                      <KeyHintDisplay value={row.keyHint} />
                    </div>
                    <div className="key-record-list-cell is-tag" role="cell">
                      {row.tag ? <Tag>{row.tag}</Tag> : <Text type="secondary">-</Text>}
                    </div>
                    <div className="key-record-list-cell is-models" role="cell">
                      <ModelSummary models={row.models} />
                    </div>
                    <div className="key-record-list-cell is-tpm" role="cell">
                      {row.expectedTpm > 0 ? formatQuota(row.expectedTpm) : <Text type="secondary">-</Text>}
                    </div>
                    <div className="key-record-list-cell is-usage" role="cell">
                      {formatQuota(row.usageQuota30d)}
                    </div>
                    <div className="key-record-list-cell is-status" role="cell">
                      <Tag color={statusColor(row.status)}>{statusText(row.status)}</Tag>
                    </div>
                    <div className="key-record-list-cell is-health" role="cell">
                      <Tag color={healthStatusColor(row.lastHealthStatus)}>{healthStatusText(row.lastHealthStatus)}</Tag>
                    </div>
                    <div className="key-record-list-cell is-created" role="cell">
                      {formatDateTime(row.createdAt)}
                    </div>
                    <div className="key-record-list-cell is-action" role="cell">
                      <KeyManageDropdown row={row} loading={actionID === row.id} canManageKeys={canManageKeys} onAction={onAction} />
                    </div>
                  </div>
                ))
              : null}
          </div>
        </div>
      </div>
      <div className="key-record-list-footer">
        <Text type="secondary">
          显示 {startLabel}-{endLabel} / {records.length}
        </Text>
        <Space size={8}>
          <Button size="small" disabled={currentPage <= 1} onClick={() => setPageIndex((value) => Math.max(1, value - 1))}>
            上一页
          </Button>
          <Text className="key-record-page-indicator">
            {currentPage} / {pageCount}
          </Text>
          <Button
            size="small"
            disabled={currentPage >= pageCount}
            onClick={() => setPageIndex((value) => Math.min(pageCount, value + 1))}
          >
            下一页
          </Button>
        </Space>
      </div>
    </div>
  )
}

function KeyManageDropdown({
  row,
  loading,
  canManageKeys,
  onAction,
}: {
  row: APIKeyRecord
  loading: boolean
  canManageKeys: boolean
  onAction: (row: APIKeyRecord, action: KeyRecordAction) => void
}) {
  const items: MenuProps['items'] = [
    {
      key: 'edit',
      icon: <Pencil size={14} />,
      label: '编辑',
      disabled: loading,
    },
    {
      key: 'activate',
      icon: <Power size={14} />,
      label: '上线',
      disabled: loading || !canManageKeys || row.status === 'active',
    },
    {
      key: 'disable',
      icon: <PowerOff size={14} />,
      label: '下线',
      disabled: loading || !canManageKeys || row.status !== 'active',
    },
    {
      type: 'divider',
    },
    {
      key: 'delete',
      icon: <Trash2 size={14} />,
      label: '删除',
      danger: true,
      disabled: loading || !canManageKeys || row.status === 'active',
    },
  ]

  const handleMenuClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'activate' || key === 'disable' || key === 'delete') {
      onAction(row, key)
      return
    }
    if (key === 'edit') {
      message.info('编辑功能待接入')
      return
    }
  }

  return (
    <Dropdown menu={{ items, onClick: handleMenuClick }} trigger={['click']} placement="bottomRight">
      <Button className="key-record-manage-button" size="small" loading={loading} icon={<MoreHorizontal size={15} />}>
        管理
      </Button>
    </Dropdown>
  )
}

function KeyHintDisplay({ value }: { value: string }) {
  const keyHint = String(value || '').trim()

  if (!keyHint) {
    return <Text type="secondary">-</Text>
  }

  return (
    <Tooltip title={keyHint}>
      <span className="key-record-secret" title={keyHint}>
        {keyHint}
      </span>
    </Tooltip>
  )
}

function ModelSummary({ models }: { models: string[] }) {
  if (models.length === 0) {
    return <Text type="secondary">-</Text>
  }

  const modelText = models.join('、')

  return (
    <Tooltip title={modelText}>
      <span className="key-record-model-text" title={modelText}>
        {modelText}
      </span>
    </Tooltip>
  )
}

function UsagePage({
  usageSummary,
  workerRuns,
  loading,
  canManageKeys,
  syncing,
  onSync,
}: {
  usageSummary: UsageSummary | null
  workerRuns: WorkerRunRecord[]
  loading: boolean
  canManageKeys: boolean
  syncing: boolean
  onSync: () => Promise<void>
}) {
  const usage = usageSummary ?? {
    days: 30,
    totalQuota: 0,
    totalUsd: 0,
    byDay: [],
    categories: [],
    channels: [],
  }
  const lastWorker = workerRuns[0]
  const maxDayUSD = Math.max(...usage.byDay.map((item) => item.usd), 0.000001)

  const categoryColumns = useMemo<ColumnsType<UsageSummary['categories'][number]>>(
    () => [
      {
        title: '分类',
        dataIndex: 'categoryLabel',
        render: (value: string, row) => (
          <Space>
            <Text strong>{value}</Text>
            <Tag>{row.categoryCode}</Tag>
          </Space>
        ),
      },
      { title: '额度', dataIndex: 'quota', width: 140, render: (value: number) => formatQuota(value) },
      { title: '金额', dataIndex: 'usd', width: 120, render: (value: number) => `$${value.toFixed(4)}` },
    ],
    [],
  )

  const channelColumns = useMemo<ColumnsType<UsageSummary['channels'][number]>>(
    () => [
      { title: 'Key', dataIndex: 'apiKeyId', width: 90, render: (value: number) => `#${value}` },
      {
        title: '接收平台',
        dataIndex: 'targetCode',
        width: 130,
        render: (value: string) => <Tag>{value || 'default'}</Tag>,
      },
      {
        title: 'new-api',
        dataIndex: 'newApiChannelId',
        width: 110,
        render: (value: number) => <Tag color="blue">#{value}</Tag>,
      },
      {
        title: '分类',
        dataIndex: 'categoryCode',
        width: 150,
        render: (value: string) => <Tag>{value}</Tag>,
      },
      { title: '密钥', dataIndex: 'keyHint' },
      { title: '标签', dataIndex: 'tag', width: 140 },
      { title: '额度', dataIndex: 'quota', width: 140, render: (value: number) => formatQuota(value) },
      { title: '金额', dataIndex: 'usd', width: 120, render: (value: number) => `$${value.toFixed(4)}` },
    ],
    [],
  )

  return (
    <div className="page-stack">
      <section className="page-heading">
        <div>
          <Title level={2}>消费快照</Title>
          <Text type="secondary">按接收平台 KeyHub 用量接口返回的渠道累计额度计算每日增量。</Text>
        </div>
        {canManageKeys ? (
          <Button type="primary" icon={<RefreshCw size={16} />} onClick={onSync} loading={syncing}>
            立即同步
          </Button>
        ) : null}
      </section>

      <section className="stats-grid">
        <MetricCard
          icon={<CircleDollarSign size={22} />}
          label={`${usage.days} 天消费`}
          value={`$${usage.totalUsd.toFixed(4)}`}
          color="teal"
          loading={loading}
        />
        <MetricCard
          icon={<Database size={22} />}
          label="额度增量"
          value={formatQuota(usage.totalQuota)}
          color="blue"
          loading={loading}
        />
        <MetricCard
          icon={<BarChart3 size={22} />}
          label="有消费渠道"
          value={usage.channels.length}
          color="green"
          loading={loading}
        />
        <MetricCard
          icon={<Clock size={22} />}
          label="最近任务"
          value={lastWorker ? workerStatusText(lastWorker.status) : '-'}
          color="orange"
          loading={loading}
        />
      </section>

      <section className="two-column">
        <Card title="日趋势" loading={loading}>
          <div className="usage-bars">
            {usage.byDay.length === 0 ? (
              <Text type="secondary">暂无消费快照</Text>
            ) : (
              usage.byDay.map((item) => (
                <div className="usage-bar-row" key={item.statDate}>
                  <Text className="usage-date">{item.statDate.slice(5)}</Text>
                  <div className="usage-bar-track">
                    <div className="usage-bar-fill" style={{ width: `${Math.max(4, (item.usd / maxDayUSD) * 100)}%` }} />
                  </div>
                  <Text className="usage-amount">${item.usd.toFixed(4)}</Text>
                </div>
              ))
            )}
          </div>
        </Card>

        <Card title="分类排行" loading={loading}>
          <Table
            rowKey="categoryCode"
            dataSource={usage.categories}
            columns={categoryColumns}
            pagination={false}
            size="small"
            locale={tableEmpty('暂无分类消费')}
            scroll={{ x: 520 }}
          />
        </Card>
      </section>

      <Card title="渠道明细">
        <Table
          rowKey={(row) => `${row.apiKeyId}-${row.targetCode}-${row.newApiChannelId}`}
          dataSource={usage.channels}
          columns={channelColumns}
          loading={loading}
          pagination={{ pageSize: 10, size: 'small' }}
          locale={tableEmpty('暂无渠道消费')}
          scroll={{ x: 1030 }}
        />
      </Card>

    </div>
  )
}

function AuditPage({ auditLogs, loading }: { auditLogs: AuditLogRecord[]; loading: boolean }) {
  const columns = useMemo<ColumnsType<AuditLogRecord>>(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      { title: '操作者', dataIndex: 'actor', width: 130 },
      { title: '动作', dataIndex: 'action', width: 180 },
      { title: '对象', dataIndex: 'targetType', width: 150 },
      {
        title: '对象ID',
        dataIndex: 'targetId',
        width: 100,
        render: (value?: number) => (value ? `#${value}` : '-'),
      },
      {
        title: '详情',
        dataIndex: 'detailJson',
        render: (value: string) => <Text className="audit-detail">{value || '-'}</Text>,
      },
      {
        title: '时间',
        dataIndex: 'createdAt',
        width: 180,
        render: (value: string) => formatDateTime(value),
      },
    ],
    [],
  )

  return (
    <div className="page-stack">
      <section className="page-heading">
        <div>
          <Title level={2}>审计日志</Title>
          <Text type="secondary">记录登录、人工操作和后台 worker 执行。</Text>
        </div>
      </section>
      <Card>
        <Table
          rowKey="id"
          dataSource={auditLogs}
          columns={columns}
          loading={loading}
          pagination={{ pageSize: 12, size: 'small' }}
          locale={tableEmpty('暂无审计日志')}
          scroll={{ x: 1000 }}
        />
      </Card>
    </div>
  )
}

interface AggregationTargetFormValues {
  code: string
  name: string
  connectionMode: AggregationTargetConnectionMode
  baseUrl: string
  token?: string
  reverseUsername?: string
  reversePassword?: string
  enabled: boolean
  default: boolean
}

const aggregationTargetModeLabels: Record<AggregationTargetConnectionMode, string> = {
  api: 'API方式',
  new_api_reverse: 'new-api逆向',
}

function AggregationTargetsPage({
  targets,
  loading,
  operationLoading,
  onSave,
  onDelete,
}: {
  targets: AdminAggregationTarget[]
  loading: boolean
  operationLoading: string | null
  onSave: (values: AggregationTargetInput, originalCode?: string) => Promise<void>
  onDelete: (code: string) => Promise<void>
}) {
  const [form] = Form.useForm<AggregationTargetFormValues>()
  const [modalOpen, setModalOpen] = useState(false)
  const [editingTarget, setEditingTarget] = useState<AdminAggregationTarget | null>(null)
  const [contractChecking, setContractChecking] = useState(false)
  const [reverseAdminChecking, setReverseAdminChecking] = useState(false)
  const connectionMode = Form.useWatch('connectionMode', form) ?? 'api'
  const canRetainToken = editingTarget?.connectionMode === 'api' && editingTarget.hasToken
  const canRetainReversePassword =
    editingTarget?.connectionMode === 'new_api_reverse' && editingTarget.hasReversePassword

  const openCreate = () => {
    setEditingTarget(null)
    form.setFieldsValue({
      code: '',
      name: '',
      connectionMode: 'api',
      baseUrl: '',
      token: '',
      reverseUsername: '',
      reversePassword: '',
      enabled: true,
      default: targets.length === 0,
    })
    setModalOpen(true)
  }

  const openEdit = (target: AdminAggregationTarget) => {
    if (target.source !== 'database') {
      return
    }
    setEditingTarget(target)
    form.setFieldsValue({
      code: target.code,
      name: target.name,
      connectionMode: target.connectionMode ?? 'api',
      baseUrl: target.baseUrl,
      token: '',
      reverseUsername: target.reverseUsername ?? '',
      reversePassword: '',
      enabled: target.enabled,
      default: target.default,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    const mode = values.connectionMode ?? 'api'
    const payload: AggregationTargetInput = {
      code: values.code.trim(),
      name: values.name.trim(),
      baseUrl: values.baseUrl.trim(),
      connectionMode: mode,
      enabled: values.enabled,
      default: values.default,
    }
    if (mode === 'api') {
      payload.token = values.token?.trim() || undefined
    } else {
      payload.reverseUsername = values.reverseUsername?.trim() || undefined
      payload.reversePassword = values.reversePassword?.trim() || undefined
    }
    await onSave(payload, editingTarget?.code)
    setModalOpen(false)
    setEditingTarget(null)
  }

  const checkContract = async () => {
    if (connectionMode !== 'api') {
      return
    }
    const values = await form.validateFields(['code', 'baseUrl'])
    const token = String(form.getFieldValue('token') ?? '').trim()
    if (!canRetainToken && !token) {
      message.warning('请填写 Token 后再检查')
      return
    }
    setContractChecking(true)
    try {
      const result = await checkAggregationTargetContract({
        code: editingTarget?.code ?? String(values.code ?? '').trim(),
        baseUrl: String(values.baseUrl ?? '').trim(),
        token: token || undefined,
      })
      message.success(
        result.usageCount > 0
          ? `契约接口检查通过，读取到 ${result.usageCount} 条渠道用量`
          : '契约接口检查通过，当前暂无渠道用量',
      )
    } catch (err) {
      message.error(err instanceof Error ? err.message : '契约接口检查失败')
    } finally {
      setContractChecking(false)
    }
  }

  const checkReverseAdmin = async () => {
    if (connectionMode !== 'new_api_reverse') {
      return
    }
    const values = await form.validateFields(['baseUrl', 'reverseUsername'])
    const password = String(form.getFieldValue('reversePassword') ?? '').trim()
    if (!canRetainReversePassword && !password) {
      message.warning('请填写密码后再校验')
      return
    }
    setReverseAdminChecking(true)
    try {
      const result = await checkAggregationTargetReverseAdmin({
        code: editingTarget?.code ?? String(form.getFieldValue('code') ?? '').trim(),
        baseUrl: String(values.baseUrl ?? '').trim(),
        reverseUsername: String(values.reverseUsername ?? '').trim(),
        reversePassword: password || undefined,
      })
      message.success(`管理员权限校验通过，可查询渠道列表，共 ${result.channelTotal} 个渠道`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '管理员权限校验失败')
    } finally {
      setReverseAdminChecking(false)
    }
  }

  const columns = useMemo<ColumnsType<AdminAggregationTarget>>(
    () => [
      {
        title: '标识',
        dataIndex: 'code',
        width: 150,
        render: (value: string, row) => (
          <Space>
            <Text strong>{value}</Text>
            {row.default ? <Tag color="blue">默认</Tag> : null}
          </Space>
        ),
      },
      { title: '名称', dataIndex: 'name', width: 180 },
      {
        title: '接入方式',
        dataIndex: 'connectionMode',
        width: 130,
        render: (value: AggregationTargetConnectionMode) => (
          <Tag color={value === 'new_api_reverse' ? 'purple' : 'blue'}>{aggregationTargetConnectionModeText(value)}</Tag>
        ),
      },
      {
        title: '地址',
        dataIndex: 'baseUrl',
        render: (value: string) => <Text className="audit-detail">{value}</Text>,
      },
      {
        title: '状态',
        width: 120,
        render: (_, row) => <Tag color={row.enabled ? 'green' : 'default'}>{row.enabled ? '启用' : '停用'}</Tag>,
      },
      {
        title: '凭据',
        width: 150,
        render: (_, row) =>
          row.connectionMode === 'new_api_reverse' ? (
            <Space size={4} wrap>
              <Tag color={row.reverseUsername ? 'blue' : 'red'}>{row.reverseUsername || '账号缺失'}</Tag>
              <Tag color={row.hasReversePassword ? 'green' : 'red'}>
                {row.hasReversePassword ? '密码已配置' : '密码缺失'}
              </Tag>
            </Space>
          ) : (
            <Tag color={row.hasToken ? 'green' : 'red'}>{row.hasToken ? 'Token 已配置' : 'Token 缺失'}</Tag>
          ),
      },
      {
        title: '来源',
        dataIndex: 'source',
        width: 110,
        render: (value: string) => <Tag>{aggregationTargetSourceText(value)}</Tag>,
      },
      {
        title: '更新时间',
        dataIndex: 'updatedAt',
        width: 180,
        render: (value?: string) => formatDateTime(value ?? ''),
      },
      {
        title: '操作',
        width: 120,
        fixed: 'right',
        render: (_, row) => (
          <Space>
            <Tooltip title={row.source === 'database' ? '编辑' : '环境变量配置不可编辑'}>
              <Button
                icon={<Pencil size={15} />}
                disabled={row.source !== 'database'}
                onClick={() => openEdit(row)}
              />
            </Tooltip>
            <Tooltip title={row.source === 'database' ? '删除' : '环境变量配置不可删除'}>
              <Button
                danger
                icon={<Trash2 size={15} />}
                disabled={row.source !== 'database'}
                loading={operationLoading === `target-delete:${row.code}`}
                onClick={() => onDelete(row.code)}
              />
            </Tooltip>
          </Space>
        ),
      },
    ],
    [operationLoading, onDelete],
  )

  return (
    <div className="page-stack">
      <section className="page-heading">
        <div>
          <Title level={2}>接收平台</Title>
          <Text type="secondary">配置上线弹窗中可选择的 new-api 接收平台地址。</Text>
        </div>
        <Button type="primary" icon={<Plus size={16} />} onClick={openCreate}>
          新增平台
        </Button>
      </section>

      <Card>
        <Table
          rowKey={(row) => `${row.source}:${row.code}`}
          dataSource={targets}
          columns={columns}
          loading={loading}
          pagination={false}
          locale={tableEmpty('暂无接收平台')}
          scroll={{ x: 1120 }}
        />
      </Card>

      <Card title="接入文档" className="receiver-doc-card">
        <MarkdownDocument source={receiverPlatformDoc} />
      </Card>

      <Modal
        title={editingTarget ? '编辑接收平台' : '新增接收平台'}
        open={modalOpen}
        okText="保存"
        cancelText="取消"
        confirmLoading={operationLoading === 'target-save'}
        onOk={submit}
        onCancel={() => setModalOpen(false)}
        destroyOnHidden
      >
        <Form
          form={form}
          layout="vertical"
          autoComplete="off"
          initialValues={{ connectionMode: 'api', enabled: true, default: false }}
        >
          <div className="browser-autofill-decoy" aria-hidden="true">
            <input type="text" name="username" autoComplete="username" tabIndex={-1} />
            <input type="password" name="password" autoComplete="current-password" tabIndex={-1} />
          </div>
          <Form.Item
            label="标识"
            name="code"
            rules={[
              { required: true, message: '请输入标识' },
              {
                pattern: /^[A-Za-z0-9_.-]{1,64}$/,
                message: '仅支持字母、数字、点、下划线或连字符',
              },
            ]}
          >
            <Input
              disabled={Boolean(editingTarget)}
              placeholder="default"
              autoComplete="off"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
          <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
            <Input
              placeholder="生产 new-api"
              autoComplete="off"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
          <Form.Item name="connectionMode" hidden>
            <Input />
          </Form.Item>
          <Tabs
            className="receiver-mode-tabs"
            activeKey={connectionMode}
            onChange={(key) => form.setFieldValue('connectionMode', key as AggregationTargetConnectionMode)}
            items={[
              {
                key: 'api',
                label: aggregationTargetModeLabels.api,
                children:
                  connectionMode === 'api' ? (
                    <>
                      <Form.Item
                        label="平台地址"
                        name="baseUrl"
                        rules={[{ required: true, message: '请输入平台地址' }]}
                      >
                        <Input
                          name="keyhub-target-endpoint"
                          placeholder="https://new-api.example.com"
                          autoComplete="section-keyhub-target off"
                          inputMode="url"
                          autoCapitalize="none"
                          spellCheck={false}
                          data-1p-ignore="true"
                          data-lpignore="true"
                          data-form-type="other"
                        />
                      </Form.Item>
                      <Form.Item
                        label="Token"
                        name="token"
                        rules={canRetainToken ? [] : [{ required: true, message: '请输入 Token' }]}
                      >
                        <Input.Password
                          name="keyhub-target-auth-token"
                          placeholder={canRetainToken ? '留空则保持原 Token' : 'new-api 的 KEYHUB_PUSH_TOKEN'}
                          autoComplete="section-keyhub-target new-password"
                          autoCapitalize="none"
                          spellCheck={false}
                          data-1p-ignore="true"
                          data-lpignore="true"
                          data-form-type="other"
                        />
                      </Form.Item>
                      <Form.Item>
                        <Tooltip title="检查 GET /api/keyhub/channels/usage 是否可用">
                          <Button icon={<ShieldCheck size={15} />} loading={contractChecking} onClick={checkContract}>
                            检查契约接口
                          </Button>
                        </Tooltip>
                      </Form.Item>
                    </>
                  ) : null,
              },
              {
                key: 'new_api_reverse',
                label: aggregationTargetModeLabels.new_api_reverse,
                children:
                  connectionMode === 'new_api_reverse' ? (
                    <>
                      <Alert className="receiver-mode-hint" type="info" showIcon message="账号需要具备号池管理员权限" />
                      <Form.Item
                        label="站点地址"
                        name="baseUrl"
                        rules={[{ required: true, message: '请输入站点地址' }]}
                      >
                        <Input
                          name="keyhub-reverse-site"
                          placeholder="https://new-api.example.com"
                          autoComplete="section-keyhub-reverse off"
                          inputMode="url"
                          autoCapitalize="none"
                          spellCheck={false}
                          data-1p-ignore="true"
                          data-lpignore="true"
                          data-form-type="other"
                        />
                      </Form.Item>
                      <Form.Item label="账号" name="reverseUsername" rules={[{ required: true, message: '请输入账号' }]}>
                        <Input
                          name="keyhub-reverse-account"
                          placeholder="new-api 管理员账号"
                          autoComplete="section-keyhub-reverse username"
                          autoCapitalize="none"
                          spellCheck={false}
                          data-1p-ignore="true"
                          data-lpignore="true"
                          data-form-type="other"
                        />
                      </Form.Item>
                      <Form.Item
                        label="密码"
                        name="reversePassword"
                        rules={canRetainReversePassword ? [] : [{ required: true, message: '请输入密码' }]}
                      >
                        <Input.Password
                          name="keyhub-reverse-password"
                          placeholder={canRetainReversePassword ? '留空则保持原密码' : 'new-api 管理员密码'}
                          autoComplete="section-keyhub-reverse new-password"
                          autoCapitalize="none"
                          spellCheck={false}
                          data-1p-ignore="true"
                          data-lpignore="true"
                          data-form-type="other"
                        />
                      </Form.Item>
                      <Form.Item>
                        <Tooltip title="登录 new-api 后请求 GET /api/channel/，确认账号可查询渠道列表">
                          <Button icon={<ShieldCheck size={15} />} loading={reverseAdminChecking} onClick={checkReverseAdmin}>
                            校验管理员权限
                          </Button>
                        </Tooltip>
                      </Form.Item>
                    </>
                  ) : null,
              },
            ]}
          />
          <Space size={24}>
            <Form.Item label="启用" name="enabled" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item label="默认" name="default" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Form>
      </Modal>
    </div>
  )
}

function OpsPage({
  opsStatus,
  keyExportRows,
  loading,
  exporting,
  onExport,
}: {
  opsStatus: OpsStatus | null
  keyExportRows: KeyExportRecord[]
  loading: boolean
  exporting: boolean
  onExport: () => Promise<void>
}) {
  const health = opsStatus?.health
  const tableStats = opsStatus?.tableStats ?? []
  const workerStats = opsStatus?.workerStats ?? []
  const recentErrors = opsStatus?.recentErrors ?? []
  const componentEntries = Object.entries(health?.components ?? {})
  const failedComponents = componentEntries.filter(([, component]) => component.status === 'failed').length
  const totalDataMB = tableStats.reduce((total, item) => total + item.dataMb + item.indexMb, 0)

  const tableColumns = useMemo<ColumnsType<TableStat>>(
    () => [
      { title: '表名', dataIndex: 'tableName' },
      { title: '行数', dataIndex: 'rows', width: 120 },
      { title: '数据', dataIndex: 'dataMb', width: 120, render: (value: number) => `${value.toFixed(2)} MB` },
      { title: '索引', dataIndex: 'indexMb', width: 120, render: (value: number) => `${value.toFixed(2)} MB` },
    ],
    [],
  )

  const workerColumns = useMemo<ColumnsType<WorkerStat>>(
    () => [
      { title: '任务', dataIndex: 'workerName' },
      { title: '总次数', dataIndex: 'totalRuns', width: 90 },
      { title: '成功', dataIndex: 'successRuns', width: 90 },
      {
        title: '成功率',
        dataIndex: 'successRate',
        width: 110,
        render: (value: number) => `${Math.round(value * 100)}%`,
      },
      {
        title: '最近状态',
        dataIndex: 'lastStatus',
        width: 110,
        render: (value: string) => <Tag color={syncStatusColor(value)}>{workerStatusText(value)}</Tag>,
      },
      {
        title: '最近完成',
        dataIndex: 'lastFinishedAt',
        width: 180,
        render: (value?: string) => formatDateTime(value ?? ''),
      },
    ],
    [],
  )

  const errorColumns = useMemo<ColumnsType<RecentError>>(
    () => [
      {
        title: '来源',
        dataIndex: 'source',
        width: 110,
        render: (value: string) => <Tag>{sourceText(value)}</Tag>,
      },
      { title: '错误', dataIndex: 'message', render: (value: string) => <Text className="audit-detail">{value}</Text> },
      {
        title: '时间',
        dataIndex: 'createdAt',
        width: 180,
        render: (value: string) => formatDateTime(value),
      },
    ],
    [],
  )

  const exportColumns = useMemo<ColumnsType<KeyExportRecord>>(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80, render: (value: number) => `#${value}` },
      {
        title: '分类',
        dataIndex: 'categoryCode',
        width: 170,
        render: (value: string, row) => (
          <Space>
            <Text>{row.categoryLabel}</Text>
            <Tag>{value}</Tag>
          </Space>
        ),
      },
      { title: 'Key', dataIndex: 'keyHint', width: 150 },
      { title: '标签', dataIndex: 'tag', width: 130 },
      {
        title: '预期 TPM',
        dataIndex: 'expectedTpm',
        width: 120,
        render: (value: number) => (value > 0 ? formatQuota(value) : '-'),
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (value: string) => <Tag color={statusColor(value)}>{statusText(value)}</Tag>,
      },
      {
        title: 'new-api',
        dataIndex: 'newApiChannelId',
        width: 110,
        render: (value?: number) => (value ? <Tag color="blue">#{value}</Tag> : '-'),
      },
      {
        title: '模型',
        dataIndex: 'models',
        render: (models: string[]) =>
          models?.length ? (
            <Space wrap size={[4, 4]}>
              {models.map((model) => (
                <Tag key={model}>{model}</Tag>
              ))}
            </Space>
          ) : (
            '-'
          ),
      },
      { title: '错误', dataIndex: 'lastError', render: (value: string) => <Text className="audit-detail">{value || '-'}</Text> },
      {
        title: '创建时间',
        dataIndex: 'createdAt',
        width: 180,
        render: (value: string) => formatDateTime(value),
      },
    ],
    [],
  )

  return (
    <div className="page-stack">
      <section className="page-heading">
        <div>
          <Title level={2}>运维状态</Title>
          <Text type="secondary">数据库、new-api、静态资源和后台任务状态。</Text>
        </div>
        <Button type="primary" icon={<Download size={16} />} onClick={onExport} loading={exporting}>
          导出清单
        </Button>
      </section>

      <section className="stats-grid">
        <MetricCard
          icon={<ServerCog size={22} />}
          label="系统状态"
          value={componentStatusText(health?.status ?? '')}
          color={health?.status === 'ok' ? 'green' : 'orange'}
          loading={loading}
        />
        <MetricCard icon={<AlertTriangle size={22} />} label="异常组件" value={failedComponents} color="orange" loading={loading} />
        <MetricCard icon={<HardDrive size={22} />} label="库表容量" value={`${totalDataMB.toFixed(2)} MB`} color="blue" loading={loading} />
        <MetricCard icon={<Database size={22} />} label="最近错误" value={recentErrors.length} color="teal" loading={loading} />
      </section>

      <section className="component-grid">
        {componentEntries.length === 0 ? (
          <Card loading={loading}>
            <Text type="secondary">暂无组件状态</Text>
          </Card>
        ) : (
          componentEntries.map(([name, component]) => (
            <Card key={name} loading={loading} className="component-card">
              <div className="component-header">
                <Text strong>{componentLabel(name)}</Text>
                <Tag color={componentStatusColor(component.status)}>{componentStatusText(component.status)}</Tag>
              </div>
              <Text className="component-message" type="secondary">
                {component.message}
              </Text>
              <Text type="secondary">{component.latencyMs} ms</Text>
            </Card>
          ))
        )}
      </section>

      <section className="two-column">
        <Card title="MySQL 表容量">
          <Table
            rowKey="tableName"
            dataSource={tableStats}
            columns={tableColumns}
            loading={loading}
            pagination={false}
            size="small"
            locale={tableEmpty('暂无表统计')}
            scroll={{ x: 520 }}
          />
        </Card>
        <Card title="Worker 近 7 天">
          <Table
            rowKey="workerName"
            dataSource={workerStats}
            columns={workerColumns}
            loading={loading}
            pagination={false}
            size="small"
            locale={tableEmpty('暂无 worker 统计')}
            scroll={{ x: 760 }}
          />
        </Card>
      </section>

      <Card title="最近错误">
        <Table
          rowKey={(row) => `${row.source}-${row.createdAt}-${row.message}`}
          dataSource={recentErrors}
          columns={errorColumns}
          loading={loading}
          pagination={{ pageSize: 8, size: 'small' }}
          size="small"
          locale={tableEmpty('暂无错误记录')}
          scroll={{ x: 760 }}
        />
      </Card>

      <Card title="脱敏 Key 清单">
        <Table
          rowKey="id"
          dataSource={keyExportRows}
          columns={exportColumns}
          loading={exporting}
          pagination={{ pageSize: 8, size: 'small' }}
          locale={tableEmpty('点击导出清单后展示脱敏结果')}
          scroll={{ x: 1250 }}
          size="small"
        />
      </Card>
    </div>
  )
}

function MetricCard({
  icon,
  label,
  value,
  color,
  loading,
}: {
  icon: ReactNode
  label: string
  value: number | string
  color: 'blue' | 'teal' | 'orange' | 'green'
  loading: boolean
}) {
  return (
    <Card loading={loading}>
      <div className="metric">
        <span className={`metric-icon ${color}`}>{icon}</span>
        <Statistic title={label} value={value} />
      </div>
    </Card>
  )
}

function tableEmpty(description: string) {
  return {
    emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={description} />,
  }
}

function MarkdownDocument({ source }: { source: string }) {
  const nodes: ReactNode[] = []
  let listItems: string[] = []
  let listType: 'ul' | 'ol' | null = null
  let codeLines: string[] = []
  let inCodeBlock = false
  let codeBlockIndex = 0

  const flushList = () => {
    if (!listType || listItems.length === 0) {
      return
    }
    const items = listItems.map((item, index) => <li key={index}>{renderMarkdownInline(item)}</li>)
    nodes.push(
      listType === 'ol' ? (
        <ol key={`list-${nodes.length}`}>{items}</ol>
      ) : (
        <ul key={`list-${nodes.length}`}>{items}</ul>
      ),
    )
    listItems = []
    listType = null
  }

  const flushCodeBlock = () => {
    nodes.push(
      <pre key={`code-${codeBlockIndex}`}>
        <code>{codeLines.join('\n')}</code>
      </pre>,
    )
    codeBlockIndex += 1
    codeLines = []
  }

  source.split(/\r?\n/).forEach((line, index) => {
    const trimmed = line.trim()

    if (trimmed.startsWith('```')) {
      if (inCodeBlock) {
        flushCodeBlock()
        inCodeBlock = false
      } else {
        flushList()
        codeLines = []
        inCodeBlock = true
      }
      return
    }

    if (inCodeBlock) {
      codeLines.push(line)
      return
    }

    if (!trimmed) {
      flushList()
      return
    }

    const heading = /^(#{1,3})\s+(.+)$/.exec(trimmed)
    if (heading) {
      flushList()
      const level = heading[1].length
      const HeadingTag = `h${level}` as 'h1' | 'h2' | 'h3'
      nodes.push(<HeadingTag key={`heading-${index}`}>{renderMarkdownInline(heading[2])}</HeadingTag>)
      return
    }

    const unordered = /^[-*]\s+(.+)$/.exec(trimmed)
    if (unordered) {
      if (listType && listType !== 'ul') {
        flushList()
      }
      listType = 'ul'
      listItems.push(unordered[1])
      return
    }

    const ordered = /^\d+\.\s+(.+)$/.exec(trimmed)
    if (ordered) {
      if (listType && listType !== 'ol') {
        flushList()
      }
      listType = 'ol'
      listItems.push(ordered[1])
      return
    }

    flushList()
    nodes.push(<p key={`paragraph-${index}`}>{renderMarkdownInline(trimmed)}</p>)
  })

  if (inCodeBlock) {
    flushCodeBlock()
  }
  flushList()

  return <div className="markdown-document">{nodes}</div>
}

function renderMarkdownInline(text: string) {
  return text.split(/(`[^`]+`)/g).map((part, index) => {
    if (part.startsWith('`') && part.endsWith('`')) {
      return <code key={index}>{part.slice(1, -1)}</code>
    }
    return part
  })
}

function ComingSoon({ page }: { page: PageKey }) {
  const labels: Record<PageKey, string> = {
    dashboard: '控制台',
    upload: '上传密钥',
    channels: '我的秘钥',
    usage: '消费快照',
    audit: '审计日志',
    ops: '运维状态',
    targets: '接收平台',
  }

  return (
    <div className="page-stack">
      <Title level={2}>{labels[page]}</Title>
      <Card>
        <Text type="secondary">下一步会实现这里。</Text>
      </Card>
    </div>
  )
}

interface KeyTableColumn {
  title: string
  placeholder: string
}

interface KeyDraftRow {
  id: string
  cells: string[]
}

let keyDraftRowSeed = 0

function KeyRowsInput({
  value = '',
  onChange,
  category,
}: {
  value?: string
  onChange?: (value: string) => void
  category?: Category
}) {
  const columns = useMemo(() => keyTableColumnsFor(category), [category])
  const columnSignature = useMemo(() => columns.map((column) => column.title).join('|'), [columns])
  const [rows, setRows] = useState<KeyDraftRow[]>(() => parseKeyRowsFromText(value, columns))
  const lastEmittedValue = useRef<string | undefined>(undefined)
  const lastColumnSignature = useRef(columnSignature)

  useEffect(() => {
    const incomingValue = value ?? ''
    if (incomingValue === lastEmittedValue.current && columnSignature === lastColumnSignature.current) {
      return
    }
    setRows(parseKeyRowsFromText(incomingValue, columns))
    lastEmittedValue.current = incomingValue
    lastColumnSignature.current = columnSignature
  }, [columnSignature, columns, value])

  const emitRows = (nextRows: KeyDraftRow[]) => {
    const normalizedRows = nextRows.length > 0 ? nextRows : [createEmptyKeyRow(columns)]
    const nextValue = serializeKeyRows(normalizedRows, columns)

    setRows(normalizedRows)
    lastEmittedValue.current = nextValue
    lastColumnSignature.current = columnSignature
    onChange?.(nextValue)
  }

  const updateCell = (rowIndex: number, cellIndex: number, nextValue: string) => {
    emitRows(
      rows.map((row, index) =>
        index === rowIndex
          ? {
              ...row,
              cells: row.cells.map((cell, nextCellIndex) => (nextCellIndex === cellIndex ? nextValue : cell)),
            }
          : row,
      ),
    )
  }

  const insertRowAfter = (rowIndex: number) => {
    const nextRows = [...rows]
    nextRows.splice(rowIndex + 1, 0, createEmptyKeyRow(columns))
    emitRows(nextRows)
  }

  const removeRow = (rowIndex: number) => {
    if (rows.length === 1) {
      emitRows([createEmptyKeyRow(columns)])
      return
    }
    emitRows(rows.filter((_, index) => index !== rowIndex))
  }

  const handleCellPaste = (event: ClipboardEvent<HTMLInputElement>, rowIndex: number) => {
    const text = event.clipboardData.getData('text')
    const shouldParseTable = text.includes('\n') || text.includes('\r') || (columns.length > 1 && text.includes('|'))
    if (!text.trim() || !shouldParseTable) {
      return
    }

    const pastedRows = parseKeyRowsFromText(text, columns).filter((row) =>
      row.cells.some((cell) => cell.trim()),
    )
    if (pastedRows.length === 0) {
      return
    }

    event.preventDefault()
    const nextRows = [...rows]
    nextRows.splice(rowIndex, 1, ...pastedRows)
    emitRows(nextRows)
  }

  const handleCellKeyDown = (event: KeyboardEvent<HTMLInputElement>, rowIndex: number) => {
    if (event.key !== 'Enter') {
      return
    }
    event.preventDefault()
    insertRowAfter(rowIndex)
  }

  const filledRows = rows.filter((row) => row.cells.some((cell) => cell.trim())).length
  const gridTemplateColumns = keyTableGridTemplate(columns)

  return (
    <div className="key-table-input">
      <div className="key-table-toolbar">
        <Text type="secondary">已填写 {filledRows} 行</Text>
      </div>
      <div className="key-table-scroll">
        <div className="key-table-row key-table-head" style={{ gridTemplateColumns }}>
          {columns.map((column) => (
            <div key={column.title}>{column.title}</div>
          ))}
          <div>操作</div>
        </div>
        <div className="key-table-body">
          {rows.map((row, rowIndex) => (
            <div className="key-table-row key-table-body-row" style={{ gridTemplateColumns }} key={row.id}>
              {columns.map((column, cellIndex) => (
                <Input
                  aria-label={`${column.title} 第 ${rowIndex + 1} 行`}
                  className="key-table-cell-input"
                  key={`${row.id}-${column.title}`}
                  placeholder={rowIndex === 0 ? column.placeholder : ''}
                  value={row.cells[cellIndex] ?? ''}
                  onChange={(event) => updateCell(rowIndex, cellIndex, event.target.value)}
                  onKeyDown={(event) => handleCellKeyDown(event, rowIndex)}
                  onPaste={(event) => handleCellPaste(event, rowIndex)}
                />
              ))}
              <div className="key-table-action">
                <Tooltip title="删除此行">
                  <Button
                    type="text"
                    size="small"
                    icon={<Trash2 size={14} />}
                    disabled={rows.length === 1 && !rows[0].cells.some((cell) => cell.trim())}
                    onClick={() => removeRow(rowIndex)}
                  />
                </Tooltip>
              </div>
            </div>
          ))}
        </div>
        <div className="key-table-add-zone">
          <Tooltip title="新增一行">
            <button
              aria-label="新增一行"
              className="key-table-add-button"
              type="button"
              onClick={() => emitRows([...rows, createEmptyKeyRow(columns)])}
            >
              <Plus size={24} strokeWidth={2.2} />
            </button>
          </Tooltip>
        </div>
      </div>
    </div>
  )
}

function keyTableColumnsFor(category?: Category): KeyTableColumn[] {
  if (category?.code === 'aws_bedrock') {
    return [
      { title: 'AccessKey', placeholder: 'AKIAxxxx' },
      { title: 'SecretKey', placeholder: 'secretxxxx' },
      { title: 'Region', placeholder: 'us-east-1' },
    ]
  }
  if (category?.code === 'azure_openai') {
    return [
      { title: 'Endpoint', placeholder: 'https://example.openai.azure.com' },
      { title: 'ApiKey', placeholder: 'api-key' },
      { title: 'ApiVersion', placeholder: '2024-12-01-preview' },
    ]
  }
  return [{ title: 'Key', placeholder: placeholderFor(category).split('\n')[0] || 'sk-...' }]
}

function keyTableGridTemplate(columns: KeyTableColumn[]) {
  const titles = columns.map((column) => column.title)
  if (titles.length === 1) {
    return 'minmax(320px, 1fr) 56px'
  }
  if (titles.includes('Region')) {
    return 'minmax(176px, 1fr) minmax(176px, 1fr) minmax(116px, 0.48fr) 56px'
  }
  return `repeat(${columns.length}, minmax(132px, 1fr)) 56px`
}

function parseKeyRowsFromText(rawText: string | undefined, columns: KeyTableColumn[]) {
  const normalizedText = String(rawText ?? '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const lines = normalizedText
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)

  if (lines.length === 0) {
    return [createEmptyKeyRow(columns)]
  }
  return lines.map((line) => createKeyRow(splitKeyLine(line, columns.length), columns))
}

function splitKeyLine(line: string, columnCount: number) {
  if (columnCount === 1) {
    return [line.trim()]
  }

  const parts = line.split('|').map((part) => part.trim())
  if (parts.length <= columnCount) {
    return parts
  }
  return [...parts.slice(0, columnCount - 1), parts.slice(columnCount - 1).join('|')]
}

function createEmptyKeyRow(columns: KeyTableColumn[]) {
  return createKeyRow([], columns)
}

function createKeyRow(cells: string[], columns: KeyTableColumn[]) {
  return {
    id: `key-row-${keyDraftRowSeed++}`,
    cells: normalizeKeyCells(cells, columns.length),
  }
}

function normalizeKeyCells(cells: string[], columnCount: number) {
  return Array.from({ length: columnCount }, (_, index) => cells[index] ?? '')
}

function serializeKeyRows(rows: KeyDraftRow[], columns: KeyTableColumn[]) {
  return rows
    .map((row) => normalizeKeyCells(row.cells, columns.length).map((cell) => cell.trim()))
    .filter((cells) => cells.some(Boolean))
    .map((cells) => (columns.length === 1 ? cells[0] : cells.join('|')))
    .join('\n')
}

function hasKeyRowsContent(value?: string) {
  return String(value ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .some((line) => line.split('|').some((part) => part.trim()))
}

function normalizePastedAWSKeys(rawText: string) {
  const rows = String(rawText ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .map((line, index) => ({ index: index + 1, line: line.trim() }))
    .filter((row) => row.line)

  if (rows.length === 0) {
    throw new Error('请粘贴至少一行密钥')
  }

  const normalizedRows = rows.map((row) => {
    const parts = row.line.split('|').map((part) => part.trim())
    if (parts.length !== 3 || parts.some((part) => !part)) {
      throw new Error(`第 ${row.index} 行格式应为 AccessKey|SecretKey|Region`)
    }
    return parts.join('|')
  })

  return {
    count: normalizedRows.length,
    rawText: normalizedRows.join('\n'),
  }
}

function parseModelText(text: string) {
  return text
    .split(/[,\n，]+/)
    .map((model) => model.trim())
    .filter(Boolean)
}

function uniqueModels(models: string[]) {
  const seen = new Set<string>()
  return models.filter((model) => {
    if (seen.has(model)) {
      return false
    }
    seen.add(model)
    return true
  })
}

function ModelTagsInput({
  value = [],
  onChange,
  placeholder,
}: {
  value?: string[]
  onChange?: (value: string[]) => void
  placeholder?: string
}) {
  const [draft, setDraft] = useState('')
  const models = uniqueModels((Array.isArray(value) ? value : []).map((model) => model.trim()).filter(Boolean))

  const emit = (nextModels: string[]) => {
    onChange?.(uniqueModels(nextModels.map((model) => model.trim()).filter(Boolean)))
  }

  const addFromText = (text: string) => {
    const nextModels = parseModelText(text)
    if (nextModels.length === 0) {
      return
    }
    const existing = new Set(models)
    const additions = nextModels.filter((model) => {
      if (existing.has(model)) {
        return false
      }
      existing.add(model)
      return true
    })
    if (additions.length > 0) {
      emit([...models, ...additions])
    }
    setDraft('')
  }

  const removeModel = (target: string) => {
    emit(models.filter((model) => model !== target))
  }

  const handleInputChange = (nextDraft: string) => {
    if (/[,\n，]/.test(nextDraft)) {
      addFromText(nextDraft)
      return
    }
    setDraft(nextDraft)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' || event.key === 'Tab' || event.key === ',' || event.key === '，') {
      if (draft.trim()) {
        event.preventDefault()
        addFromText(draft)
      }
      return
    }
    if (event.key === 'Backspace' && !draft && models.length > 0) {
      removeModel(models[models.length - 1])
    }
  }

  const handlePaste = (event: ClipboardEvent<HTMLInputElement>) => {
    const text = event.clipboardData.getData('text')
    if (!text.trim()) {
      return
    }
    event.preventDefault()
    addFromText(text)
  }

  const copyModels = async () => {
    if (models.length === 0) {
      return
    }
    try {
      await navigator.clipboard.writeText(models.join(','))
      message.success('已复制模型范围')
    } catch {
      message.error('复制失败')
    }
  }

  return (
    <div className="model-tags-input">
      <div className="model-tags-box">
        <div className="model-tags-list">
          {models.map((model) => (
            <Tag
              key={model}
              closable
              onClose={(event) => {
                event.preventDefault()
                removeModel(model)
              }}
            >
              {model}
            </Tag>
          ))}
          <input
            aria-label="模型范围"
            className="model-tags-entry"
            placeholder={models.length === 0 ? placeholder : ''}
            value={draft}
            onChange={(event) => handleInputChange(event.target.value)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
          />
        </div>
        <Tooltip title="复制全部模型">
          <Button type="text" size="small" icon={<Copy size={14} />} onClick={copyModels} disabled={models.length === 0} />
        </Tooltip>
      </div>
    </div>
  )
}

function placeholderFor(category?: Category) {
  if (!category) {
    return ''
  }
  if (category.code === 'aws_bedrock') {
    return 'AKIAxxxx|secretxxxx|us-east-1\nAKIAyyyy|secretyyyy|us-west-2'
  }
  if (category.code === 'azure_openai') {
    return 'https://example.openai.azure.com|api-key|2024-12-01-preview'
  }
  return 'sk-xxxx\nsk-yyyy'
}

function confirmDanger(title: string, content: string) {
  return new Promise<boolean>((resolve) => {
    Modal.confirm({
      title,
      content,
      okText: '确认',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: () => resolve(true),
      onCancel: () => resolve(false),
    })
  })
}

function formatDateTime(value: string) {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatQuota(value: number) {
  if (Math.abs(value) >= 1000000) {
    return `${(value / 1000000).toFixed(2)}M`
  }
  if (Math.abs(value) >= 1000) {
    return `${(value / 1000).toFixed(1)}K`
  }
  return String(value)
}

function statusColor(status: string) {
  if (status === 'active') {
    return 'green'
  }
  if (status === 'inventory') {
    return 'orange'
  }
  if (status === 'disabled' || status === 'revoked') {
    return 'red'
  }
  return undefined
}

function statusText(status: string) {
  const map: Record<string, string> = {
    active: '活跃',
    inventory: '库存',
    disabled: '停用',
    error: '异常',
    revoked: '吊销',
  }
  return map[status] ?? status
}

function syncStatusColor(status: string) {
  if (status === 'success') {
    return 'green'
  }
  if (status === 'failed') {
    return 'red'
  }
  if (status === 'pending') {
    return 'blue'
  }
  return undefined
}

function healthStatusColor(status: string) {
  if (status === 'success') {
    return 'green'
  }
  if (status === 'failed') {
    return 'red'
  }
  if (status === 'unknown') {
    return undefined
  }
  return 'blue'
}

function healthStatusText(status: string) {
  const map: Record<string, string> = {
    success: '正常',
    failed: '失败',
    skipped: '跳过',
    unknown: '未知',
  }
  return map[status] ?? status
}

function componentLabel(name: string) {
  const map: Record<string, string> = {
    database: 'MySQL',
    static: '前端静态资源',
    newApi: 'new-api 管理接口',
  }
  return map[name] ?? name
}

function componentStatusColor(status: string) {
  if (status === 'ok') {
    return 'green'
  }
  if (status === 'failed' || status === 'degraded') {
    return 'red'
  }
  if (status === 'skipped') {
    return 'blue'
  }
  return undefined
}

function componentStatusText(status: string) {
  const map: Record<string, string> = {
    ok: '正常',
    degraded: '降级',
    failed: '失败',
    skipped: '跳过',
  }
  return map[status] ?? '-'
}

function sourceText(source: string) {
  const map: Record<string, string> = {
    sync: '同步',
    health: '健康',
    worker: '任务',
    key: 'Key',
  }
  return map[source] ?? source
}

function aggregationTargetSourceText(source: string) {
  const map: Record<string, string> = {
    database: '页面配置',
    env: '环境变量',
  }
  return map[source] ?? source
}

function aggregationTargetConnectionModeText(mode?: string) {
  const map: Record<string, string> = {
    api: aggregationTargetModeLabels.api,
    new_api_reverse: aggregationTargetModeLabels.new_api_reverse,
  }
  return map[mode ?? 'api'] ?? mode ?? aggregationTargetModeLabels.api
}

function workerStatusText(status: string) {
  const map: Record<string, string> = {
    success: '成功',
    failed: '失败',
  }
  return map[status] ?? status
}
