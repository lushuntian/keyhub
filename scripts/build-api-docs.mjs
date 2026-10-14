import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = fileURLToPath(new URL('../', import.meta.url))
const escape = value => value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')
const inline = value => escape(value).replace(/`([^`]+)`/g, '<code>$1</code>').replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>')
const lines = readFileSync(path.join(root, 'docs/public-api.md'), 'utf8').split(/\r?\n/)
let code = false, table = false, list = false
const html = []
const closeBlocks = () => {
  if (table) { html.push('</tbody></table></div>'); table = false }
  if (list) { html.push('</ul>'); list = false }
}
for (const line of lines) {
  if (line.startsWith('```')) {
    if (code) html.push('</code></pre>')
    else { closeBlocks(); html.push('<pre><code>') }
    code = !code
    continue
  }
  if (code) { html.push(escape(line) + '\n'); continue }
  if (!line.trim()) { closeBlocks(); continue }
  if (/^\|/.test(line)) {
    if (/^\|[\s|:-]+\|$/.test(line)) continue
    const cells = line.slice(1, -1).split('|').map(value => inline(value.trim()))
    if (!table) { closeBlocks(); html.push('<div class="table"><table><thead><tr>' + cells.map(cell => `<th>${cell}</th>`).join('') + '</tr></thead><tbody>'); table = true }
    else html.push('<tr>' + cells.map(cell => `<td>${cell}</td>`).join('') + '</tr>')
    continue
  }
  if (line.startsWith('- ')) {
    if (!list) { closeBlocks(); html.push('<ul>'); list = true }
    html.push(`<li>${inline(line.slice(2))}</li>`); continue
  }
  closeBlocks()
  const heading = line.match(/^(#{1,3}) (.+)$/)
  html.push(heading ? `<h${heading[1].length}>${inline(heading[2])}</h${heading[1].length}>` : `<p>${inline(line)}</p>`)
}
closeBlocks()
const page = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>KeyHub 用户 API 接入指南</title><style>
*{box-sizing:border-box}body{margin:0;background:#f5f7fa;color:#1f2937;font:16px/1.8 system-ui,"Microsoft YaHei",sans-serif}main{max-width:1060px;margin:32px auto;padding:32px 48px;background:white;border:1px solid #e5e7eb;border-radius:12px}h1{font-size:30px}h2{margin-top:40px;border-top:1px solid #e5e7eb;padding-top:24px;font-size:22px}a{color:#1677ff}pre{background:#111827;color:#e5e7eb;padding:20px;border-radius:8px;overflow:auto;line-height:1.6}code{font:13px/1.6 ui-monospace,Consolas,monospace;overflow-wrap:anywhere}p code,li code,td code{background:#f3f4f6;padding:2px 5px;border-radius:4px}.table{overflow:auto}table{border-collapse:collapse;width:100%;font-size:14px}th,td{text-align:left;padding:10px 12px;border:1px solid #e5e7eb;min-width:100px}th{background:#f0f5ff}li{margin:8px 0}@media(max-width:700px){main{margin:0;padding:20px;border:0;border-radius:0}h1{font-size:25px}}
</style><main><nav><a href="./#/apiAccess">返回 API 访问</a> · <a href="openapi.json">下载 OpenAPI</a></nav>${html.join('')}</main></html>`
const out = path.join(root, 'web/public')
mkdirSync(out, { recursive: true })
writeFileSync(path.join(out, 'api-docs.html'), page)
const spec = JSON.parse(readFileSync(path.join(root, 'docs/openapi.json'), 'utf8'))
writeFileSync(path.join(out, 'openapi.json'), JSON.stringify(spec, null, 2) + '\n')
