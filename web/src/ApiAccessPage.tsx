import { Alert, Button, Card, Form, Input, Popconfirm, Select, Space, Table, Tag, Typography, message } from 'antd'
import { useEffect, useState } from 'react'
import { createUserAPIToken, listUserAPITokens, revokeUserAPIToken, type UserAPIToken } from './api'

const formatTime = (value?: string) => value ? new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false }) : '尚未使用'

export default function ApiAccessPage({ authEnabled }: { authEnabled: boolean }) {
  const [form] = Form.useForm<{ name: string; expiresInDays: number }>()
  const [tokens, setTokens] = useState<UserAPIToken[]>([])
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [revoking, setRevoking] = useState<number | null>(null)
  const [newToken, setNewToken] = useState('')
  const [error, setError] = useState('')
  const base = import.meta.env.BASE_URL
  const apiURL = new URL(`${base}api/v1`, window.location.origin).href

  const refresh = async () => {
    setLoading(true)
    setError('')
    try { setTokens(await listUserAPITokens()) }
    catch (err) { setError(err instanceof Error ? err.message : '加载 Token 失败') }
    finally { setLoading(false) }
  }

  useEffect(() => { if (authEnabled) void refresh() }, [authEnabled])

  const create = async (values: { name: string; expiresInDays: number }) => {
    setCreating(true)
    try {
      const result = await createUserAPIToken(values)
      setNewToken(result.token)
      form.resetFields()
      await refresh()
    } catch (err) { message.error(err instanceof Error ? err.message : '创建失败') }
    finally { setCreating(false) }
  }

  const revoke = async (id: number) => {
    setRevoking(id)
    try {
      await revokeUserAPIToken(id)
      message.success('Token 已撤销')
      setNewToken('')
      await refresh()
    } catch (err) { message.error(err instanceof Error ? err.message : '撤销失败') }
    finally { setRevoking(null) }
  }

  return <Space orientation="vertical" size="large" style={{ width: '100%' }}>
    <div>
      <Typography.Title level={3}>API 访问</Typography.Title>
      <Typography.Paragraph type="secondary">通过脚本批量上传供应商密钥，并查询后台发布结果。上传的密钥归属于当前账号。</Typography.Paragraph>
      <Space wrap>
        <Button href={`${base}api-docs.html`} target="_blank" rel="noopener noreferrer">中文接入文档</Button>
        <Button href={`${base}openapi.json`} target="_blank" rel="noopener noreferrer">OpenAPI 定义</Button>
        <Typography.Text code copyable>{apiURL}</Typography.Text>
      </Space>
    </div>
    {!authEnabled ? <Alert type="warning" showIcon title="API 访问需要启用登录鉴权，并使用真实用户账号登录。" /> : <>
      {error && <Alert type="error" showIcon title={error} />}
      {newToken && <Alert type="success" showIcon title="Token 已创建，请现在复制保存" description={<>
        <Typography.Paragraph>完整 Token 只展示这一次，离开页面后无法再次查看。它用于访问 KeyHub，与待上传的供应商密钥不同。</Typography.Paragraph>
        <Typography.Paragraph code copyable style={{ overflowWrap: 'anywhere' }}>{newToken}</Typography.Paragraph>
        <Button size="small" onClick={() => setNewToken('')}>我已保存，隐藏 Token</Button>
      </>} />}
      <Card title="创建 API Token">
        <Typography.Paragraph type="secondary">权限：导入密钥、查询本人导入批次、重试失败项。最多保留 10 个有效 Token；按用户共享调用限额。</Typography.Paragraph>
        <Form form={form} layout="inline" initialValues={{ expiresInDays: 90 }} onFinish={create} style={{ gap: 12 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, whitespace: true, message: '请输入名称' }, { max: 64 }]}>
            <Input placeholder="例如：供应商同步脚本" maxLength={64} style={{ width: 240 }} />
          </Form.Item>
          <Form.Item name="expiresInDays" label="有效期" rules={[{ required: true }]}>
            <Select style={{ width: 130 }} options={[7, 30, 90, 180, 365].map(value => ({ value, label: `${value} 天` }))} />
          </Form.Item>
          <Form.Item><Button type="primary" htmlType="submit" loading={creating}>创建 Token</Button></Form.Item>
        </Form>
      </Card>
      <Card title="我的 API Token" extra={<Button onClick={() => void refresh()} loading={loading}>刷新</Button>}>
        <Table<UserAPIToken> rowKey="id" dataSource={tokens} loading={loading} scroll={{ x: 900 }} pagination={{ pageSize: 10 }} columns={[
          { title: '名称', dataIndex: 'name' },
          { title: 'Token 标识', dataIndex: 'prefix', render: value => <Typography.Text code>{value}…</Typography.Text> },
          { title: '状态', render: (_, row) => row.revokedAt ? <Tag>已撤销</Tag> : new Date(row.expiresAt).getTime() <= Date.now() ? <Tag color="orange">已过期</Tag> : <Tag color="green">有效</Tag> },
          { title: '到期时间', dataIndex: 'expiresAt', render: formatTime },
          { title: '最近使用', dataIndex: 'lastUsedAt', render: formatTime },
          { title: '操作', render: (_, row) => row.revokedAt ? '—' : <Popconfirm title="撤销此 Token？" description="使用它的脚本将无法继续调用 API，已受理的发布任务仍会继续。" onConfirm={() => revoke(row.id)} okText="撤销" cancelText="取消"><Button danger size="small" loading={revoking === row.id}>撤销</Button></Popconfirm> },
        ]} />
      </Card>
    </>}
  </Space>
}
