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
  Progress,
  Radio,
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
import zhCN from 'antd/locale/zh_CN'
import {
  Activity,
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  BarChart3,
  CircleDollarSign,
  Copy,
  ClipboardPaste,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Database,
  Download,
  FileText,
  HardDrive,
  KeyRound,
  ListTree,
  LogOut,
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
  Users,
  Zap,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ClipboardEvent, type KeyboardEvent, type ReactNode } from 'react'
import ApiAccessPage from './ApiAccessPage'
import {
  activateKeyToAssociatedTargets,
  checkAggregationTargetContract,
  checkAggregationTargetReverseAdmin,
  checkAggregationTargetTokenSupply,
  createAdminUser,
  createAggregationTarget,
  createModelGap,
  deleteAggregationTarget,
  exportKeyInventory,
  getAggregationTargets,
  getChannelGroups,
  getDashboardSummary,
  getOpsStatus,
  getSession,
  getUsageLogConfiguration,
  getUsageLogs,
  importKeys,
  listAuditLogs,
  listAdminAggregationTargets,
  listAdminUsers,
  listAPIKeys,
  listModelGaps,
  login,
  logout,
  register,
  resetAdminUserPassword,
  retryKeyPush,
  revealAPIKeyPlaintext,
  updateAggregationTarget,
  updateAdminUserAggregationTargets,
  updateAdminUserRole,
  updateAdminUserStatus,
  updateModelGap,
  type AdminAggregationTarget,
  type AdminUser,
  type AdminUserAggregationTarget,
  type AggregationTarget,
  type AggregationTargetChannelType,
  type AggregationTargetConnectionMode,
  type AggregationTargetInput,
  type APIKeyRecord,
  type AWSBedrockCredentialType,
  type AuditLogRecord,
  type AuthUser,
  type ChannelGroup,
  type DashboardSummary,
  type KeyExportRecord,
  type ModelGap,
  type ModelGapInput,
  type OpsStatus,
  type RecentError,
  type TableStat,
  type UsageLogItem,
  type UsageLogPage,
  type UsageLogQuery,
  type WorkerStat,
} from './api'
const { Sider, Content } = Layout
const { Text, Title } = Typography
const { TextArea } = Input

type PageKey = 'dashboard' | 'upload' | 'channels' | 'usageLogs' | 'modelGaps' | 'audit' | 'ops' | 'targets' | 'users' | 'apiAccess'

interface UploadFormValues {
  rawText: string
  channelType?: AggregationTargetChannelType
  awsCredentialType?: AWSBedrockCredentialType
  awsBedrockRegions?: string
  models?: string[]
}

interface MultiRegionKeyFormValues {
  accessKey: string
  secretKey: string
  regions: string[]
}

interface PasteKeyFormValues {
  rawText: string
}

interface AutoOnlineProgress {
  total: number
  completed: number
  failed: number
  active: boolean
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

const AWS_BEDROCK_NEW_REGION_PREFIX = 'https://bedrock-mantle'

const emptySummary: DashboardSummary = {
  totalKeys: 0,
  activeKeys: 0,
  inventoryKeys: 0,
  disabledKeys: 0,
  totalUsageUsd: 0,
  categories: [],
}

const baseNavigationItems = [
  { key: 'channels', icon: <ListTree size={16} />, label: '我的秘钥' },
  { key: 'usageLogs', icon: <BarChart3 size={16} />, label: '使用日志' },
  { key: 'modelGaps', icon: <AlertTriangle size={16} />, label: '模型缺口' },
  { key: 'dashboard', icon: <Activity size={16} />, label: '控制台' },
  { key: 'apiAccess', icon: <KeyRound size={16} />, label: 'API 访问' },
]

const adminNavigationItems = [
  { key: 'users', icon: <Users size={16} />, label: '用户管理' },
  { key: 'audit', icon: <FileText size={16} />, label: '审计日志' },
  { key: 'ops', icon: <ServerCog size={16} />, label: '运维状态' },
  { key: 'targets', icon: <HardDrive size={16} />, label: '接收平台' },
]

const accountManagerNavigationItems = [
  { key: 'users', icon: <Users size={16} />, label: '用户管理' },
]

const defaultAuthenticatedPage: PageKey = 'channels'
const pageKeys: PageKey[] = ['dashboard', 'upload', 'channels', 'usageLogs', 'modelGaps', 'audit', 'ops', 'targets', 'users', 'apiAccess']

function pageFromLocation(): PageKey {
  const hashPage = window.location.hash.replace(/^#\/?/, '')
  return pageKeys.includes(hashPage as PageKey) ? (hashPage as PageKey) : defaultAuthenticatedPage
}

export default function App() {
  const [page, setPage] = useState<PageKey>(pageFromLocation)
  const [summary, setSummary] = useState<DashboardSummary>(emptySummary)
  const [groups, setGroups] = useState<ChannelGroup[]>([])
  const [aggregationTargets, setAggregationTargets] = useState<AggregationTarget[]>([])
  const [adminAggregationTargets, setAdminAggregationTargets] = useState<AdminAggregationTarget[]>([])
  const [adminUsers, setAdminUsers] = useState<AdminUser[]>([])
  const [modelGaps, setModelGaps] = useState<ModelGap[]>([])
  const [keyRecords, setKeyRecords] = useState<APIKeyRecord[]>([])
  const [keyTotalUsageUSD, setKeyTotalUsageUSD] = useState(0)
  const [usageLogsConfigured, setUsageLogsConfigured] = useState(false)
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
  const canManageModelGaps = !authEnabled || currentUser?.role === 'root'
  const isAccountManager = authEnabled && currentUser?.role === 'account_manager'
  const canManageUsers = canManageKeys || isAccountManager
  const navigationItems = useMemo(
    () => {
      if (canManageKeys) {
        return [...baseNavigationItems, ...adminNavigationItems]
      }
      if (isAccountManager) {
        return [...baseNavigationItems, ...accountManagerNavigationItems]
      }
      return baseNavigationItems
    },
    [canManageKeys, isAccountManager],
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
        aggregationTargetData,
        keyData,
        auditData,
        opsData,
        adminAggregationTargetData,
        adminUserData,
        modelGapData,
        usageLogConfiguration,
      ] = await Promise.all([
        getDashboardSummary(),
        getChannelGroups(),
        getAggregationTargets(),
        listAPIKeys(),
        canManageKeys ? listAuditLogs() : Promise.resolve([]),
        canManageKeys ? getOpsStatus() : Promise.resolve(null),
        canManageKeys ? listAdminAggregationTargets() : Promise.resolve([]),
        canManageUsers ? listAdminUsers() : Promise.resolve([]),
        listModelGaps(),
        getUsageLogConfiguration().catch(() => ({ configured: false })),
      ])
      setSummary(summaryData)
      setGroups(groupData)
      setAggregationTargets(aggregationTargetData)
      setKeyRecords(keyData.items)
      setKeyTotalUsageUSD(keyData.totalUsageUsd)
      setAuditLogs(auditData)
      setOpsStatus(opsData)
      setAdminAggregationTargets(adminAggregationTargetData)
      setAdminUsers(adminUserData)
      setModelGaps(modelGapData)
      setUsageLogsConfigured(usageLogConfiguration.configured)
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
    const pageHash = `#/${page}`
    if (window.location.hash !== pageHash) {
      window.history.replaceState(null, '', pageHash)
    }
  }, [page])

  useEffect(() => {
    if (authChecked && (!authEnabled || currentUser)) {
      void refresh()
    }
  }, [authChecked, authEnabled, currentUser])

  useEffect(() => {
    if (authChecked && !canManageKeys && ['audit', 'ops', 'targets'].includes(page)) {
      setPage(defaultAuthenticatedPage)
      return
    }
    if (authChecked && !canManageUsers && page === 'users') {
      setPage(defaultAuthenticatedPage)
    }
  }, [authChecked, canManageKeys, canManageUsers, page])

  const handleLogin = async (values: { username: string; password: string }) => {
    const session = await login(values)
    setAuthEnabled(session.authEnabled)
    setRegistrationEnabled(session.registrationEnabled)
    setCurrentUser(session.user ?? null)
    setPage(defaultAuthenticatedPage)
    message.success('登录成功')
  }

  const handleRegister = async (values: { username: string; password: string; displayName?: string }) => {
    const session = await register(values)
    setAuthEnabled(session.authEnabled)
    setRegistrationEnabled(session.registrationEnabled)
    setCurrentUser(session.user ?? null)
    setPage(defaultAuthenticatedPage)
    message.success('注册成功')
  }

  const handleLogout = async () => {
    await logout()
    setCurrentUser(null)
    setPage(defaultAuthenticatedPage)
    message.success('已退出')
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

  const handleSaveModelGap = async (values: ModelGapInput, id?: number) => {
    setOperationLoading('model-gap-save')
    try {
      if (id) {
        await updateModelGap(id, values)
      } else {
        await createModelGap(values)
      }
      message.success(id ? '模型缺口已更新' : '模型缺口已新增')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存模型缺口失败')
      throw err
    } finally {
      setOperationLoading(null)
    }
  }

  const handleDeleteAggregationTarget = async (code: string, name: string) => {
    const confirmed = await confirmDanger('删除接收平台', `确认删除接收平台“${name}”？`)
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

  const handleCreateAdminUser = async (values: { username: string; password: string; displayName?: string }) => {
    setOperationLoading('user-create')
    try {
      await createAdminUser(values)
      message.success('用户已创建')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建用户失败')
      throw err
    } finally {
      setOperationLoading(null)
    }
  }

  const handleUpdateAdminUserAggregationTargets = async (userId: number, targets: AdminUserAggregationTarget[]) => {
    setOperationLoading(`user-targets:${userId}`)
    try {
      await updateAdminUserAggregationTargets(userId, targets)
      message.success('用户接收平台关联已保存')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存用户接收平台关联失败')
      throw err
    } finally {
      setOperationLoading(null)
    }
  }

  const handleUpdateAdminUserStatus = async (userId: number, disabled: boolean) => {
    setOperationLoading(`user-status:${userId}`)
    try {
      await updateAdminUserStatus(userId, disabled)
      message.success(disabled ? '用户已禁用' : '用户已启用')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : disabled ? '禁用用户失败' : '启用用户失败')
      throw err
    } finally {
      setOperationLoading(null)
    }
  }

  const handleResetAdminUserPassword = async (userId: number, password: string) => {
    setOperationLoading(`user-password:${userId}`)
    try {
      await resetAdminUserPassword(userId, password)
      if (currentUser?.id === userId) {
        message.success('密码已重置，请使用新密码重新登录')
        setCurrentUser(null)
        return
      }
      message.success('用户密码已重置，原登录会话已失效')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '重置密码失败')
      throw err
    } finally {
      setOperationLoading(null)
    }
  }

  const handleUpdateAdminUserRole = async (userId: number, role: 'user' | 'account_manager') => {
    setOperationLoading(`user-role:${userId}`)
    try {
      await updateAdminUserRole(userId, role)
      message.success(role === 'account_manager' ? '已设为账号管理员' : '已设为普通用户')
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '更新用户权限失败')
      throw err
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
                loading={loading}
                onUpload={() => setPage('upload')}
              />
            ) : null}
            {page === 'upload' ? (
              <UploadPage
                aggregationTargets={aggregationTargets}
                onImported={refresh}
              />
            ) : null}
            {page === 'channels' ? (
              <KeyListPage
                keyRecords={keyRecords}
                totalUsageUSD={keyTotalUsageUSD}
                aggregationTargets={aggregationTargets}
                receiverPlatformTargets={canManageKeys ? adminAggregationTargets : aggregationTargets}
                usageLogsConfigured={usageLogsConfigured}
                loading={loading}
                canManageKeys={canManageKeys}
                showOwnerColumn={canManageKeys || isAccountManager}
                onImported={refresh}
                showHeading={false}
              />
            ) : null}
            {page === 'usageLogs' ? <UsageLogsPage /> : null}
            {page === 'apiAccess' ? <ApiAccessPage authEnabled={authEnabled} /> : null}
            {page === 'modelGaps' ? (
              <ModelGapsPage
                items={modelGaps}
                loading={loading}
                saving={operationLoading === 'model-gap-save'}
                canManage={canManageModelGaps}
                onSave={handleSaveModelGap}
              />
            ) : null}
            {page === 'audit' && canManageKeys ? <AuditPage auditLogs={auditLogs} loading={loading} /> : null}
            {page === 'users' && canManageUsers ? (
              <UserManagementPage
                users={adminUsers}
                targets={adminAggregationTargets}
                currentUserId={currentUser?.id}
                canConfigureUsers={canManageKeys}
                canChangeUserStatus={canManageUsers}
                loading={loading}
                creating={operationLoading === 'user-create'}
                operationLoading={operationLoading}
                onCreate={handleCreateAdminUser}
                onUpdateAggregationTargets={handleUpdateAdminUserAggregationTargets}
                onUpdateStatus={handleUpdateAdminUserStatus}
                onResetPassword={handleResetAdminUserPassword}
                onUpdateRole={handleUpdateAdminUserRole}
              />
            ) : null}
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
  loading,
  onUpload,
}: {
  summary: DashboardSummary
  groups: ChannelGroup[]
  loading: boolean
  onUpload: () => void
}) {
  const channelColumns = useMemo<ColumnsType<ChannelGroup>>(
    () => [
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
          <Text>库存、上线和消费概览集中在这里处理。</Text>
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
        <Card title="我的秘钥">
          <Table
            rowKey={(row) => row.tag}
            dataSource={groups}
            columns={channelColumns}
            loading={loading}
            pagination={false}
            locale={tableEmpty('暂无渠道')}
            scroll={{ x: 760 }}
          />
        </Card>
      </section>

    </div>
  )
}

function UploadPage({
  aggregationTargets,
  onImported,
}: {
  aggregationTargets: AggregationTarget[]
  onImported: () => Promise<void>
}) {
  const [form] = Form.useForm<UploadFormValues>()
  const [pasteForm] = Form.useForm<PasteKeyFormValues>()
  const [multiRegionForm] = Form.useForm<MultiRegionKeyFormValues>()
  const [importLoading, setImportLoading] = useState(false)
  const [pasteModalOpen, setPasteModalOpen] = useState(false)
  const [multiRegionModalOpen, setMultiRegionModalOpen] = useState(false)
  const [pasteLoading, setPasteLoading] = useState(false)
  const [multiRegionLoading, setMultiRegionLoading] = useState(false)
  const [modelsEdited, setModelsEdited] = useState(false)
  const [autoOnlineProgress, setAutoOnlineProgress] = useState<AutoOnlineProgress | null>(null)
  const importSubmittingRef = useRef(false)
  const authorizedAggregationTargetCount = aggregationTargets.reduce(
    (total, target) => total + (target.targetCount ?? 1),
    0,
  )
  const availableChannelTypes = useMemo(
    () => Array.from(new Set(aggregationTargets.map((target) => target.channelType))),
    [aggregationTargets],
  )
  const availableChannelTypeSignature = availableChannelTypes.join('|')
  const selectedChannelType = Form.useWatch('channelType', form)
  const selectedAggregationTargetCount = aggregationTargets
    .filter((target) => target.channelType === selectedChannelType)
    .reduce((total, target) => total + (target.targetCount ?? 1), 0)
  const selectedChannelModels =
    aggregationTargets.find((target) => target.channelType === selectedChannelType)?.models ?? []
  const requiresAWSCredentialType = selectedChannelType === 'aws_bedrock'
  const requiresAWSBedrockRegions = selectedChannelType === 'aws_bedrock_new'
  const selectedAWSCredentialType = Form.useWatch('awsCredentialType', form)
  const selectedMultiRegions = Form.useWatch('regions', multiRegionForm) ?? []

  useEffect(() => {
    const currentChannelType = form.getFieldValue('channelType') as AggregationTargetChannelType | undefined
    if (currentChannelType && !availableChannelTypes.includes(currentChannelType)) {
      form.setFieldValue('channelType', undefined)
      form.setFieldValue('models', [])
      setModelsEdited(false)
    }
  }, [availableChannelTypeSignature, form])

  const selectChannelType = (channelType: AggregationTargetChannelType) => {
    const models = aggregationTargets.find((target) => target.channelType === channelType)?.models ?? []
    form.setFieldValue('models', normalizeUploadModels(models))
    setModelsEdited(false)
  }

  const handleImport = async () => {
    if (importSubmittingRef.current) {
      message.warning('正在入库，请勿重复提交')
      return
    }

    if (authorizedAggregationTargetCount === 0) {
      message.warning('当前账号未关联任何已启用的接收平台，不能上传密钥')
      return
    }

    importSubmittingRef.current = true
    setImportLoading(true)
    setAutoOnlineProgress(null)

    let values: UploadFormValues
    try {
      values = await form.validateFields()
    } catch (err) {
      message.warning(firstFormErrorMessage(err) || '请至少填写一行密钥')
      importSubmittingRef.current = false
      setImportLoading(false)
      return
    }

    const requestedChannelTypes = values.channelType ? [values.channelType] : []
    const requestedAggregationTargetCount = aggregationTargets
      .filter((target) => requestedChannelTypes.includes(target.channelType))
      .reduce((total, target) => total + (target.targetCount ?? 1), 0)

    try {
      const result = await importKeys({
        rawText: values.rawText,
        channelTypes: requestedChannelTypes,
        awsCredentialType: requiresAWSCredentialType ? values.awsCredentialType : undefined,
        awsBedrockRegions: requiresAWSBedrockRegions
          ? normalizedMultilineRows(values.awsBedrockRegions)
          : undefined,
        models: modelsEdited ? normalizeUploadModels(values.models) : undefined,
        expectedTpm: 0,
      })
      const importedKeyIds = result.importedKeyIds ?? []
      if (authorizedAggregationTargetCount > 0 && importedKeyIds.length > 0) {
        const onlineResult = await activateImportedKeysToAssociatedTargets(
          importedKeyIds,
          requestedAggregationTargetCount,
          requestedChannelTypes,
          setAutoOnlineProgress,
        )
        if (onlineResult.failed > 0) {
          message.warning(
            `导入 ${result.imported} 个，已同步入库 ${onlineResult.success} 项，失败 ${onlineResult.failed} 项；跳过 ${result.duplicates} 个`,
          )
        } else {
          message.success(`导入 ${result.imported} 个，已同步入库 ${onlineResult.success} 项；跳过 ${result.duplicates} 个`)
        }
      } else {
        message.success(`导入 ${result.imported} 个，跳过 ${result.duplicates} 个，失败 ${result.failed} 个`)
      }
      await onImported()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '导入失败')
    } finally {
      importSubmittingRef.current = false
      setImportLoading(false)
    }
  }

  const openPasteModal = () => {
    pasteForm.setFieldsValue({ rawText: '' })
    setPasteModalOpen(true)
  }

  const closePasteModal = () => {
    if (pasteLoading) {
      return
    }
    setPasteModalOpen(false)
  }

  const handlePasteAppend = async () => {
    setPasteLoading(true)
    try {
      const values = await pasteForm.validateFields()
      const parsedText = normalizePastedKeys(values.rawText)
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

  const openMultiRegionModal = () => {
    multiRegionForm.setFieldsValue({ accessKey: '', secretKey: '', regions: [] })
    setMultiRegionModalOpen(true)
  }

  const closeMultiRegionModal = () => {
    if (multiRegionLoading) {
      return
    }
    setMultiRegionModalOpen(false)
  }

  const handleMultiRegionAppend = async () => {
    setMultiRegionLoading(true)
    try {
      const values = await multiRegionForm.validateFields()
      const lines = values.regions
        .map((region) => `${values.accessKey.trim()}|${values.secretKey.trim()}|${region}`)
        .join('\n')
      const currentRawText = String(form.getFieldValue('rawText') ?? '').trim()

      form.setFieldValue('rawText', currentRawText ? `${currentRawText}\n${lines}` : lines)
      setMultiRegionModalOpen(false)
      multiRegionForm.resetFields()
      message.success(`已生成 ${values.regions.length} 行密钥`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '添加失败')
    } finally {
      setMultiRegionLoading(false)
    }
  }

  return (
    <div className="upload-waterfall">
      <section className="upload-panel upload-intake-panel">
        {authorizedAggregationTargetCount === 0 ? (
          <Alert
            className="upload-permission-alert"
            type="warning"
            showIcon
            message="当前账号未关联任何已启用的接收平台，管理员配置关联后才可上传密钥。"
          />
        ) : null}
        <Form form={form} layout="vertical">
          {availableChannelTypes.length > 0 ? (
            <div className="upload-channel-type-selection">
              <div className="upload-channel-type-head">
                <div>
                  <Text strong>接收渠道类型</Text>
                </div>
                <Text type="secondary">
                  {selectedChannelType
                    ? `已选择 1 种，将入库到 ${selectedAggregationTargetCount} 个平台`
                    : `请选择 1 种，共 ${availableChannelTypes.length} 种可选`}
                </Text>
              </div>
              <Form.Item
                name="channelType"
                rules={[{ required: true, message: '请选择一种渠道类型' }]}
              >
                <Radio.Group
                  className="upload-channel-type-options"
                  options={availableChannelTypes.map((channelType) => ({
                    label: aggregationTargetChannelTypeText(channelType),
                    value: channelType,
                  }))}
                  onChange={(event) => selectChannelType(event.target.value as AggregationTargetChannelType)}
                />
              </Form.Item>
            </div>
          ) : null}
          {selectedChannelType ? (
            <div className="upload-model-selection">
              <Form.Item
                label="模型列表"
                name="models"
                rules={[{ required: true, type: 'array', min: 1, message: '请至少保留一个模型' }]}
              >
                <Select
                  mode="tags"
                  tokenSeparators={[',', '\n']}
                  placeholder="输入模型名称后按回车"
                  options={selectedChannelModels.map((model) => ({ label: model, value: model }))}
                  onChange={() => setModelsEdited(true)}
                />
              </Form.Item>
              {requiresAWSBedrockRegions ? (
                <Text className="upload-model-redirect-note" type="secondary">
                  备注：秘钥入库时，自定义附带重定向配置
                </Text>
              ) : null}
            </div>
          ) : null}
          {requiresAWSCredentialType ? (
            <div className="upload-aws-credential-type">
              <Alert
                type="info"
                showIcon
                message="本次选择包含旧版 AWS Bedrock，请选择对应的密钥类型。"
              />
              <Form.Item
                label="密钥类型"
                name="awsCredentialType"
                rules={[{ required: true, message: '请选择旧版 AWS Bedrock 密钥类型' }]}
              >
                <Segmented
                  block
                  options={[
                    { label: 'AccessKey / SecretAccessKey', value: 'ak_sk' },
                    { label: 'API Key', value: 'api_key' },
                  ]}
                />
              </Form.Item>
              {selectedAWSCredentialType ? (
                <Text type="secondary">
                  {selectedAWSCredentialType === 'api_key'
                    ? '每行格式：APIKey|Region'
                    : '每行格式：AccessKey|SecretAccessKey|Region'}
                </Text>
              ) : null}
            </div>
          ) : null}
          {requiresAWSBedrockRegions ? (
            <div className="upload-aws-bedrock-new-regions">
              <Form.Item
                label="区域"
                name="awsBedrockRegions"
                rules={[
                  {
                    validator: (_, value: string) => {
                      const validation = validateAWSBedrockNewRegions(value)
                      return validation.ok ? Promise.resolve() : Promise.reject(new Error(validation.message))
                    },
                  },
                ]}
              >
                <TextArea
                  rows={4}
                  placeholder={[
                    'https://bedrock-mantle.eu-north-1.api.aws/anthropic',
                    'https://bedrock-mantle.us-east-1.api.aws/anthropic',
                  ].join('\n')}
                />
              </Form.Item>
              <Text type="secondary">
                每行一个区域地址；入库时会按“区域 × 密钥”逐个创建渠道，且必须以 {AWS_BEDROCK_NEW_REGION_PREFIX} 开头。
              </Text>
            </div>
          ) : null}
          <div className="upload-key-heading">
            <span className="upload-required-label">密钥列表</span>
            <Space className="upload-key-tools" size={10}>
              <Tooltip title="粘贴多行密钥，每行作为一条原始凭据">
                <Button
                  className="upload-key-tool-button upload-key-tool-button-paste"
                  icon={<ClipboardPaste size={16} />}
                  onClick={openPasteModal}
                >
                  粘贴
                </Button>
              </Tooltip>
              {selectedChannelType !== 'aws_bedrock_new' ? (
                <Tooltip title="填写 AK、SK 并选择多个区域，合并成一行密钥">
                  <Button
                    className="upload-key-tool-button upload-key-tool-button-batch"
                    icon={<Upload size={16} />}
                    onClick={openMultiRegionModal}
                  >
                    多区域上key
                  </Button>
                </Tooltip>
              ) : null}
            </Space>
          </div>
          <Form.Item
            className="upload-key-field"
            name="rawText"
            dependencies={['awsCredentialType']}
            rules={[
              {
                validator: (_, value: string) => {
                  const validation = validateKeyRows(value)
                  if (!validation.ok) {
                    return Promise.reject(new Error(validation.message))
                  }
                  if (requiresAWSCredentialType && selectedAWSCredentialType) {
                    const awsValidation = validateAWSCredentialRows(value, selectedAWSCredentialType)
                    if (!awsValidation.ok) {
                      return Promise.reject(new Error(awsValidation.message))
                    }
                  }
                  return Promise.resolve()
                },
              },
            ]}
          >
            <KeyRowsInput />
          </Form.Item>
          <div className="upload-action-row">
            <Button
              className="upload-submit-button"
              type="primary"
              size="large"
              icon={<Upload size={16} />}
              onClick={handleImport}
              loading={importLoading}
              disabled={authorizedAggregationTargetCount === 0}
            >
              {autoOnlineProgress?.active ? '自动上线中' : importLoading ? '正在入库' : '入库'}
            </Button>
          </div>
        </Form>
      </section>

      <Modal
        title={autoOnlineProgress?.active ? '正在入库到关联接收平台' : '关联接收平台入库完成'}
        open={autoOnlineProgress !== null}
        footer={
          autoOnlineProgress && !autoOnlineProgress.active ? (
            <Button type="primary" onClick={() => setAutoOnlineProgress(null)}>
              关闭
            </Button>
          ) : null
        }
        closable={Boolean(autoOnlineProgress && !autoOnlineProgress.active)}
        maskClosable={Boolean(autoOnlineProgress && !autoOnlineProgress.active)}
        keyboard={Boolean(autoOnlineProgress && !autoOnlineProgress.active)}
        centered
        width={520}
        zIndex={1200}
        className="upload-progress-modal"
        onCancel={() => {
          if (autoOnlineProgress && !autoOnlineProgress.active) {
            setAutoOnlineProgress(null)
          }
        }}
      >
        {autoOnlineProgress ? (
          <div className="upload-auto-online-progress">
            <Space className="upload-auto-online-progress-head" align="center">
              <Text strong>{autoOnlineProgress.active ? '自动上线中，请稍候' : '自动上线处理完成'}</Text>
              <Text type="secondary">
                {autoOnlineProgress.completed}/{autoOnlineProgress.total}
                {autoOnlineProgress.failed > 0 ? `，失败 ${autoOnlineProgress.failed}` : ''}
              </Text>
            </Space>
            <Progress
              percent={
                autoOnlineProgress.total > 0
                  ? Math.round((autoOnlineProgress.completed / autoOnlineProgress.total) * 100)
                  : 0
              }
              status={autoOnlineProgress.failed > 0 ? 'exception' : autoOnlineProgress.active ? 'active' : 'success'}
            />
          </div>
        ) : null}
      </Modal>

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
            rules={[
              { required: true, whitespace: true, message: '请粘贴密钥文本' },
              {
                validator: (_, value: string) => {
                  const validation = validateKeyRows(value)
                  return validation.ok ? Promise.resolve() : Promise.reject(new Error(validation.message))
                },
              },
            ]}
          >
            <TextArea
              autoFocus
              className="paste-key-textarea"
              placeholder={'第一行密钥\n第二行密钥\n第三行密钥'}
              rows={12}
            />
          </Form.Item>
          <Text type="secondary">每个非空行会作为一个完整密钥保存，不检查前缀或格式。</Text>
        </Form>
      </Modal>

      <Modal
        title="多区域上key"
        open={multiRegionModalOpen}
        okText="合并到列表"
        cancelText="取消"
        width={860}
        className="batch-key-modal"
        confirmLoading={multiRegionLoading}
        onOk={handleMultiRegionAppend}
        onCancel={closeMultiRegionModal}
      >
        <Form form={multiRegionForm} layout="vertical" className="batch-key-form" initialValues={{ regions: [] }}>
          <Alert
            className="receiver-mode-hint"
            type="info"
            showIcon
            message="每个已选区域生成一行：AK|SK|区域"
          />
          <div className="batch-aksk-grid">
            <Form.Item label="AK" name="accessKey" rules={[{ required: true, whitespace: true, message: '请输入 AK' }]}>
              <Input autoComplete="off" placeholder="Access Key" />
            </Form.Item>
            <Form.Item label="SK" name="secretKey" rules={[{ required: true, whitespace: true, message: '请输入 SK' }]}>
              <Input.Password autoComplete="new-password" placeholder="Secret Key" />
            </Form.Item>
          </div>
          <div className="batch-region-head">
            <Text strong>区域</Text>
            <Space size={8} wrap>
              <Text type="secondary">
                已选择 {selectedMultiRegions.length}/{AWS_BEDROCK_REGIONS.length}
              </Text>
              <Button size="small" onClick={() => multiRegionForm.setFieldValue('regions', AWS_BEDROCK_REGIONS)}>
                全选
              </Button>
              <Button size="small" onClick={() => multiRegionForm.setFieldValue('regions', [])}>
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

async function activateImportedKeysToAssociatedTargets(
  keyIds: number[],
  authorizedTargetCount: number,
  channelTypes: AggregationTargetChannelType[],
  onProgress: (progress: AutoOnlineProgress) => void,
) {
  let success = 0
  let failed = 0
  let completed = 0
  const total = keyIds.length * authorizedTargetCount
  onProgress({ total, completed: 0, failed: 0, active: true })

  for (const keyId of keyIds) {
    try {
      const result = await activateKeyToAssociatedTargets(keyId, { channelTypes })
      const resultTotal = result.total ?? authorizedTargetCount
      success += result.success ?? (result.activated ? 1 : 0)
      failed += result.failed ?? 0
      completed += resultTotal
    } catch {
      failed += authorizedTargetCount
      completed += authorizedTargetCount
    }
    const nextCompleted = Math.min(completed, total)
    onProgress({ total, completed: nextCompleted, failed, active: nextCompleted < total })
  }

  onProgress({ total, completed: total, failed, active: false })
  return { success, failed }
}

function KeyListPage({
  keyRecords,
  totalUsageUSD,
  aggregationTargets,
  receiverPlatformTargets,
  usageLogsConfigured,
  loading,
  canManageKeys,
  showOwnerColumn,
  compact = false,
  onImported,
  title = '我的秘钥',
  showHeading = true,
}: {
  keyRecords: APIKeyRecord[]
  totalUsageUSD: number
  aggregationTargets: AggregationTarget[]
  receiverPlatformTargets: AggregationTarget[]
  usageLogsConfigured: boolean
  loading: boolean
  canManageKeys: boolean
  showOwnerColumn: boolean
  compact?: boolean
  onImported: () => Promise<void>
  title?: string
  showHeading?: boolean
}) {
  const [searchText, setSearchText] = useState('')
  const [ownerSearchText, setOwnerSearchText] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [uploadModalOpen, setUploadModalOpen] = useState(false)
  const [uploadModalKey, setUploadModalKey] = useState(0)
  const [retryingKeyID, setRetryingKeyID] = useState<number | null>(null)

  const openUploadModal = () => {
    setUploadModalKey((value) => value + 1)
    setUploadModalOpen(true)
  }

  const handleModalImported = async () => {
    await onImported()
    setUploadModalOpen(false)
  }

  const statusFilterOptions = useMemo(() => {
    const actualStatuses = new Map<string, string>()
    keyRecords.forEach((row) => {
      const option = keyRecordStatusFilterOption(row, aggregationTargets)
      if (option) {
        actualStatuses.set(option.value, option.label)
      }
    })
    return [
      { value: 'all', label: '全部状态' },
      ...Array.from(actualStatuses, ([value, label]) => ({ value, label })),
    ]
  }, [aggregationTargets, keyRecords])

  useEffect(() => {
    if (!statusFilterOptions.some((option) => option.value === statusFilter)) {
      setStatusFilter('all')
    }
  }, [statusFilter, statusFilterOptions])

  const visibleKeyRecords = useMemo(() => {
    if (compact) {
      return keyRecords
    }
    const keyword = searchText.trim().toLowerCase()
    const ownerKeyword = ownerSearchText.trim().toLowerCase()
    return keyRecords.filter((row) => {
      const remoteStatus = aggregateNewAPIChannelStatus(row, aggregationTargets)
      const displayedStatus = keyRecordStatusFilterOption(row, aggregationTargets)?.value
      const matchesStatus = statusFilter === 'all' || displayedStatus === statusFilter
      const matchesOwner =
        !showOwnerColumn ||
        !ownerKeyword ||
        String(row.createdBy ?? '')
          .toLowerCase()
          .includes(ownerKeyword)
      const haystack = [
        row.id,
        row.keyHint,
        row.status,
        remoteStatus?.label,
        row.newApiChannelId,
      ]
        .filter((value) => value !== undefined && value !== null)
        .join(' ')
        .toLowerCase()
      return matchesStatus && matchesOwner && (!keyword || haystack.includes(keyword))
    })
  }, [aggregationTargets, compact, keyRecords, ownerSearchText, searchText, showOwnerColumn, statusFilter])

  const keyOverview = useMemo(
    () => ({
      total: keyRecords.length,
      active: keyRecords.filter((row) => row.status === 'active').length,
      inventory: keyRecords.filter((row) => row.status === 'inventory').length,
    }),
    [keyRecords],
  )

  const runRetryPush = async (row: APIKeyRecord) => {
    setRetryingKeyID(row.id)
    try {
      const result = await retryKeyPush(row.id)
      if (result.skipped) {
        message.warning('该密钥所属用户当前没有可用的接收平台')
      } else if (result.failed > 0) {
        message.warning(`重新推送完成：成功 ${result.success} 个平台，失败 ${result.failed} 个平台`)
      } else {
        message.success(`重新推送成功，共 ${result.success} 个平台`)
      }
      await onImported()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '重新推送失败')
    } finally {
      setRetryingKeyID(null)
    }
  }

  return (
    <div className="page-stack">
      {!compact && showHeading ? <Title level={2}>{title}</Title> : null}
      <Card className="key-list-card" title={compact ? title : null}>
        {!compact ? (
          <>
            <section className="key-overview-grid" aria-label="秘钥资产概览">
              <div className="key-overview-item">
                <span className="key-overview-icon is-total">
                  <KeyRound size={18} />
                </span>
                <div>
                  <Text type="secondary">秘钥总数</Text>
                  <strong>{keyOverview.total}</strong>
                </div>
              </div>
              <div className="key-overview-item">
                <span className="key-overview-icon is-active">
                  <Activity size={18} />
                </span>
                <div>
                  <Text type="secondary">已上线</Text>
                  <strong>{keyOverview.active}</strong>
                </div>
              </div>
              <div className="key-overview-item">
                <span className="key-overview-icon is-inventory">
                  <HardDrive size={18} />
                </span>
                <div>
                  <Text type="secondary">库存待推送</Text>
                  <strong>{keyOverview.inventory}</strong>
                </div>
              </div>
              <div className="key-overview-item is-amount">
                <span className="key-overview-icon is-usage">
                  <CircleDollarSign size={18} />
                </span>
                <div>
                  <Text type="secondary">累计消耗</Text>
                  <strong>${formatTotalUsageUSD(totalUsageUSD)}</strong>
                </div>
              </div>
            </section>

            <div className="key-filter-bar">
              <div className="key-filter-fields">
                {showOwnerColumn ? (
                  <Input.Search
                    allowClear
                    aria-label="搜索用户"
                    placeholder="搜索用户"
                    value={ownerSearchText}
                    onChange={(event) => setOwnerSearchText(event.target.value)}
                    style={{ width: 260, maxWidth: '100%' }}
                  />
                ) : null}
                <Input.Search
                  allowClear
                  placeholder="搜索 Key 或渠道 ID"
                  value={searchText}
                  onChange={(event) => setSearchText(event.target.value)}
                  style={{ width: 320, maxWidth: '100%' }}
                />
                <Select
                  value={statusFilter}
                  onChange={setStatusFilter}
                  style={{ width: 140 }}
                  options={statusFilterOptions}
                />
              </div>
              <div className="key-filter-actions">
                <Text type="secondary" className="key-filter-result">
                  显示 {visibleKeyRecords.length} / {keyRecords.length}
                </Text>
                <span className="key-auto-sync">
                  <span className="key-auto-sync-dot" />
                  系统每分钟同步一次用量
                </span>
                <Button
                  className="key-detail-trigger"
                  icon={<RefreshCw size={16} />}
                  loading={loading}
                  onClick={() => void onImported()}
                >
                  刷新数据
                </Button>
                <Button
                  type="primary"
                  className="key-upload-trigger"
                  icon={<Upload size={16} />}
                  onClick={openUploadModal}
                >
                  上传秘钥
                </Button>
              </div>
            </div>
          </>
        ) : null}
        <div className="key-data-section">
          <KeyRecordList
            records={visibleKeyRecords}
            aggregationTargets={receiverPlatformTargets}
            usageLogsConfigured={usageLogsConfigured}
            loading={loading}
            compact={compact}
            canManageKeys={canManageKeys}
            showOwnerColumn={showOwnerColumn}
            retryingKeyID={retryingKeyID}
            onRetryPush={runRetryPush}
          />
        </div>
      </Card>
      <Modal
        title={
          <span className="key-upload-modal-title">
            <span className="key-upload-modal-title-icon">
              <KeyRound size={18} />
            </span>
            <span>上传秘钥</span>
          </span>
        }
        open={uploadModalOpen}
        footer={null}
        width={1080}
        className="key-upload-modal"
        destroyOnHidden
        onCancel={() => setUploadModalOpen(false)}
      >
        <UploadPage
          key={uploadModalKey}
          aggregationTargets={aggregationTargets}
          onImported={handleModalImported}
        />
      </Modal>
    </div>
  )
}

interface KeyRecordListProps {
  records: APIKeyRecord[]
  aggregationTargets: AggregationTarget[]
  usageLogsConfigured: boolean
  loading: boolean
  compact: boolean
  canManageKeys: boolean
  showOwnerColumn: boolean
  retryingKeyID: number | null
  onRetryPush: (record: APIKeyRecord) => Promise<void>
}

type KeyRecordSortKey = 'channelId' | 'usage'
type KeyRecordSortDirection = 'asc' | 'desc'

const keyRecordListHeaders = [
  { key: 'channelId', label: '渠道 ID' },
  { key: 'key', label: '密钥' },
  { key: 'usage', label: '消耗额度（$）' },
  { key: 'firstUsed', label: '首次使用时间' },
  { key: 'lastUsed', label: '最近使用时间' },
  { key: 'status', label: '状态' },
  { key: 'created', label: '创建时间' },
]

const accountManagerKeyRecordListHeaders = [
  { key: 'channelId', label: '渠道 ID' },
  { key: 'owner', label: '用户' },
  ...keyRecordListHeaders.slice(1),
]

const adminKeyRecordListHeaders = [
  { key: 'channelId', label: '渠道 ID' },
  { key: 'owner', label: '用户' },
  { key: 'key', label: '密钥' },
  { key: 'usage', label: '消耗额度（$）' },
  { key: 'firstUsed', label: '首次使用时间' },
  { key: 'lastUsed', label: '最近使用时间' },
  { key: 'status', label: '状态' },
  { key: 'created', label: '创建时间' },
  { key: 'actions', label: '操作' },
]

function KeyRecordList({
  records,
  aggregationTargets,
  usageLogsConfigured,
  loading,
  compact,
  canManageKeys,
  showOwnerColumn,
  retryingKeyID,
  onRetryPush,
}: KeyRecordListProps) {
  const pageSize = compact ? 6 : 12
  const [pageIndex, setPageIndex] = useState(1)
  const [secretRecord, setSecretRecord] = useState<APIKeyRecord | null>(null)
  const [secretPlaintext, setSecretPlaintext] = useState('')
  const [secretError, setSecretError] = useState('')
  const [secretLoading, setSecretLoading] = useState(false)
  const [usageLogRecord, setUsageLogRecord] = useState<APIKeyRecord | null>(null)
  const [sortState, setSortState] = useState<{
    key: KeyRecordSortKey
    direction: KeyRecordSortDirection
  } | null>(null)
  const secretRequestRef = useRef(0)
  const headers = canManageKeys
    ? adminKeyRecordListHeaders
    : showOwnerColumn
      ? accountManagerKeyRecordListHeaders
      : keyRecordListHeaders
  const sortedRecords = useMemo(() => {
    if (!sortState) {
      return records
    }
    const directionFactor = sortState.direction === 'asc' ? 1 : -1
    return [...records].sort((left, right) => {
      const primaryDifference =
        sortState.key === 'channelId'
          ? left.id - right.id
          : Number(left.usageUsd30d ?? 0) - Number(right.usageUsd30d ?? 0)
      if (primaryDifference !== 0) {
        return primaryDifference * directionFactor
      }
      return (left.id - right.id) * directionFactor
    })
  }, [records, sortState])
  const pageCount = Math.max(1, Math.ceil(sortedRecords.length / pageSize))
  const currentPage = Math.min(pageIndex, pageCount)
  const pageStart = (currentPage - 1) * pageSize
  const pagedRecords = sortedRecords.slice(pageStart, pageStart + pageSize)
  const startLabel = sortedRecords.length === 0 ? 0 : pageStart + 1
  const endLabel = Math.min(sortedRecords.length, pageStart + pagedRecords.length)
  const paginationItems = usageLogPaginationItems(currentPage, pageCount)
  const showSkeleton = loading && records.length === 0
  useEffect(() => {
    setPageIndex(1)
  }, [records])

  useEffect(() => {
    if (pageIndex > pageCount) {
      setPageIndex(pageCount)
    }
  }, [pageCount, pageIndex])

  useEffect(() => {
    if (!usageLogsConfigured) {
      setUsageLogRecord(null)
    }
  }, [usageLogsConfigured])

  const toggleSort = (key: KeyRecordSortKey) => {
    setSortState((current) => ({
      key,
      direction: current?.key === key && current.direction === 'asc' ? 'desc' : 'asc',
    }))
    setPageIndex(1)
  }

  const openSecretModal = async (record: APIKeyRecord) => {
    const requestID = secretRequestRef.current + 1
    secretRequestRef.current = requestID
    setSecretRecord(record)
    setSecretPlaintext('')
    setSecretError('')
    setSecretLoading(true)
    try {
      const response = await revealAPIKeyPlaintext(record.id)
      if (secretRequestRef.current === requestID) {
        setSecretPlaintext(response.plaintext)
      }
    } catch (err) {
      if (secretRequestRef.current === requestID) {
        setSecretError(err instanceof Error ? err.message : '请求失败')
      }
    } finally {
      if (secretRequestRef.current === requestID) {
        setSecretLoading(false)
      }
    }
  }

  const closeSecretModal = () => {
    secretRequestRef.current += 1
    setSecretRecord(null)
    setSecretPlaintext('')
    setSecretError('')
    setSecretLoading(false)
  }

  const copySecret = async () => {
    try {
      await navigator.clipboard.writeText(secretPlaintext)
      message.success('密钥已复制')
    } catch {
      message.error('复制失败，请手动选择密钥复制')
    }
  }

  return (
    <div className={`key-record-list ${loading ? 'is-loading' : ''}`} aria-busy={loading}>
      <div className="key-record-list-scroll">
        <div
          className={`key-record-list-grid ${canManageKeys ? 'is-admin' : showOwnerColumn ? 'has-owner' : 'without-owner'}`}
          role="table"
          aria-label="我的秘钥列表"
        >
          <div className="key-record-list-header" role="row">
            {headers.map((column) => {
              const sortable = column.key === 'channelId' || column.key === 'usage'
              const activeDirection = sortState?.key === column.key ? sortState.direction : null
              return (
                <div
                  className={`key-record-list-head-cell is-${column.key}${sortable ? ' is-sortable' : ''}`}
                  role="columnheader"
                  aria-sort={
                    activeDirection === 'asc'
                      ? 'ascending'
                      : activeDirection === 'desc'
                        ? 'descending'
                        : sortable
                          ? 'none'
                          : undefined
                  }
                  key={column.key}
                >
                  {sortable ? (
                    <button
                      className={`key-record-sort-button${activeDirection ? ' is-active' : ''}`}
                      type="button"
                      onClick={() => toggleSort(column.key as KeyRecordSortKey)}
                    >
                      <span>{column.label}</span>
                      {activeDirection === 'asc' ? (
                        <ArrowUp size={13} />
                      ) : activeDirection === 'desc' ? (
                        <ArrowDown size={13} />
                      ) : (
                        <ArrowUpDown size={13} />
                      )}
                    </button>
                  ) : (
                    column.label
                  )}
                </div>
              )
            })}
          </div>
          <div className="key-record-list-body">
            {showSkeleton
              ? Array.from({ length: 5 }, (_, index) => (
                  <div className="key-record-list-row is-skeleton" role="row" key={`key-record-skeleton-${index}`}>
                    {headers.map((column) => (
                      <div className={`key-record-list-cell is-${column.key}`} role="cell" key={column.key}>
                        <span className="key-record-skeleton-bar" />
                      </div>
                    ))}
                  </div>
                ))
              : null}

            {!showSkeleton && sortedRecords.length === 0 ? (
              <div className="key-record-list-empty">
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无 Key 记录" />
              </div>
            ) : null}

            {!showSkeleton
              ? pagedRecords.map((row) => (
                  <div
                    className={`key-record-list-row${usageLogsConfigured ? ' is-clickable' : ''}`}
                    role="row"
                    tabIndex={usageLogsConfigured ? 0 : undefined}
                    aria-label={usageLogsConfigured ? `查看渠道 ${row.id} 的使用日志` : undefined}
                    key={row.id}
                    onClick={(event) => {
                      if (!usageLogsConfigured) {
                        return
                      }
                      const target = event.target as HTMLElement
                      if (target.closest('button, a, input, textarea, select, [role="button"]')) {
                        return
                      }
                      setUsageLogRecord(row)
                    }}
                    onKeyDown={(event) => {
                      if (!usageLogsConfigured) {
                        return
                      }
                      if (event.target !== event.currentTarget || (event.key !== 'Enter' && event.key !== ' ')) {
                        return
                      }
                      event.preventDefault()
                      setUsageLogRecord(row)
                    }}
                  >
                    <div className="key-record-list-cell is-channelId" role="cell">
                      <span className="key-channel-id">{row.id}</span>
                    </div>
                    {showOwnerColumn || canManageKeys ? (
                      <div className="key-record-list-cell is-owner" role="cell">
                        <KeyOwnerDisplay value={row.createdBy} />
                      </div>
                    ) : null}
                    <div className="key-record-list-cell is-key" role="cell">
                      <KeyHintDisplay
                        value={row.keyHint}
                        clickable={canManageKeys}
                        loading={secretLoading && secretRecord?.id === row.id}
                        onClick={() => void openSecretModal(row)}
                      />
                    </div>
                    <div className="key-record-list-cell is-usage" role="cell">
                      <span className="key-record-usage-value">${formatTotalUsageUSD(row.usageUsd30d)}</span>
                    </div>
                    <div className="key-record-list-cell is-firstUsed" role="cell">
                      {formatDateTime(row.firstUsedAt ?? '')}
                    </div>
                    <div className="key-record-list-cell is-lastUsed" role="cell">
                      {formatDateTime(row.lastUsedAt ?? '')}
                    </div>
                    <div className="key-record-list-cell is-status" role="cell">
                      <KeyRecordStatus row={row} aggregationTargets={aggregationTargets} />
                    </div>
                    <div className="key-record-list-cell is-created" role="cell">
                      {formatDateTime(row.createdAt)}
                    </div>
                    {canManageKeys ? (
                      <div className="key-record-list-cell is-actions" role="cell">
                        {row.status === 'inventory' ? (
                          <Button
                            size="small"
                            type="link"
                            icon={<RefreshCw size={14} />}
                            loading={retryingKeyID === row.id}
                            disabled={retryingKeyID !== null && retryingKeyID !== row.id}
                            onClick={() => void onRetryPush(row)}
                          >
                            重新推送
                          </Button>
                        ) : (
                          <Text type="secondary">-</Text>
                        )}
                      </div>
                    ) : null}
                  </div>
                ))
              : null}
          </div>
        </div>
      </div>
      <div className="key-record-list-footer">
        <div className="key-record-list-count">
          <span>
            {startLabel}-{endLabel}
          </span>
          <Text type="secondary">共 {sortedRecords.length} 条</Text>
        </div>
        <nav className="key-record-pagination" aria-label="秘钥列表分页">
          <Button
            type="text"
            className="key-record-page-arrow"
            aria-label="上一页"
            icon={<ChevronLeft size={15} />}
            disabled={currentPage <= 1}
            onClick={() => setPageIndex((value) => Math.max(1, value - 1))}
          />
          {paginationItems.map((item, index) =>
            item === 'ellipsis' ? (
              <span className="key-record-page-ellipsis" key={`ellipsis-${index}`}>
                ···
              </span>
            ) : (
              <Button
                type={item === currentPage ? 'primary' : 'text'}
                className={`key-record-page-number${item === currentPage ? ' is-active' : ''}`}
                aria-label={`第 ${item} 页`}
                aria-current={item === currentPage ? 'page' : undefined}
                key={item}
                onClick={() => setPageIndex(item)}
              >
                {item}
              </Button>
            ),
          )}
          <Button
            type="text"
            className="key-record-page-arrow"
            aria-label="下一页"
            icon={<ChevronRight size={15} />}
            disabled={currentPage >= pageCount}
            onClick={() => setPageIndex((value) => Math.min(pageCount, value + 1))}
          />
        </nav>
      </div>
      <Modal
        title={`查看密钥${secretRecord ? ` #${secretRecord.id}` : ''}`}
        open={secretRecord !== null}
        className="key-plaintext-modal"
        width={640}
        destroyOnHidden
        onCancel={closeSecretModal}
        footer={[
          <Button key="copy" icon={<Copy size={15} />} disabled={secretLoading || !secretPlaintext} onClick={() => void copySecret()}>
            复制密钥
          </Button>,
          <Button key="close" type="primary" onClick={closeSecretModal}>
            关闭
          </Button>,
        ]}
      >
        <Alert
          type="warning"
          showIcon
          message="敏感信息"
          description="该操作已记录到审计日志。请勿在不受信任的位置粘贴或分享此密钥。"
        />
        <div className="key-plaintext-content">
          {secretError ? <Alert type="error" showIcon message="读取密钥失败" description={secretError} /> : null}
          <TextArea
            aria-label="密钥明文"
            value={secretPlaintext}
            readOnly
            autoSize={{ minRows: 3, maxRows: 10 }}
            placeholder={secretLoading ? '正在解密密钥…' : ''}
          />
        </div>
      </Modal>
      <ChannelUsageLogsModal record={usageLogRecord} onClose={() => setUsageLogRecord(null)} />
    </div>
  )
}

function KeyOwnerDisplay({ value }: { value?: string }) {
  const owner = String(value ?? '').trim()
  if (!owner) {
    return <Text type="secondary">-</Text>
  }
  return (
    <Tooltip title={owner}>
      <span className="key-record-owner">{owner}</span>
    </Tooltip>
  )
}

type RemoteChannelStatusKind = 'normal' | 'abnormal' | 'pending'

interface RemoteChannelStatusView {
  label: string
  color: string
  kind: RemoteChannelStatusKind
}

interface AggregatedChannelStatusView extends RemoteChannelStatusView {
  filterValue: 'normal' | 'abnormal' | 'pending'
  tooltip: string
}

function newAPIChannelRemoteStatus(binding: NonNullable<APIKeyRecord['pushBindings']>[number]) {
  if (binding.remoteStatus === undefined) {
    return binding.remoteStatusCheckedAt
      ? { label: '上游缺失', color: 'red', kind: 'abnormal' as const }
      : { label: '待同步', color: 'default', kind: 'pending' as const }
  }
  if (binding.remoteStatus === 1) {
    return { label: '正常', color: 'green', kind: 'normal' as const }
  }
  if (binding.remoteStatus === 2) {
    return { label: '手动禁用', color: 'orange', kind: 'abnormal' as const }
  }
  if (binding.remoteStatus === 3) {
    return { label: '自动禁用', color: 'gold', kind: 'abnormal' as const }
  }
  if (binding.remoteStatus === 0) {
    return { label: '未知', color: 'red', kind: 'abnormal' as const }
  }
  return { label: `状态 ${binding.remoteStatus}`, color: 'red', kind: 'abnormal' as const }
}

function aggregateNewAPIChannelStatus(
  row: APIKeyRecord,
  aggregationTargets: AggregationTarget[],
): AggregatedChannelStatusView | null {
  const targetByCode = new Map(aggregationTargets.map((target) => [target.code, target]))
  const entries = (row.pushBindings ?? []).flatMap((binding) => {
    const target = targetByCode.get(binding.targetCode)
    const hasSyncedRemoteStatus =
      binding.remoteStatus !== undefined || binding.remoteStatusCheckedAt !== undefined
    if (target?.connectionMode !== 'new_api_reverse' && !hasSyncedRemoteStatus) {
      return []
    }
    return [
      {
        binding,
        targetLabel: target?.name || '未知接收平台',
        status: newAPIChannelRemoteStatus(binding),
      },
    ]
  })
  if (entries.length === 0) {
    return null
  }

  const abnormalEntries = entries.filter((entry) => entry.status.kind === 'abnormal')
  const pendingEntries = entries.filter((entry) => entry.status.kind === 'pending')
  let aggregate: RemoteChannelStatusView
  let filterValue: AggregatedChannelStatusView['filterValue']
  if (abnormalEntries.length > 0) {
    const labels = new Set(abnormalEntries.map((entry) => entry.status.label))
    aggregate =
      labels.size === 1
        ? abnormalEntries[0].status
        : { label: '异常', color: 'red', kind: 'abnormal' }
    filterValue = 'abnormal'
  } else if (pendingEntries.length > 0) {
    aggregate = { label: '待同步', color: 'default', kind: 'pending' }
    filterValue = 'pending'
  } else {
    aggregate = { label: '正常', color: 'green', kind: 'normal' }
    filterValue = 'normal'
  }

  const tooltip = entries
    .map(({ binding, targetLabel, status }) => {
      const checkedAt = binding.remoteStatusCheckedAt
        ? `同步于 ${formatRemoteStatusCheckedAt(binding.remoteStatusCheckedAt)}`
        : '等待首次同步'
      return `${targetLabel}：${status.label}（渠道 #${binding.remoteChannelId}，${checkedAt}）`
    })
    .join('；')
  return { ...aggregate, filterValue, tooltip }
}

function keyRecordStatusFilterOption(
  row: APIKeyRecord,
  aggregationTargets: AggregationTarget[],
): { value: string; label: string } | null {
  if (row.status !== 'active') {
    return {
      value: `local:${row.status}`,
      label: statusText(row.status),
    }
  }
  const remoteStatus = aggregateNewAPIChannelStatus(row, aggregationTargets)
  if (!remoteStatus) {
    return null
  }
  return {
    value: `remote:${remoteStatus.label}`,
    label: remoteStatus.label,
  }
}

function KeyRecordStatus({
  row,
  aggregationTargets,
}: {
  row: APIKeyRecord
  aggregationTargets: AggregationTarget[]
}) {
  if (row.status !== 'active') {
    return <Tag color={statusColor(row.status)}>{statusText(row.status)}</Tag>
  }
  const remoteStatus = aggregateNewAPIChannelStatus(row, aggregationTargets)
  if (!remoteStatus) {
    return <Text type="secondary">-</Text>
  }
  return (
    <Tooltip title={remoteStatus.tooltip}>
      <Tag color={remoteStatus.color}>{remoteStatus.label}</Tag>
    </Tooltip>
  )
}

function formatRemoteStatusCheckedAt(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString('zh-CN', { hour12: false })
}

function KeyHintDisplay({
  value,
  clickable = false,
  loading = false,
  onClick,
}: {
  value: string
  clickable?: boolean
  loading?: boolean
  onClick?: () => void
}) {
  const keyHint = String(value || '').trim()

  if (!keyHint) {
    return <Text type="secondary">-</Text>
  }

  if (clickable) {
    return (
      <Tooltip title="点击查看密钥明文">
        <button className="key-record-secret is-clickable" type="button" disabled={loading} onClick={onClick}>
          {loading ? '读取中…' : keyHint}
        </button>
      </Tooltip>
    )
  }

  return <span className="key-record-secret">{keyHint}</span>
}

interface UsageLogFilterValues {
  startTime?: string
  endTime?: string
  channelId?: number
  type?: number
  modelName?: string
  requestId?: string
  upstreamRequestId?: string
}

function usageLogDateTimeLocal(date: Date) {
  const localTime = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return localTime.toISOString().slice(0, 16)
}

function defaultUsageLogTimeRange(): Pick<UsageLogFilterValues, 'startTime' | 'endTime'> {
  const endTime = new Date()
  const startTime = new Date(endTime.getTime() - 60 * 60 * 1000)
  return {
    startTime: usageLogDateTimeLocal(startTime),
    endTime: usageLogDateTimeLocal(endTime),
  }
}

const usageLogTypeOptions = [
  { value: 0, label: '全部类型' },
  { value: 2, label: '消费' },
  { value: 5, label: '错误' },
  { value: 6, label: '退款' },
]

function usageLogTypeDisplay(type: number) {
  switch (type) {
    case 2:
      return { label: '消费', color: 'blue' }
    case 5:
      return { label: '错误', color: 'red' }
    case 6:
      return { label: '退款', color: 'orange' }
    default:
      return { label: `类型 ${type}`, color: 'default' }
  }
}

function usageLogTimestamp(value: string | undefined) {
  if (!value) {
    return undefined
  }
  const timestamp = new Date(value).getTime()
  return Number.isNaN(timestamp) ? undefined : Math.floor(timestamp / 1000)
}

function formatUsageLogTime(timestamp: number) {
  if (!timestamp) {
    return '-'
  }
  return new Date(timestamp * 1000).toLocaleString('zh-CN', { hour12: false })
}

function usageLogPaginationItems(currentPage: number, maximumPage: number): Array<number | 'ellipsis'> {
  if (maximumPage <= 0) {
    return []
  }
  if (maximumPage <= 7) {
    return Array.from({ length: maximumPage }, (_, index) => index + 1)
  }
  const pages = new Set([1, maximumPage, currentPage - 2, currentPage - 1, currentPage, currentPage + 1, currentPage + 2])
  const sortedPages = Array.from(pages)
    .filter((page) => page >= 1 && page <= maximumPage)
    .sort((left, right) => left - right)
  const items: Array<number | 'ellipsis'> = []
  sortedPages.forEach((page, index) => {
    if (index > 0 && page - sortedPages[index - 1] > 1) {
      items.push('ellipsis')
    }
    items.push(page)
  })
  return items
}

interface ChannelUsageLogFilterValues {
  startTime?: string
  endTime?: string
}

function ChannelUsageLogsModal({
  record,
  onClose,
}: {
  record: APIKeyRecord | null
  onClose: () => void
}) {
  const [form] = Form.useForm<ChannelUsageLogFilterValues>()
  const [result, setResult] = useState<UsageLogPage | null>(null)
  const [loading, setLoading] = useState(false)
  const [pageError, setPageError] = useState<string | null>(null)
  const [activeQuery, setActiveQuery] = useState<UsageLogQuery | null>(null)
  const [cursorStack, setCursorStack] = useState<string[]>([''])
  const [pageIndex, setPageIndex] = useState(0)
  const requestIDRef = useRef(0)

  const load = async (query: UsageLogQuery, cursor = '') => {
    const requestID = requestIDRef.current + 1
    requestIDRef.current = requestID
    setLoading(true)
    setPageError(null)
    try {
      const response = await getUsageLogs({ ...query, cursor: cursor || undefined })
      if (requestIDRef.current === requestID) {
        setResult(response)
      }
    } catch (err) {
      if (requestIDRef.current === requestID) {
        setPageError(err instanceof Error ? err.message : '渠道使用日志加载失败')
      }
    } finally {
      if (requestIDRef.current === requestID) {
        setLoading(false)
      }
    }
  }

  useEffect(() => {
    requestIDRef.current += 1
    if (!record) {
      setResult(null)
      setPageError(null)
      setLoading(false)
      setActiveQuery(null)
      setCursorStack([''])
      setPageIndex(0)
      return
    }
    const timeRange = defaultUsageLogTimeRange()
    form.setFieldsValue(timeRange)
    const query: UsageLogQuery = {
      pageSize: 10,
      channelId: record.id,
      startTimestamp: usageLogTimestamp(timeRange.startTime),
      endTimestamp: usageLogTimestamp(timeRange.endTime),
    }
    setResult(null)
    setActiveQuery(query)
    setCursorStack([''])
    setPageIndex(0)
    void load(query)
  }, [record?.id])

  const search = async () => {
    if (!record) {
      return
    }
    const values = await form.validateFields()
    const startTimestamp = usageLogTimestamp(values.startTime)
    const endTimestamp = usageLogTimestamp(values.endTime)
    if (startTimestamp && endTimestamp && startTimestamp > endTimestamp) {
      setPageError('开始时间不能晚于结束时间')
      return
    }
    const query: UsageLogQuery = {
      pageSize: 10,
      channelId: record.id,
      startTimestamp,
      endTimestamp,
    }
    setActiveQuery(query)
    setCursorStack([''])
    setPageIndex(0)
    await load(query)
  }

  const reset = async () => {
    if (!record) {
      return
    }
    const timeRange = defaultUsageLogTimeRange()
    form.setFieldsValue(timeRange)
    const query: UsageLogQuery = {
      pageSize: 10,
      channelId: record.id,
      startTimestamp: usageLogTimestamp(timeRange.startTime),
      endTimestamp: usageLogTimestamp(timeRange.endTime),
    }
    setActiveQuery(query)
    setCursorStack([''])
    setPageIndex(0)
    await load(query)
  }

  const nextPage = async () => {
    if (!activeQuery) {
      return
    }
    const nextIndex = pageIndex + 1
    const knownCursor = cursorStack[nextIndex]
    if (knownCursor !== undefined) {
      setPageIndex(nextIndex)
      await load(activeQuery, knownCursor)
      return
    }
    const nextCursor = result?.nextCursor
    if (!nextCursor) {
      return
    }
    setCursorStack((current) => [...current.slice(0, nextIndex), nextCursor])
    setPageIndex(nextIndex)
    await load(activeQuery, nextCursor)
  }

  const previousPage = async () => {
    if (!activeQuery || pageIndex <= 0) {
      return
    }
    const previousIndex = pageIndex - 1
    setPageIndex(previousIndex)
    await load(activeQuery, cursorStack[previousIndex] ?? '')
  }

  const goToPage = async (page: number) => {
    if (!activeQuery) {
      return
    }
    const targetIndex = page - 1
    if (targetIndex === pageIndex || targetIndex < 0) {
      return
    }
    if (targetIndex === cursorStack.length && pageIndex === cursorStack.length - 1 && result?.nextCursor) {
      await nextPage()
      return
    }
    const cursor = cursorStack[targetIndex]
    if (cursor === undefined) {
      return
    }
    setPageIndex(targetIndex)
    await load(activeQuery, cursor)
  }

  const columns = useMemo<ColumnsType<UsageLogItem>>(
    () => [
      {
        title: '时间',
        dataIndex: 'createdAt',
        width: 170,
        render: (value: number) => <span className="usage-log-nowrap">{formatUsageLogTime(value)}</span>,
      },
      {
        title: '类型',
        dataIndex: 'type',
        width: 80,
        render: (value: number) => {
          const display = usageLogTypeDisplay(value)
          return <Tag color={display.color}>{display.label}</Tag>
        },
      },
      {
        title: '模型',
        dataIndex: 'modelName',
        width: 190,
        render: (value: string) => (value ? <Tag color="gold">{value}</Tag> : <Text type="secondary">-</Text>),
      },
      {
        title: '输入 / 输出',
        width: 210,
        render: (_, row) => (
          <Text className="usage-log-nowrap">
            {row.promptTokens.toLocaleString()} / {row.completionTokens.toLocaleString()}
            {row.cacheReadTokens > 0 || row.cacheCreationTokens > 0
              ? ` · 缓存 ${row.cacheReadTokens.toLocaleString()} / ${row.cacheCreationTokens.toLocaleString()}`
              : ''}
          </Text>
        ),
      },
      {
        title: '用时 / 首字',
        width: 170,
        render: (_, row) => (
          <Text className="usage-log-nowrap">
            {row.useTime > 0 ? `${row.useTime} s` : '-'} · {row.firstResponseTimeMs > 0 ? `${row.firstResponseTimeMs} ms` : '-'}
          </Text>
        ),
      },
      {
        title: '消费',
        dataIndex: 'quotaUsd',
        width: 120,
        render: (value: number) => `$${value.toFixed(6)}`,
      },
      {
        title: '模式',
        dataIndex: 'isStream',
        width: 80,
        render: (value: boolean) => <Tag color={value ? 'purple' : 'default'}>{value ? '流式' : '非流'}</Tag>,
      },
      {
        title: 'Request ID',
        dataIndex: 'requestId',
        width: 260,
        render: (value?: string) => (
          <Text className="usage-log-nowrap" copyable={Boolean(value)} ellipsis={value ? { tooltip: value } : false}>
            {value || '-'}
          </Text>
        ),
      },
      {
        title: '详情',
        dataIndex: 'content',
        width: 320,
        render: (value?: string, row?: UsageLogItem) => {
          const detail = [value, row?.streamStatus, row?.streamEndReason].filter(Boolean).join(' · ')
          return detail ? (
            <Tooltip title={detail}>
              <Text ellipsis>{detail}</Text>
            </Tooltip>
          ) : (
            <Text type="secondary">-</Text>
          )
        },
      },
    ],
    [],
  )

  const totalPages = result?.total ? Math.ceil(result.total / (result.pageSize || 10)) : 0
  const accessiblePageCount = Math.min(
    totalPages,
    cursorStack.length + (pageIndex === cursorStack.length - 1 && result?.nextCursor ? 1 : 0),
  )
  const paginationItems = usageLogPaginationItems(pageIndex + 1, accessiblePageCount)

  return (
    <Modal
      className="channel-usage-modal"
      width={1420}
      open={record !== null}
      footer={null}
      destroyOnHidden
      onCancel={onClose}
      title={
        <div className="channel-usage-modal-title">
          <span className="channel-usage-modal-icon">
            <BarChart3 size={18} />
          </span>
          <div>
            <strong>渠道 {record?.id ?? '-'} 使用日志</strong>
            <span>
              {record?.createdBy || '未知用户'} · {record?.keyHint || '无密钥摘要'}
            </span>
          </div>
        </div>
      }
    >
      {pageError ? <Alert type="error" showIcon message="使用日志加载失败" description={pageError} /> : null}
      {result && !result.configured ? (
        <Alert type="warning" showIcon message={result.message || '系统默认接收平台未配置'} />
      ) : null}

      <div className="usage-log-stats channel-usage-log-stats">
        <Statistic title="消费额度" value={result?.stats.quotaUsd ?? 0} precision={6} prefix="$" />
        <Statistic title="RPM" value={result?.stats.rpm ?? 0} />
        <Statistic title="TPM" value={result?.stats.tpm ?? 0} />
        <Statistic title="日志总数" value={result?.total ?? 0} />
      </div>

      <Form
        form={form}
        className="channel-usage-filter-form"
        layout="vertical"
        onFinish={() => void search()}
      >
        <div className="channel-usage-filter-bar">
          <Form.Item label="开始时间" name="startTime">
            <Input type="datetime-local" />
          </Form.Item>
          <Form.Item label="结束时间" name="endTime">
            <Input type="datetime-local" />
          </Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>
              查询
            </Button>
            <Button disabled={loading} onClick={() => void reset()}>
              最近一小时
            </Button>
          </Space>
        </div>
      </Form>

      <Table
        className="usage-log-table channel-usage-log-table"
        rowKey={(row) => `${row.createdAt}:${row.channelId}:${row.requestId || ''}:${row.upstreamRequestId || ''}`}
        dataSource={result?.items ?? []}
        columns={columns}
        loading={loading}
        pagination={false}
        tableLayout="fixed"
        locale={tableEmpty(result?.configured === false ? '请先配置系统默认接收平台' : '该时间段暂无使用日志')}
        scroll={{ x: 1600, y: 430 }}
      />

      <div className="usage-log-pagination">
        <Text type="secondary">
          第 {pageIndex + 1} 页{totalPages > 0 ? ` / 共 ${totalPages} 页` : ''}，共 {result?.total ?? 0} 条
        </Text>
        <div className="usage-log-page-controls">
          <Button disabled={pageIndex <= 0 || loading} onClick={() => void previousPage()}>
            上一页
          </Button>
          {paginationItems.map((item, index) =>
            item === 'ellipsis' ? (
              <span className="usage-log-page-ellipsis" key={`ellipsis-${index}`}>…</span>
            ) : (
              <Button
                className="usage-log-page-number"
                type={item === pageIndex + 1 ? 'primary' : 'default'}
                aria-current={item === pageIndex + 1 ? 'page' : undefined}
                disabled={loading}
                key={item}
                onClick={() => void goToPage(item)}
              >
                {item}
              </Button>
            ),
          )}
          {accessiblePageCount < totalPages ? <span className="usage-log-page-ellipsis">…</span> : null}
          <Button
            disabled={(pageIndex + 1 >= cursorStack.length && !result?.nextCursor) || loading}
            onClick={() => void nextPage()}
          >
            下一页
          </Button>
        </div>
      </div>
    </Modal>
  )
}

function UsageLogsPage() {
  const [form] = Form.useForm<UsageLogFilterValues>()
  const [initialTimeRange] = useState(defaultUsageLogTimeRange)
  const [result, setResult] = useState<UsageLogPage | null>(null)
  const [loading, setLoading] = useState(false)
  const [pageError, setPageError] = useState<string | null>(null)
  const [activeQuery, setActiveQuery] = useState<UsageLogQuery>(() => ({
    pageSize: 20,
    startTimestamp: usageLogTimestamp(initialTimeRange.startTime),
    endTimestamp: usageLogTimestamp(initialTimeRange.endTime),
  }))
  const [cursorStack, setCursorStack] = useState<string[]>([''])
  const [pageIndex, setPageIndex] = useState(0)

  const load = async (query: UsageLogQuery, cursor = '') => {
    setLoading(true)
    setPageError(null)
    try {
      const response = await getUsageLogs({ ...query, cursor: cursor || undefined })
      setResult(response)
    } catch (err) {
      setPageError(err instanceof Error ? err.message : '使用日志加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(activeQuery)
  }, [])

  const search = async () => {
    const values = await form.validateFields()
    const query: UsageLogQuery = {
      pageSize: 20,
      type: values.type || undefined,
      modelName: values.modelName?.trim() || undefined,
      startTimestamp: usageLogTimestamp(values.startTime),
      endTimestamp: usageLogTimestamp(values.endTime),
      channelId: values.channelId || undefined,
      requestId: values.requestId?.trim() || undefined,
      upstreamRequestId: values.upstreamRequestId?.trim() || undefined,
    }
    setActiveQuery(query)
    setCursorStack([''])
    setPageIndex(0)
    await load(query)
  }

  const reset = async () => {
    const timeRange = defaultUsageLogTimeRange()
    form.setFieldsValue({ ...timeRange, type: 0 })
    const query: UsageLogQuery = {
      pageSize: 20,
      startTimestamp: usageLogTimestamp(timeRange.startTime),
      endTimestamp: usageLogTimestamp(timeRange.endTime),
    }
    setActiveQuery(query)
    setCursorStack([''])
    setPageIndex(0)
    await load(query)
  }

  const nextPage = async () => {
    const nextIndex = pageIndex + 1
    const knownCursor = cursorStack[nextIndex]
    if (knownCursor !== undefined) {
      setPageIndex(nextIndex)
      await load(activeQuery, knownCursor)
      return
    }
    const nextCursor = result?.nextCursor
    if (!nextCursor) {
      return
    }
    setCursorStack((current) => [...current.slice(0, nextIndex), nextCursor])
    setPageIndex(nextIndex)
    await load(activeQuery, nextCursor)
  }

  const previousPage = async () => {
    if (pageIndex <= 0) {
      return
    }
    const previousIndex = pageIndex - 1
    setPageIndex(previousIndex)
    await load(activeQuery, cursorStack[previousIndex] ?? '')
  }

  const goToPage = async (page: number) => {
    const targetIndex = page - 1
    if (targetIndex === pageIndex || targetIndex < 0) {
      return
    }
    if (targetIndex === cursorStack.length && pageIndex === cursorStack.length - 1 && result?.nextCursor) {
      await nextPage()
      return
    }
    const cursor = cursorStack[targetIndex]
    if (cursor === undefined) {
      return
    }
    setPageIndex(targetIndex)
    await load(activeQuery, cursor)
  }

  const columns = useMemo<ColumnsType<UsageLogItem>>(
    () => [
      {
        title: '时间',
        dataIndex: 'createdAt',
        width: 180,
        render: (value: number) => <span className="usage-log-nowrap">{formatUsageLogTime(value)}</span>,
      },
      {
        title: '渠道id',
        dataIndex: 'channelId',
        width: 100,
        render: (value: number) => <Text code>{value}</Text>,
      },
      {
        title: '类型',
        dataIndex: 'type',
        width: 90,
        render: (value: number) => {
          const display = usageLogTypeDisplay(value)
          return <Tag color={display.color}>{display.label}</Tag>
        },
      },
      {
        title: '模型',
        dataIndex: 'modelName',
        width: 210,
        render: (value: string) => (value ? <Tag color="gold">{value}</Tag> : <Text type="secondary">-</Text>),
      },
      {
        title: '用时/首字',
        width: 180,
        render: (_, row) => (
          <Text className="usage-log-nowrap">
            {row.useTime > 0 ? `${row.useTime} s` : '-'} · {row.firstResponseTimeMs > 0 ? `首字 ${row.firstResponseTimeMs} ms` : '首字 -'}
          </Text>
        ),
      },
      {
        title: '输入/输出',
        width: 210,
        render: (_, row) => (
          <Text className="usage-log-nowrap">
            {row.promptTokens.toLocaleString()} / {row.completionTokens.toLocaleString()}
            {row.cacheReadTokens > 0 || row.cacheCreationTokens > 0
              ? ` · 缓存 ${row.cacheReadTokens.toLocaleString()} / ${row.cacheCreationTokens.toLocaleString()}`
              : ''}
          </Text>
        ),
      },
      {
        title: '消费',
        dataIndex: 'quotaUsd',
        width: 120,
        render: (value: number) => `$${value.toFixed(6)}`,
      },
      {
        title: '模式',
        dataIndex: 'isStream',
        width: 90,
        render: (value: boolean) => <Tag color={value ? 'purple' : 'default'}>{value ? '流式' : '非流'}</Tag>,
      },
      {
        title: 'Request ID',
        dataIndex: 'requestId',
        width: 300,
        render: (value?: string) => (
          <Text className="usage-log-nowrap" copyable={Boolean(value)} ellipsis={value ? { tooltip: value } : false}>
            {value || '-'}
          </Text>
        ),
      },
      {
        title: '详情',
        dataIndex: 'content',
        width: 420,
        render: (value?: string, row?: UsageLogItem) => {
          const detail = [value, row?.streamStatus, row?.streamEndReason].filter(Boolean).join(' · ')
          return detail ? (
            <Tooltip title={detail}>
              <Text ellipsis>{detail}</Text>
            </Tooltip>
          ) : (
            <Text type="secondary">-</Text>
          )
        },
      },
    ],
    [],
  )

  const totalPages = result?.total ? Math.ceil(result.total / (result.pageSize || 20)) : 0
  const accessiblePageCount = Math.min(
    totalPages,
    cursorStack.length + (pageIndex === cursorStack.length - 1 && result?.nextCursor ? 1 : 0),
  )
  const paginationItems = usageLogPaginationItems(pageIndex + 1, accessiblePageCount)

  return (
    <div className="page-stack">
      <section className="page-heading">
        <div>
          <Title level={2}>使用日志</Title>
        </div>
        <Button icon={<RefreshCw size={16} />} loading={loading} onClick={() => void load(activeQuery, cursorStack[pageIndex] ?? '')}>
          刷新
        </Button>
      </section>

      {pageError ? <Alert type="error" showIcon message="使用日志加载失败" description={pageError} /> : null}
      {result && !result.configured ? <Alert type="warning" showIcon message={result.message || '系统默认接收平台未配置'} /> : null}

      <Card>
        <div className="usage-log-stats">
          <Statistic title="消费额度" value={result?.stats.quotaUsd ?? 0} precision={6} prefix="$" />
          <Statistic title="RPM" value={result?.stats.rpm ?? 0} />
          <Statistic title="TPM" value={result?.stats.tpm ?? 0} />
          <Statistic title="日志总数" value={result?.total ?? 0} />
        </div>

        <Form form={form} layout="vertical" initialValues={{ ...initialTimeRange, type: 0 }} onFinish={() => void search()}>
          <div className="usage-log-filters">
            <Form.Item label="开始时间" name="startTime">
              <Input type="datetime-local" />
            </Form.Item>
            <Form.Item label="结束时间" name="endTime">
              <Input type="datetime-local" />
            </Form.Item>
            <Form.Item label="渠道id" name="channelId">
              <Select
                allowClear
                showSearch
                placeholder="全部渠道"
                options={(result?.channels ?? []).map((channelId) => ({ value: channelId, label: String(channelId) }))}
              />
            </Form.Item>
            <Form.Item label="类型" name="type">
              <Select options={usageLogTypeOptions} />
            </Form.Item>
            <Form.Item label="模型名称" name="modelName">
              <Input allowClear placeholder="精确匹配模型名称" />
            </Form.Item>
            <Form.Item label="Request ID" name="requestId">
              <Input allowClear />
            </Form.Item>
            <Form.Item label="Upstream Request ID" name="upstreamRequestId">
              <Input allowClear />
            </Form.Item>
          </div>
          <Space className="usage-log-filter-actions">
            <Button type="primary" htmlType="submit" loading={loading}>查询</Button>
            <Button onClick={() => void reset()}>重置</Button>
          </Space>
        </Form>
      </Card>

      <Card>
        <Table
          className="usage-log-table"
          rowKey={(row) => `${row.createdAt}:${row.channelId}:${row.requestId || ''}:${row.upstreamRequestId || ''}`}
          dataSource={result?.items ?? []}
          columns={columns}
          loading={loading}
          pagination={false}
          tableLayout="fixed"
          locale={tableEmpty(result?.configured === false ? '请先配置系统默认接收平台' : '暂无使用日志')}
          scroll={{ x: 1900 }}
        />
        <div className="usage-log-pagination">
          <Text type="secondary">
            第 {result?.total ? pageIndex + 1 : 0} 页{totalPages > 0 ? ` / 共 ${totalPages} 页` : ''}，共 {result?.total ?? 0} 条
          </Text>
          <div className="usage-log-page-controls">
            <Button disabled={pageIndex <= 0 || loading} onClick={() => void previousPage()}>上一页</Button>
            {paginationItems.map((item, index) =>
              item === 'ellipsis' ? (
                <span className="usage-log-page-ellipsis" key={`ellipsis-${index}`}>…</span>
              ) : (
                <Button
                  className="usage-log-page-number"
                  type={item === pageIndex + 1 ? 'primary' : 'default'}
                  aria-current={item === pageIndex + 1 ? 'page' : undefined}
                  disabled={loading}
                  key={item}
                  onClick={() => void goToPage(item)}
                >
                  {item}
                </Button>
              ),
            )}
            {accessiblePageCount < totalPages ? <span className="usage-log-page-ellipsis">…</span> : null}
            <Button
              disabled={(pageIndex + 1 >= cursorStack.length && !result?.nextCursor) || loading}
              onClick={() => void nextPage()}
            >
              下一页
            </Button>
          </div>
        </div>
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

interface AdminUserCreateFormValues {
  username: string
  password: string
  confirmPassword: string
  displayName?: string
}

interface AdminUserTargetFormValues {
  targetCodes: string[]
}

interface AdminUserPasswordFormValues {
  password: string
  confirmPassword: string
}

interface AdminUserRoleFormValues {
  role: 'user' | 'account_manager'
}

function UserManagementPage({
  users,
  targets,
  currentUserId,
  canConfigureUsers,
  canChangeUserStatus,
  loading,
  creating,
  operationLoading,
  onCreate,
  onUpdateAggregationTargets,
  onUpdateStatus,
  onResetPassword,
  onUpdateRole,
}: {
  users: AdminUser[]
  targets: AdminAggregationTarget[]
  currentUserId?: number
  canConfigureUsers: boolean
  canChangeUserStatus: boolean
  loading: boolean
  creating: boolean
  operationLoading: string | null
  onCreate: (values: { username: string; password: string; displayName?: string }) => Promise<void>
  onUpdateAggregationTargets: (userId: number, targets: AdminUserAggregationTarget[]) => Promise<void>
  onUpdateStatus: (userId: number, disabled: boolean) => Promise<void>
  onResetPassword: (userId: number, password: string) => Promise<void>
  onUpdateRole: (userId: number, role: 'user' | 'account_manager') => Promise<void>
}) {
  const [form] = Form.useForm<AdminUserCreateFormValues>()
  const [targetForm] = Form.useForm<AdminUserTargetFormValues>()
  const [passwordForm] = Form.useForm<AdminUserPasswordFormValues>()
  const [roleForm] = Form.useForm<AdminUserRoleFormValues>()
  const [modalOpen, setModalOpen] = useState(false)
  const [targetUser, setTargetUser] = useState<AdminUser | null>(null)
  const [passwordUser, setPasswordUser] = useState<AdminUser | null>(null)
  const [roleUser, setRoleUser] = useState<AdminUser | null>(null)
  const [searchText, setSearchText] = useState('')
  const [currentPage, setCurrentPage] = useState(1)
  const selectedTargetCodes = Form.useWatch('targetCodes', targetForm) ?? []
  const selectedTargetCodeSet = useMemo(() => new Set(selectedTargetCodes), [selectedTargetCodes])
  const configurableTargets = useMemo(
    () => targets.filter((target) => target.source === 'database'),
    [targets],
  )
  const normalizedSearchText = searchText.trim().toLowerCase()
  const visibleUsers = useMemo(() => {
    if (!normalizedSearchText) {
      return users
    }
    return users.filter((user) =>
      [
        String(user.id),
        `#${user.id}`,
        user.username,
        user.displayName,
      ].some((value) => value?.toLowerCase().includes(normalizedSearchText)),
    )
  }, [normalizedSearchText, users])

  const openCreate = () => {
    form.resetFields()
    setModalOpen(true)
  }

  const submit = async () => {
    const { confirmPassword, ...values } = await form.validateFields()
    void confirmPassword
    await onCreate({
      username: values.username.trim(),
      password: values.password,
      displayName: values.displayName?.trim() || undefined,
    })
    setModalOpen(false)
    form.resetFields()
  }

  const openTargetAssociations = (user: AdminUser) => {
    setTargetUser(user)
    targetForm.resetFields()
    const existingTargets = user.aggregationTargets?.length
      ? user.aggregationTargets
      : (user.aggregationTargetCodes ?? []).map((targetCode) => ({ targetCode }))
    targetForm.setFieldsValue({
      targetCodes: existingTargets.map((target) => target.targetCode),
    })
  }

  const submitTargetAssociations = async () => {
    if (!targetUser) {
      return
    }
    const values = await targetForm.validateFields()
    await onUpdateAggregationTargets(
      targetUser.id,
      (values.targetCodes ?? []).map((targetCode) => ({ targetCode })),
    )
    setTargetUser(null)
    targetForm.resetFields()
  }

  const openPasswordReset = (user: AdminUser) => {
    passwordForm.resetFields()
    setPasswordUser(user)
  }

  const submitPasswordReset = async () => {
    if (!passwordUser) {
      return
    }
    const { password } = await passwordForm.validateFields()
    await onResetPassword(passwordUser.id, password)
    setPasswordUser(null)
    passwordForm.resetFields()
  }

  const handleUserStatusAction = async (user: AdminUser) => {
    if (user.disabled) {
      await onUpdateStatus(user.id, false)
      return
    }
    const confirmed = await confirmDanger(
      '禁用用户',
      `确认禁用用户“${user.username}”？该用户现有登录会话会立即失效。`,
    )
    if (confirmed) {
      await onUpdateStatus(user.id, true)
    }
  }

  const openRoleConfiguration = (user: AdminUser) => {
    roleForm.setFieldsValue({
      role: user.role === 'account_manager' ? 'account_manager' : 'user',
    })
    setRoleUser(user)
  }

  const submitRoleConfiguration = async () => {
    if (!roleUser) {
      return
    }
    const values = await roleForm.validateFields()
    await onUpdateRole(roleUser.id, values.role)
    setRoleUser(null)
    roleForm.resetFields()
  }

  const targetByCode = useMemo(() => new Map(targets.map((target) => [target.code, target])), [targets])

  const columns = useMemo<ColumnsType<AdminUser>>(
    () => {
      const items: ColumnsType<AdminUser> = [
        { title: 'ID', dataIndex: 'id', width: 80, render: (value: number) => `#${value}` },
        {
          title: '账号',
          dataIndex: 'username',
          width: 180,
          render: (value: string) => <Text strong>{value}</Text>,
        },
        {
          title: '显示名称',
          dataIndex: 'displayName',
          width: 180,
          render: (value: string) => value || '-',
        },
        {
          title: '角色',
          dataIndex: 'role',
          width: 130,
          render: (value: string) => <Tag color={adminUserRoleColor(value)}>{adminUserRoleText(value)}</Tag>,
        },
        {
          title: '状态',
          dataIndex: 'disabled',
          width: 110,
          render: (value: boolean) => <Tag color={value ? 'red' : 'green'}>{value ? '停用' : '启用'}</Tag>,
        },
        {
          title: '消耗金额（$）',
          dataIndex: 'totalUsageUsd',
          width: 150,
          render: (value?: number) => `$${formatTotalUsageUSD(value ?? 0)}`,
        },
        {
          title: '上传秘钥总数',
          dataIndex: 'uploadedKeyCount',
          width: 150,
          render: (value?: number) => value ?? 0,
        },
      ]
      if (canConfigureUsers) {
        items.push({
          title: '接收平台',
          dataIndex: 'aggregationTargetCodes',
          width: 280,
          render: (codes: string[], user) => (
            <Space wrap size={[4, 4]}>
              {user.inheritsAggregationTargets ? (
                <Tag color="purple">继承自 {user.createdByUsername || '账号管理员'}</Tag>
              ) : null}
              {codes?.length ? (
                codes.map((code) => {
                  const target = targetByCode.get(code)
                  return (
                    <Tag key={code} color={target?.enabled ? 'blue' : 'default'}>
                      {target?.name || '未知接收平台'}
                      {target && !target.enabled ? '（已停用）' : ''}
                    </Tag>
                  )
                })
              ) : (
                <Tag color="red">无上传权限</Tag>
              )}
            </Space>
          ),
        })
      }
      items.push(
        {
          title: '最近登录',
          dataIndex: 'lastLoginAt',
          width: 180,
          render: (value?: string) => formatDateTime(value ?? ''),
        },
        {
          title: '创建时间',
          dataIndex: 'createdAt',
          width: 180,
          render: (value: string) => formatDateTime(value),
        },
      )
      if (canConfigureUsers) {
        items.push({
          title: '操作',
          width: 130,
          fixed: 'right',
          render: (_, row) => {
            const rowBusy = operationLoading?.endsWith(`:${row.id}`) ?? false
            const cannotDisableSelf = !row.disabled && row.id === currentUserId
            const protectedRole = row.role === 'root' || row.role === 'admin'
            return (
              <Dropdown
                trigger={['click']}
                menu={{
                  items: [
                    {
                      key: 'targets',
                      icon: <Pencil size={15} />,
                      label: '配置接收平台',
                      disabled: row.inheritsAggregationTargets,
                    },
                    {
                      key: 'role',
                      icon: <ShieldCheck size={15} />,
                      label: '权限配置',
                      disabled: protectedRole || row.inheritsAggregationTargets,
                    },
                    { type: 'divider' },
                    {
                      key: 'status',
                      icon: row.disabled ? <Power size={15} /> : <PowerOff size={15} />,
                      label: row.disabled ? '启用用户' : '禁用用户',
                      danger: !row.disabled,
                      disabled: cannotDisableSelf,
                    },
                    {
                      key: 'password',
                      icon: <KeyRound size={15} />,
                      label: '重置密码',
                    },
                  ],
                  onClick: ({ key }) => {
                    if (key === 'targets') {
                      openTargetAssociations(row)
                    } else if (key === 'role') {
                      openRoleConfiguration(row)
                    } else if (key === 'status') {
                      void handleUserStatusAction(row)
                    } else if (key === 'password') {
                      openPasswordReset(row)
                    }
                  },
                }}
              >
                <Button loading={rowBusy} disabled={rowBusy}>
                  <Pencil size={15} />
                  配置
                  <ChevronDown size={14} />
                </Button>
              </Dropdown>
            )
          },
        })
      } else if (canChangeUserStatus) {
        items.push({
          title: '操作',
          width: 130,
          fixed: 'right',
          render: (_, row) => {
            const rowBusy = operationLoading === `user-status:${row.id}`
            return (
              <Button
                danger={!row.disabled}
                icon={row.disabled ? <Power size={15} /> : <PowerOff size={15} />}
                loading={rowBusy}
                disabled={rowBusy}
                onClick={() => void handleUserStatusAction(row)}
              >
                {row.disabled ? '启用' : '禁用'}
              </Button>
            )
          },
        })
      }
      return items
    },
    [canChangeUserStatus, canConfigureUsers, currentUserId, operationLoading, targetByCode],
  )

  return (
    <div className="page-stack">
      <section className="page-heading">
        <div>
          <Title level={2}>用户管理</Title>
          <Text type="secondary">
            {canConfigureUsers
              ? '创建用户账号，并配置用户权限和可以入库的接收平台。'
              : '创建普通用户；新用户实时继承你的接收平台配置。'}
          </Text>
        </div>
        <Button type="primary" icon={<UserPlus size={16} />} onClick={openCreate}>
          创建用户
        </Button>
      </section>

      <Card>
        <div className="user-search-bar">
          <Input.Search
            allowClear
            aria-label="搜索用户"
            placeholder="搜索账号、显示名称或 ID"
            value={searchText}
            onChange={(event) => {
              setSearchText(event.target.value)
              setCurrentPage(1)
            }}
          />
          <Text type="secondary">显示 {visibleUsers.length} / {users.length}</Text>
        </div>
        <Table
          rowKey="id"
          dataSource={visibleUsers}
          columns={columns}
          loading={loading}
          pagination={{
            current: currentPage,
            pageSize: 12,
            size: 'small',
            onChange: setCurrentPage,
          }}
          locale={tableEmpty(normalizedSearchText ? '未找到匹配用户' : '暂无用户')}
          scroll={{ x: canConfigureUsers ? 1470 : canChangeUserStatus ? 1170 : 1050 }}
        />
      </Card>

      <Modal
        title="创建用户"
        open={modalOpen}
        okText="创建"
        cancelText="取消"
        confirmLoading={creating}
        onOk={submit}
        onCancel={() => setModalOpen(false)}
        destroyOnHidden
      >
        {!canConfigureUsers ? (
          <Alert
            type="info"
            showIcon
            message="你创建的用户将实时继承你的接收平台，无需也不能单独配置。"
            style={{ marginBottom: 20 }}
          />
        ) : null}
        <Form form={form} layout="vertical" autoComplete="off">
          <div className="browser-autofill-decoy" aria-hidden="true">
            <input type="text" name="username" autoComplete="username" tabIndex={-1} />
            <input type="password" name="password" autoComplete="new-password" tabIndex={-1} />
          </div>
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
            <Input
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
          <Form.Item label="显示名称" name="displayName" rules={[{ max: 128, message: '显示名称最多 128 个字符' }]}>
            <Input
              autoComplete="off"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
          <Form.Item
            label="密码"
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 8, message: '密码至少 8 个字符' },
            ]}
          >
            <Input.Password
              autoComplete="new-password"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
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
                  return Promise.reject(new Error('两次输入的密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password
              autoComplete="new-password"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={roleUser ? `配置 ${roleUser.username} 的权限` : '权限配置'}
        open={Boolean(roleUser)}
        okText="保存"
        cancelText="取消"
        confirmLoading={Boolean(roleUser && operationLoading === `user-role:${roleUser.id}`)}
        onOk={submitRoleConfiguration}
        onCancel={() => {
          setRoleUser(null)
          roleForm.resetFields()
        }}
        destroyOnHidden
      >
        <Alert
          type="info"
          showIcon
          message="账号管理员权限范围"
          description="可以进入用户管理并创建普通用户；创建的用户实时继承其接收平台；在“我的秘钥”中可查看本人和所创建用户的数据。除此之外不增加任何权限。"
          style={{ marginBottom: 20 }}
        />
        <Form form={roleForm} layout="vertical">
          <Form.Item label="账号角色" name="role" rules={[{ required: true, message: '请选择账号角色' }]}>
            <Segmented
              block
              options={[
                { label: '普通用户', value: 'user' },
                { label: '账号管理员', value: 'account_manager' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={passwordUser ? `重置 ${passwordUser.username} 的密码` : '重置密码'}
        open={Boolean(passwordUser)}
        okText="确认重置"
        cancelText="取消"
        confirmLoading={Boolean(passwordUser && operationLoading === `user-password:${passwordUser.id}`)}
        onOk={submitPasswordReset}
        onCancel={() => {
          setPasswordUser(null)
          passwordForm.resetFields()
        }}
        destroyOnHidden
      >
        <Alert
          type="warning"
          showIcon
          message="密码重置后，该用户现有登录会话会立即失效。"
          style={{ marginBottom: 20 }}
        />
        <Form form={passwordForm} layout="vertical" autoComplete="off">
          <div className="browser-autofill-decoy" aria-hidden="true">
            <input type="text" name="username" autoComplete="username" tabIndex={-1} />
            <input type="password" name="password" autoComplete="new-password" tabIndex={-1} />
          </div>
          <Form.Item
            label="新密码"
            name="password"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '密码至少 8 个字符' },
            ]}
          >
            <Input.Password
              autoComplete="new-password"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
          <Form.Item
            label="确认新密码"
            name="confirmPassword"
            dependencies={['password']}
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) {
                    return Promise.resolve()
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password
              autoComplete="new-password"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        className="user-target-association-modal"
        title={targetUser ? `配置 ${targetUser.username} 的接收平台` : '配置接收平台'}
        open={Boolean(targetUser)}
        okText="保存"
        cancelText="取消"
        confirmLoading={Boolean(targetUser && operationLoading === `user-targets:${targetUser.id}`)}
        onOk={submitTargetAssociations}
        onCancel={() => {
          setTargetUser(null)
          targetForm.resetFields()
        }}
        destroyOnHidden
        width={720}
      >
        <Alert
          type="info"
          showIcon
          message="用户只能把密钥入库到已关联且已启用的平台；不选择任何平台时，该用户不能上传密钥。"
          className="user-target-association-hint"
        />
        <Form form={targetForm} layout="vertical">
          <div className="user-target-config-head">
            <div>
              <Text strong>接收平台</Text>
              <Text type="secondary">勾选用户可以使用的接收平台。</Text>
            </div>
            <Tag color={selectedTargetCodes.length > 0 ? 'blue' : 'red'}>
              已关联 {selectedTargetCodes.length}/{configurableTargets.length}
            </Tag>
          </div>
          {configurableTargets.length > 0 ? (
            <Form.Item name="targetCodes" noStyle>
              <Checkbox.Group className="user-target-config-list">
                {configurableTargets.map((target) => {
                  const selected = selectedTargetCodeSet.has(target.code)
                  return (
                    <div
                      className={`user-target-config-row${selected ? ' is-selected' : ''}${target.enabled ? '' : ' is-disabled-target'}`}
                      key={target.code}
                    >
                      <Checkbox value={target.code} className="user-target-config-check">
                        <span className="user-target-config-summary">
                          <span className="user-target-config-title">
                            <Text strong>{target.name}</Text>
                            <Tag color="blue">{aggregationTargetChannelTypeText(target.channelType)}</Tag>
                            {!target.enabled ? <Tag>已停用</Tag> : null}
                          </span>
                          <Text type="secondary">{aggregationTargetModeLabels[target.connectionMode]}</Text>
                        </span>
                      </Checkbox>
                    </div>
                  )
                })}
              </Checkbox.Group>
            </Form.Item>
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可配置的接收平台" />
          )}
        </Form>
      </Modal>
    </div>
  )
}

interface ModelGapFormValues {
  providerType: string
  modelName: string
  rpmGap: number
  tpmGap: number
}

function ModelGapsPage({
  items,
  loading,
  saving,
  canManage,
  onSave,
}: {
  items: ModelGap[]
  loading: boolean
  saving: boolean
  canManage: boolean
  onSave: (values: ModelGapInput, id?: number) => Promise<void>
}) {
  const [form] = Form.useForm<ModelGapFormValues>()
  const [modalOpen, setModalOpen] = useState(false)
  const [editingItem, setEditingItem] = useState<ModelGap | null>(null)

  const openCreate = () => {
    setEditingItem(null)
    form.setFieldsValue({
      providerType: '',
      modelName: '',
      rpmGap: 0,
      tpmGap: 0,
    })
    setModalOpen(true)
  }

  const openEdit = (item: ModelGap) => {
    setEditingItem(item)
    form.setFieldsValue({
      providerType: item.providerType,
      modelName: item.modelName,
      rpmGap: item.rpmGap,
      tpmGap: item.tpmGap,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    await onSave(
      {
        providerType: values.providerType.trim(),
        modelName: values.modelName.trim(),
        rpmGap: values.rpmGap,
        tpmGap: values.tpmGap,
      },
      editingItem?.id,
    )
    setModalOpen(false)
    setEditingItem(null)
  }

  const copyNotification = async () => {
    if (items.length === 0) {
      message.warning('暂无模型缺口可复制')
      return
    }
    const lines = items.map(
      (item, index) =>
        `${index + 1}. [${item.providerType}] ${item.modelName}：缺口 RPM ${formatModelGapInteger(item.rpmGap)}，缺口 TPM ${formatModelGapAmount(item.tpmGap)}`,
    )
    const notification = ['【模型缺口通知】', '当前急需以下模型额度：', ...lines, '如有相关资源，请及时联系。'].join('\n')
    try {
      await navigator.clipboard.writeText(notification)
      message.success('模型缺口通知已复制')
    } catch {
      message.error('复制失败，请检查浏览器剪贴板权限')
    }
  }

  const columns = useMemo<ColumnsType<ModelGap>>(() => {
    const baseColumns: ColumnsType<ModelGap> = [
      {
        title: '类型',
        dataIndex: 'providerType',
        width: 180,
        render: (value: string) => <Tag color={modelGapProviderColor(value)}>{value}</Tag>,
      },
      {
        title: '模型名',
        dataIndex: 'modelName',
        width: 280,
        render: (value: string) => (
          <Tooltip title={value}>
            <Text code className="model-gap-model-name">{value}</Text>
          </Tooltip>
        ),
      },
      {
        title: '缺口 RPM',
        dataIndex: 'rpmGap',
        width: 300,
        align: 'right',
        defaultSortOrder: 'descend',
        sorter: (left, right) => left.rpmGap - right.rpmGap,
        render: (value: number) => (
          <span className="model-gap-number model-gap-rpm">
            <span className="model-gap-heat" aria-hidden="true">
              {'🔥'.repeat(modelGapHeat(value))}
            </span>
            <Text strong>{formatModelGapInteger(value)}</Text>
          </span>
        ),
      },
      {
        title: '缺口 TPM（估）',
        dataIndex: 'tpmGap',
        width: 220,
        align: 'right',
        sorter: (left, right) => left.tpmGap - right.tpmGap,
        render: (value: number) => <span className="model-gap-number">{formatModelGapAmount(value)}</span>,
      },
    ]
    if (canManage) {
      baseColumns.push({
        title: '操作',
        width: 100,
        align: 'center',
        fixed: 'right',
        render: (_, item) => (
          <Button type="link" icon={<Pencil size={15} />} onClick={() => openEdit(item)}>
            编辑
          </Button>
        ),
      })
    }
    return baseColumns
  }, [canManage])

  return (
    <div className="page-stack model-gap-page">
      <section className="page-heading">
        <div>
          <Title level={2}>
            <span className="model-gap-title-icon" aria-hidden="true">
              <AlertTriangle size={27} />
            </span>
            模型缺口
          </Title>
          <Text type="secondary">集中展示当前模型的 RPM 与 TPM 需求缺口。</Text>
        </div>
      </section>

      <Card
        className="model-gap-card"
        title={
          <Space size={8}>
            <Text strong>实时需求缺口</Text>
            <Text type="secondary" className="model-gap-card-subtitle">
              缺口越大越紧缺
            </Text>
          </Space>
        }
        extra={
          canManage ? (
            <Space>
              <Button icon={<Copy size={16} />} onClick={copyNotification}>
                一键复制通知
              </Button>
              <Button type="primary" icon={<Plus size={16} />} onClick={openCreate}>
                新增
              </Button>
            </Space>
          ) : null
        }
      >
        <Table
          className="model-gap-table"
          rowKey="id"
          dataSource={items}
          columns={columns}
          loading={loading}
          pagination={false}
          tableLayout="fixed"
          locale={tableEmpty('暂无模型缺口')}
          scroll={{ x: canManage ? 1080 : 980 }}
        />
      </Card>

      <Modal
        title={editingItem ? '编辑模型缺口' : '新增模型缺口'}
        open={modalOpen}
        okText="保存"
        cancelText="取消"
        confirmLoading={saving}
        onOk={submit}
        onCancel={() => {
          setModalOpen(false)
          setEditingItem(null)
        }}
        destroyOnHidden
        width={560}
      >
        <Form
          form={form}
          layout="vertical"
          initialValues={{ providerType: '', modelName: '', rpmGap: 0, tpmGap: 0 }}
        >
          <Form.Item
            label="类型"
            name="providerType"
            rules={[
              { required: true, whitespace: true, message: '请输入类型' },
              { max: 64, message: '类型最多 64 个字符' },
            ]}
          >
            <Input placeholder="例如 Anthropic、VertexAI" />
          </Form.Item>
          <Form.Item
            label="模型名"
            name="modelName"
            rules={[
              { required: true, whitespace: true, message: '请输入模型名' },
              { max: 128, message: '模型名最多 128 个字符' },
            ]}
          >
            <Input placeholder="例如 claude-sonnet-4-6" />
          </Form.Item>
          <div className="model-gap-form-grid">
            <Form.Item
              label="缺口 RPM"
              name="rpmGap"
              rules={[{ required: true, message: '请输入缺口 RPM' }]}
            >
              <InputNumber min={0} max={Number.MAX_SAFE_INTEGER} precision={0} controls className="model-gap-input" />
            </Form.Item>
            <Form.Item
              label="缺口 TPM（估）"
              name="tpmGap"
              rules={[{ required: true, message: '请输入缺口 TPM' }]}
            >
              <InputNumber min={0} max={Number.MAX_SAFE_INTEGER} precision={0} controls className="model-gap-input" />
            </Form.Item>
          </div>
        </Form>
      </Modal>
    </div>
  )
}

interface AggregationTargetFormValues {
  name: string
  connectionMode: AggregationTargetConnectionMode
  channelType: AggregationTargetChannelType
  models: string[]
  baseUrl: string
  token?: string
  reverseUsername?: string
  reverseGroupName?: string
  reversePassword?: string
  enabled: boolean
  isSystemDefault: boolean
}

const aggregationTargetModeLabels: Record<AggregationTargetConnectionMode, string> = {
  api: 'API方式',
  new_api_reverse: 'new-api逆向',
  token_supply: '采购供应商',
}

const aggregationTargetChannelTypeOptions: Array<{ value: AggregationTargetChannelType; label: string }> = [
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'aws_bedrock', label: '旧版 AWS Bedrock' },
  { value: 'aws_bedrock_new', label: '新版 AWS Bedrock' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'azure_openai', label: 'Azure OpenAI' },
  { value: 'google_ai_studio', label: 'Google AI Studio' },
]

function aggregationTargetChannelTypeText(channelType: AggregationTargetChannelType) {
  return aggregationTargetChannelTypeOptions.find((option) => option.value === channelType)?.label ?? channelType
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
  onDelete: (code: string, name: string) => Promise<void>
}) {
  const [form] = Form.useForm<AggregationTargetFormValues>()
  const [modalOpen, setModalOpen] = useState(false)
  const [editingTarget, setEditingTarget] = useState<AdminAggregationTarget | null>(null)
  const [contractChecking, setContractChecking] = useState(false)
  const [reverseAdminChecking, setReverseAdminChecking] = useState(false)
  const [tokenSupplyChecking, setTokenSupplyChecking] = useState(false)
  const connectionMode = Form.useWatch('connectionMode', form) ?? 'api'
  const canRetainToken = editingTarget?.connectionMode === 'api' && editingTarget.hasToken
  const canRetainReversePassword =
    editingTarget?.connectionMode === connectionMode &&
    (connectionMode === 'new_api_reverse' || connectionMode === 'token_supply') &&
    editingTarget.hasReversePassword

  const openCreate = () => {
    setEditingTarget(null)
    form.setFieldsValue({
      name: '',
      connectionMode: 'api',
      channelType: 'anthropic',
      models: [],
      baseUrl: '',
      token: '',
      reverseUsername: '',
      reverseGroupName: 'default',
      reversePassword: '',
      enabled: true,
      isSystemDefault: false,
    })
    setModalOpen(true)
  }

  const openEdit = (target: AdminAggregationTarget) => {
    if (target.source !== 'database') {
      return
    }
    setEditingTarget(target)
    form.setFieldsValue({
      name: target.name,
      connectionMode: target.connectionMode ?? 'api',
      channelType: target.channelType ?? 'anthropic',
      models: target.models ?? [],
      baseUrl: target.baseUrl,
      token: '',
      reverseUsername: target.reverseUsername ?? '',
      reverseGroupName: target.reverseGroupName ?? 'default',
      reversePassword: '',
      enabled: target.enabled,
      isSystemDefault: target.isSystemDefault,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    const mode = values.connectionMode ?? 'api'
    const payload: AggregationTargetInput = {
      name: values.name.trim(),
      baseUrl: values.baseUrl.trim(),
      connectionMode: mode,
      channelType: values.channelType,
      models: Array.from(new Set((values.models ?? []).map((model) => model.trim()).filter(Boolean))),
      enabled: values.enabled,
      isSystemDefault: values.isSystemDefault,
    }
    if (mode === 'api') {
      payload.token = values.token?.trim() || undefined
    } else {
      payload.reverseUsername = values.reverseUsername?.trim() || undefined
      if (mode === 'new_api_reverse') {
        payload.reverseGroupName = values.reverseGroupName?.trim() || 'default'
      }
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
    const values = await form.validateFields(['baseUrl'])
    const token = String(form.getFieldValue('token') ?? '').trim()
    if (!canRetainToken && !token) {
      message.warning('请填写 Token 后再检查')
      return
    }
    setContractChecking(true)
    try {
      const result = await checkAggregationTargetContract({
        code: editingTarget?.code,
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
        code: editingTarget?.code,
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

  const checkTokenSupply = async () => {
    if (connectionMode !== 'token_supply') {
      return
    }
    const values = await form.validateFields(['baseUrl', 'reverseUsername'])
    const password = String(form.getFieldValue('reversePassword') ?? '').trim()
    if (!canRetainReversePassword && !password) {
      message.warning('请填写密码后再校验')
      return
    }
    setTokenSupplyChecking(true)
    try {
      const result = await checkAggregationTargetTokenSupply({
        code: editingTarget?.code,
        baseUrl: String(values.baseUrl ?? '').trim(),
        reverseUsername: String(values.reverseUsername ?? '').trim(),
        reversePassword: password || undefined,
      })
      message.success(`平台校验通过，供应商 ${result.providerCount} 个，ApiKey ${result.apiKeyCount} 个`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '平台校验失败')
    } finally {
      setTokenSupplyChecking(false)
    }
  }

  const columns = useMemo<ColumnsType<AdminAggregationTarget>>(
    () => [
      {
        title: '名称',
        dataIndex: 'name',
        width: 200,
        render: (value: string) => <Text strong>{value}</Text>,
      },
      {
        title: '接入方式',
        dataIndex: 'connectionMode',
        width: 130,
        render: (value: AggregationTargetConnectionMode) => (
          <Tag color={aggregationTargetConnectionModeColor(value)}>{aggregationTargetConnectionModeText(value)}</Tag>
        ),
      },
      {
        title: '渠道类型',
        dataIndex: 'channelType',
        width: 150,
        render: (value: AggregationTargetChannelType) => <Tag color="blue">{aggregationTargetChannelTypeText(value)}</Tag>,
      },
      {
        title: '模型列表',
        dataIndex: 'models',
        width: 300,
        render: (models: string[]) =>
          models?.length ? (
            <Space size={[4, 4]} wrap>
              {models.slice(0, 3).map((model) => (
                <Tag key={model}>{model}</Tag>
              ))}
              {models.length > 3 ? <Tag>+{models.length - 3}</Tag> : null}
            </Space>
          ) : (
            <Text type="danger">未配置</Text>
          ),
      },
      {
        title: '分组',
        width: 110,
        render: (_, row) =>
          row.connectionMode === 'new_api_reverse' ? <Tag>{row.reverseGroupName || 'default'}</Tag> : <Text type="secondary">-</Text>,
      },
      {
        title: '地址',
        dataIndex: 'baseUrl',
        width: 260,
        render: (value: string) => <Text className="audit-detail">{value}</Text>,
      },
      {
        title: '状态',
        width: 120,
        render: (_, row) => <Tag color={row.enabled ? 'green' : 'default'}>{row.enabled ? '启用' : '停用'}</Tag>,
      },
      {
        title: '系统默认',
        dataIndex: 'isSystemDefault',
        width: 120,
        render: (value: boolean) => (value ? <Tag color="gold">系统默认</Tag> : <Text type="secondary">-</Text>),
      },
      {
        title: '凭据',
        width: 150,
        render: (_, row) =>
          row.connectionMode === 'new_api_reverse' || row.connectionMode === 'token_supply' ? (
            <Space size={4}>
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
                onClick={() => onDelete(row.code, row.name)}
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
          <Text type="secondary">启用可接入的平台；用户权限由“用户管理”中的平台关联控制。</Text>
        </div>
        <Button type="primary" icon={<Plus size={16} />} onClick={openCreate}>
          新增平台
        </Button>
      </section>

      <Card>
        <Table
          className="receiver-platform-table"
          rowKey={(row) => `${row.source}:${row.code}`}
          dataSource={targets}
          columns={columns}
          loading={loading}
          pagination={false}
          locale={tableEmpty('暂无接收平台')}
          scroll={{ x: 1800 }}
        />
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
        width={680}
      >
        <Form
          form={form}
          layout="vertical"
          autoComplete="off"
          initialValues={{
            connectionMode: 'api',
            channelType: 'anthropic',
            models: [],
            reverseGroupName: 'default',
            enabled: true,
            isSystemDefault: false,
          }}
        >
          <div className="browser-autofill-decoy" aria-hidden="true">
            <input type="text" name="username" autoComplete="username" tabIndex={-1} />
            <input type="password" name="password" autoComplete="current-password" tabIndex={-1} />
          </div>
          <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
            <Input
              placeholder="生产 new-api"
              autoComplete="off"
              data-1p-ignore="true"
              data-lpignore="true"
              data-form-type="other"
            />
          </Form.Item>
          <Form.Item label="渠道类型" name="channelType" rules={[{ required: true, message: '请选择渠道类型' }]}>
            <Select options={aggregationTargetChannelTypeOptions} placeholder="请选择接收平台渠道类型" />
          </Form.Item>
          <Form.Item
            label="模型列表"
            name="models"
            rules={[{ required: true, type: 'array', min: 1, message: '请至少配置一个模型' }]}
            extra="输入模型名称后按回车；可粘贴逗号或换行分隔的模型列表"
          >
            <Select
              mode="tags"
              tokenSeparators={[',', '\n']}
              placeholder="例如 claude-sonnet-4-6"
              options={[]}
            />
          </Form.Item>
          <Form.Item name="connectionMode" hidden>
            <Input />
          </Form.Item>
          <Tabs
            className="receiver-mode-tabs"
            activeKey={connectionMode}
            onChange={(key) => {
              form.setFieldValue('connectionMode', key as AggregationTargetConnectionMode)
              if (key !== 'new_api_reverse') {
                form.setFieldValue('isSystemDefault', false)
              }
            }}
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
                      <Form.Item
                        label="分组名称"
                        name="reverseGroupName"
                        rules={[{ max: 64, message: '分组名称最多 64 个字符' }]}
                      >
                        <Input
                          placeholder="default"
                          autoComplete="off"
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
              {
                key: 'token_supply',
                label: aggregationTargetModeLabels.token_supply,
                children:
                  connectionMode === 'token_supply' ? (
                    <>
                      <Form.Item
                        label="平台地址"
                        name="baseUrl"
                        rules={[{ required: true, message: '请输入平台地址' }]}
                      >
                        <Input
                          name="keyhub-token-supply-site"
                          placeholder="https://z.tokensupply.cc"
                          autoComplete="section-keyhub-token-supply off"
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
                          name="keyhub-token-supply-account"
                          placeholder="采购供应商账号"
                          autoComplete="section-keyhub-token-supply username"
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
                          name="keyhub-token-supply-password"
                          placeholder={canRetainReversePassword ? '留空则保持原密码' : '采购供应商密码'}
                          autoComplete="section-keyhub-token-supply new-password"
                          autoCapitalize="none"
                          spellCheck={false}
                          data-1p-ignore="true"
                          data-lpignore="true"
                          data-form-type="other"
                        />
                      </Form.Item>
                      <Form.Item>
                        <Tooltip title="登录平台并读取供应商与 ApiKey 列表">
                          <Button icon={<ShieldCheck size={15} />} loading={tokenSupplyChecking} onClick={checkTokenSupply}>
                            校验平台账号
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
            <Form.Item
              label="系统默认接收平台"
              name="isSystemDefault"
              valuePropName="checked"
              extra="同一时间只能设置一个，且必须使用 new-api逆向 接入方式；使用日志将实时读取该平台。"
            >
              <Switch disabled={connectionMode !== 'new_api_reverse'} />
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

function ComingSoon({ page }: { page: PageKey }) {
  const labels: Record<PageKey, string> = {
    apiAccess: 'API 访问',
    dashboard: '控制台',
    upload: '上传密钥',
    channels: '我的秘钥',
    usageLogs: '使用日志',
    modelGaps: '模型缺口',
    audit: '审计日志',
    ops: '运维状态',
    targets: '接收平台',
    users: '用户管理',
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
}: {
  value?: string
  onChange?: (value: string) => void
}) {
  const columns = useMemo(() => [{ title: 'Key', placeholder: '每行一个密钥' }], [])
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

function keyTableGridTemplate(columns: KeyTableColumn[]) {
  return 'minmax(320px, 1fr) 56px'
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
    .map((cells) => trimTrailingEmptyKeyCells(cells, columns.length))
    .filter((cells) => cells.some(Boolean))
    .map((cells) => (columns.length === 1 ? cells[0] : cells.join('|')))
    .join('\n')
}

function trimTrailingEmptyKeyCells(cells: string[], columnCount: number) {
  if (columnCount <= 1) {
    return cells
  }
  let lastIndex = cells.length - 1
  while (lastIndex > 0 && !cells[lastIndex]) {
    lastIndex -= 1
  }
  return cells.slice(0, lastIndex + 1)
}

type KeyRowsValidationResult = { ok: true } | { ok: false; message: string }

function validateKeyRows(rawText: string | undefined): KeyRowsValidationResult {
  const hasKey = String(rawText ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .some((line) => line.trim())
  return hasKey ? { ok: true } : { ok: false, message: '请至少填写一行密钥' }
}

function validateAWSCredentialRows(
  rawText: string | undefined,
  credentialType: AWSBedrockCredentialType,
): KeyRowsValidationResult {
  const expectedParts = credentialType === 'api_key' ? 2 : 3
  const expectedFormat = credentialType === 'api_key' ? 'APIKey|Region' : 'AccessKey|SecretAccessKey|Region'
  const rows = String(rawText ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)

  for (const [index, row] of rows.entries()) {
    const parts = row.split('|')
    if (parts.length !== expectedParts || parts.some((part) => !part.trim())) {
      return { ok: false, message: `第 ${index + 1} 行格式错误，应为 ${expectedFormat}` }
    }
  }
  return { ok: true }
}

function validateAWSBedrockNewRegions(rawText: string | undefined): KeyRowsValidationResult {
  const rows = normalizedMultilineRows(rawText)
  if (rows.length === 0) {
    return { ok: false, message: '请至少填写一行区域' }
  }
  for (const [index, row] of rows.entries()) {
    if (!row.startsWith(AWS_BEDROCK_NEW_REGION_PREFIX)) {
      return {
        ok: false,
        message: `区域第 ${index + 1} 行必须以 ${AWS_BEDROCK_NEW_REGION_PREFIX} 开头`,
      }
    }
  }
  return { ok: true }
}

function normalizedMultilineRows(rawText: string | undefined) {
  return String(rawText ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

function normalizeUploadModels(models: string[] | undefined) {
  return Array.from(new Set((models ?? []).map((model) => model.trim()).filter(Boolean)))
}

function normalizePastedKeys(rawText: string) {
  const rows = normalizedMultilineRows(rawText)
  if (rows.length === 0) {
    throw new Error('请粘贴至少一行密钥')
  }
  return { count: rows.length, rawText: rows.join('\n') }
}

function firstFormErrorMessage(error: unknown) {
  const errorFields = (error as { errorFields?: Array<{ errors?: string[] }> }).errorFields
  return errorFields?.find((field) => field.errors?.[0])?.errors?.[0]
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

const chinaTimeZone = 'Asia/Shanghai'
const chinaDateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: chinaTimeZone,
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
})

function formatDateTime(value: string) {
  if (!value) {
    return '-'
  }
  const displayParts = dateTimePartsForDisplay(value)
  if (!displayParts) {
    return value
  }
  const parts = displayParts
  const nowParts = chinaDateTimeParts(new Date())
  const diffDays =
    Date.UTC(parts.year, parts.month - 1, parts.day) - Date.UTC(nowParts.year, nowParts.month - 1, nowParts.day)
  const relativeDays = Math.round(diffDays / 86400000)
  const time = `${padTime(parts.hour)}:${padTime(parts.minute)}`
  let label: string
  if (relativeDays === 0) {
    label = `今天 ${time}`
  } else if (relativeDays === -1) {
    label = `昨天 ${time}`
  } else if (relativeDays === 1) {
    label = `明天 ${time}`
  } else if (parts.year === nowParts.year) {
    label = `${parts.month}月${parts.day}日 ${time}`
  } else {
    label = `${parts.year}年${parts.month}月${parts.day}日 ${time}`
  }
  const full = `${parts.year}年${parts.month}月${parts.day}日 ${padTime(parts.hour)}:${padTime(parts.minute)}:${padTime(
    parts.second,
  )} 北京时间`
  return (
    <Tooltip title={full}>
      <span className="time-display">{label}</span>
    </Tooltip>
  )
}

function dateTimePartsForDisplay(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return null
  }
  const parsedParts = chinaDateTimeParts(date)
  const literalParts = literalDateTimeParts(value)
  if (!literalParts) {
    return parsedParts
  }

  const literalDate = chinaPartsToDate(literalParts)
  const now = new Date()
  const futureToleranceMs = 60000
  if (date.getTime() - now.getTime() > futureToleranceMs && literalDate.getTime() - now.getTime() <= futureToleranceMs) {
    return literalParts
  }

  return parsedParts
}

function chinaDateTimeParts(date: Date) {
  const partMap = new Map(
    chinaDateTimeFormatter.formatToParts(date).map((part) => [part.type, part.value]),
  )
  return {
    year: Number(partMap.get('year') ?? 0),
    month: Number(partMap.get('month') ?? 1),
    day: Number(partMap.get('day') ?? 1),
    hour: Number(partMap.get('hour') ?? 0),
    minute: Number(partMap.get('minute') ?? 0),
    second: Number(partMap.get('second') ?? 0),
  }
}

function literalDateTimeParts(value: string) {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})[T\s](\d{2}):(\d{2})(?::(\d{2}))?/)
  if (!match) {
    return null
  }
  return {
    year: Number(match[1]),
    month: Number(match[2]),
    day: Number(match[3]),
    hour: Number(match[4]),
    minute: Number(match[5]),
    second: Number(match[6] ?? 0),
  }
}

function chinaPartsToDate(parts: ReturnType<typeof chinaDateTimeParts>) {
  return new Date(Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour - 8, parts.minute, parts.second))
}

function padTime(value: number) {
  return String(value).padStart(2, '0')
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

function formatModelGapInteger(value: number) {
  return value.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
}

function formatModelGapAmount(value: number) {
  if (Math.abs(value) >= 100000000) {
    return `${formatModelGapUnit(value / 100000000)} 亿`
  }
  if (Math.abs(value) >= 10000) {
    return `${formatModelGapUnit(value / 10000)} 万`
  }
  return formatModelGapInteger(value)
}

function formatModelGapUnit(value: number) {
  return value.toLocaleString('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 2 })
}

function modelGapHeat(rpmGap: number) {
  if (rpmGap >= 5000) {
    return 5
  }
  if (rpmGap >= 3000) {
    return 4
  }
  if (rpmGap >= 2000) {
    return 3
  }
  if (rpmGap >= 1000) {
    return 2
  }
  return rpmGap > 0 ? 1 : 0
}

function modelGapProviderColor(providerType: string) {
  const normalized = providerType.trim().toLowerCase()
  if (normalized.includes('anthropic')) {
    return 'purple'
  }
  if (normalized.includes('vertex') || normalized.includes('google')) {
    return 'blue'
  }
  if (normalized.includes('openai')) {
    return 'green'
  }
  if (normalized.includes('azure')) {
    return 'cyan'
  }
  if (normalized.includes('bedrock') || normalized.includes('aws')) {
    return 'orange'
  }
  return 'default'
}

function formatUsageUSD(value: number) {
  if (value === 0) {
    return '0'
  }
  return value.toFixed(Math.abs(value) < 0.01 ? 4 : 2)
}

function formatTotalUsageUSD(value: number) {
  return value.toLocaleString('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
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

function adminUserRoleColor(role: string) {
  if (role === 'root') {
    return 'gold'
  }
  if (role === 'admin') {
    return 'blue'
  }
  if (role === 'account_manager') {
    return 'purple'
  }
  return 'default'
}

function adminUserRoleText(role: string) {
  const map: Record<string, string> = {
    root: '超级管理员',
    admin: '管理员',
    account_manager: '账号管理员',
    user: '用户',
  }
  return map[role] ?? role
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
    token_supply: aggregationTargetModeLabels.token_supply,
  }
  return map[mode ?? 'api'] ?? mode ?? aggregationTargetModeLabels.api
}

function aggregationTargetConnectionModeColor(mode?: string) {
  const map: Record<string, string> = {
    api: 'blue',
    new_api_reverse: 'purple',
    token_supply: 'cyan',
  }
  return map[mode ?? 'api'] ?? 'default'
}

function workerStatusText(status: string) {
  const map: Record<string, string> = {
    success: '成功',
    failed: '失败',
  }
  return map[status] ?? status
}
