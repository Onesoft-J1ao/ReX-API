/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BookOpen,
  Braces,
  Check,
  ChevronRight,
  Copy,
  KeyRound,
  Menu,
  Server,
  ShieldCheck,
  X,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

/** Single source of truth for every address shown in these docs. */
const API_HOST = 'https://ai.j1ao.vip'
const API_BASE = `${API_HOST}/v1`

const sections = [
  { id: 'quick-start', title: '快速开始', icon: ArrowRight },
  { id: 'authentication', title: '身份认证', icon: KeyRound },
  { id: 'endpoints', title: '接口总览', icon: Server },
  { id: 'chat-completions', title: '对话补全', icon: Braces },
  { id: 'streaming', title: '流式输出', icon: Server },
  { id: 'responses', title: 'Responses 接口', icon: Braces },
  { id: 'anthropic', title: 'Claude 兼容', icon: Braces },
  { id: 'gemini', title: 'Gemini 兼容', icon: Braces },
  { id: 'embeddings-rerank', title: '向量与重排', icon: Server },
  { id: 'images', title: '图像生成', icon: Server },
  { id: 'audio', title: '语音接口', icon: Server },
  { id: 'video-tasks', title: '视频与异步任务', icon: Server },
  { id: 'realtime', title: '实时语音', icon: Server },
  { id: 'models', title: '查询模型列表', icon: BookOpen },
  { id: 'clients', title: '客户端配置', icon: KeyRound },
  { id: 'errors', title: '错误码', icon: ShieldCheck },
  { id: 'limits', title: '限额与配额', icon: ShieldCheck },
] as const

function CodeBlock(props: { children: string; language?: string }) {
  const [copied, setCopied] = useState(false)

  const copyCode = async () => {
    try {
      await navigator.clipboard.writeText(props.children)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className='border-border/60 bg-muted/30 relative overflow-hidden rounded-xl border'>
      <div className='border-border/50 text-muted-foreground flex items-center justify-between border-b px-4 py-2 font-mono text-xs tracking-wider uppercase'>
        <span>{props.language ?? 'bash'}</span>
        <button
          type='button'
          onClick={copyCode}
          className='hover:text-foreground inline-flex items-center gap-1.5 transition-colors'
          aria-label={copied ? '已复制' : '复制代码'}
        >
          {copied ? (
            <Check className='size-3.5' />
          ) : (
            <Copy className='size-3.5' />
          )}
          {copied ? '已复制' : '复制'}
        </button>
      </div>
      <pre className='overflow-x-auto p-4 text-sm leading-6'>
        <code>{props.children}</code>
      </pre>
    </div>
  )
}

/** Two-column parameter table used by every API section. */
function ParamTable(props: {
  rows: ReadonlyArray<readonly [string, string, string]>
}) {
  return (
    <div className='border-border/60 overflow-hidden rounded-xl border'>
      <table className='w-full border-collapse text-left text-sm'>
        <thead className='bg-transparent'>
          <tr className='text-muted-foreground text-xs uppercase'>
            <th className='px-4 py-2.5 font-medium'>参数</th>
            <th className='px-4 py-2.5 font-medium'>类型</th>
            <th className='px-4 py-2.5 font-medium'>说明</th>
          </tr>
        </thead>
        <tbody className='divide-border/50 divide-y'>
          {props.rows.map(([name, type, desc]) => (
            <tr key={name} className='align-top'>
              <td className='px-4 py-2.5 font-mono text-sm whitespace-nowrap'>
                {name}
              </td>
              <td className='text-muted-foreground px-4 py-2.5 font-mono text-xs whitespace-nowrap'>
                {type}
              </td>
              <td className='text-muted-foreground px-4 py-2.5 leading-6'>
                {desc}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function EndpointRow(props: {
  method: string
  path: string
  desc: string
}) {
  return (
    <tr className='align-top'>
      <td className='px-4 py-2.5 whitespace-nowrap'>
        <span className='bg-primary/10 text-primary rounded-md px-2 py-0.5 font-mono text-xs font-semibold'>
          {props.method}
        </span>
      </td>
      <td className='px-4 py-2.5 font-mono text-sm whitespace-nowrap'>
        {props.path}
      </td>
      <td className='text-muted-foreground px-4 py-2.5 leading-6'>
        {props.desc}
      </td>
    </tr>
  )
}

function SectionHeading(props: { eyebrow: string; title: string; id: string }) {
  return (
    <div id={props.id} className='scroll-mt-24'>
      <p className='text-primary mb-2 font-mono text-xs tracking-[0.18em] uppercase'>
        {props.eyebrow}
      </p>
      <h2 className='text-2xl font-semibold tracking-tight md:text-3xl'>
        {props.title}
      </h2>
    </div>
  )
}

function Note(props: { tone?: 'info' | 'warn'; children: ReactNode }) {
  const warn = props.tone === 'warn'
  return (
    <div
      className={
        warn
          ? 'rounded-xl border border-warning/20 bg-warning/5 p-4 text-sm leading-6'
          : 'border-border/60 bg-muted/20 rounded-xl border p-4 text-sm leading-6'
      }
    >
      {props.children}
    </div>
  )
}

export function DocsPage() {
  const [mobileOpen, setMobileOpen] = useState(false)

  const navigation = (
    <nav aria-label='文档导航' className='space-y-0.5'>
      {sections.map((section) => {
        const Icon = section.icon
        return (
          <a
            key={section.id}
            href={`#${section.id}`}
            onClick={() => setMobileOpen(false)}
            className='text-muted-foreground hover:text-foreground hover:bg-muted/60 flex items-center gap-2.5 rounded-md px-3 py-1.5 text-sm transition-colors'
          >
            <Icon className='size-3.5 shrink-0 opacity-70' />
            {section.title}
            <ChevronRight className='ml-auto size-3.5 opacity-30' />
          </a>
        )
      })}
    </nav>
  )

  return (
    <PublicLayout showMainContainer={false}>
      <div className='border-border/50 bg-background/95 min-h-svh border-b pt-16'>
        <div className='mx-auto flex max-w-7xl gap-8 px-4 py-8 md:px-6 lg:py-12'>
          <aside className='hidden w-56 shrink-0 lg:block'>
            <div className='sticky top-24 max-h-[calc(100vh-7rem)] overflow-y-auto'>
              <div className='mb-4 flex items-center gap-2 px-3 text-sm font-semibold'>
                <BookOpen className='text-primary size-4' />
                ReX API 文档
              </div>
              {navigation}
            </div>
          </aside>

          <div className='min-w-0 flex-1'>
            <div className='mb-8 flex items-start justify-between gap-4 lg:hidden'>
              <div>
                <p className='text-primary font-mono text-xs tracking-[0.18em] uppercase'>
                  ReX API
                </p>
                <h1 className='mt-2 text-3xl font-bold tracking-tight'>文档</h1>
              </div>
              <Button
                variant='outline'
                size='icon'
                aria-label='打开文档导航'
                onClick={() => setMobileOpen((open) => !open)}
              >
                {mobileOpen ? (
                  <X className='size-4' />
                ) : (
                  <Menu className='size-4' />
                )}
              </Button>
            </div>

            {mobileOpen && (
              <div className='border-border/60 bg-muted/20 mb-8 rounded-xl border p-3 lg:hidden'>
                {navigation}
              </div>
            )}

            <header className='border-border/60 mb-12 rounded-xl border p-6 md:p-10'>
              <p className='text-primary font-mono text-xs tracking-[0.2em] uppercase'>
                ReX API
              </p>
              <h1 className='mt-3 max-w-3xl text-3xl font-bold tracking-tight md:text-4xl'>
                统一 AI 网关，兼容 OpenAI 接口协议
              </h1>
              <p className='text-muted-foreground mt-4 max-w-2xl text-base leading-7'>
                语言模型、向量、图像、语音、视频与 3D 资产共用同一个密钥和同一个地址。已有
                OpenAI SDK 的代码只需替换 <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>base_url</code>。
              </p>
              <div className='mt-6 flex flex-wrap gap-3'>
                <Button render={<a href='#quick-start' />}>
                  开始使用
                  <ArrowRight className='ml-1.5 size-4' />
                </Button>
                <Button variant='outline' render={<Link to='/models' />}>
                  浏览模型
                </Button>
              </div>
            </header>

            <main className='space-y-16'>
              {/* ── 01 快速开始 ─────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading
                  id='quick-start'
                  eyebrow='01'
                  title='快速开始'
                />
                <p className='text-muted-foreground leading-7'>
                  所有接口共用同一个根地址，语言模型统一挂在{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    /v1
                  </code>{' '}
                  下。接入只需要三样东西：一个密钥、一个 Base URL、一个可用模型名。
                </p>

                <div className='border-border/60 bg-muted/20 grid gap-4 rounded-xl border p-5 sm:grid-cols-3'>
                  {[
                    ['根地址', API_HOST],
                    ['Base URL', API_BASE],
                    ['鉴权方式', 'Authorization: Bearer'],
                  ].map(([label, value]) => (
                    <div key={label}>
                      <div className='text-muted-foreground font-mono text-xs tracking-[0.18em] uppercase'>
                        {label}
                      </div>
                      <div className='mt-1 font-mono text-sm break-all'>
                        {value}
                      </div>
                    </div>
                  ))}
                </div>

                <p className='text-muted-foreground leading-7'>
                  接入步骤：在控制台创建密钥 → 用{' '}
                  <a href='#models' className='text-primary hover:underline'>
                    模型列表接口
                  </a>{' '}
                  查出该密钥可用的模型名 → 发起第一次请求 → 按需接流式、向量、图像或语音接口。
                </p>

                <CodeBlock language='bash'>{`export REX_API_KEY="sk-your-key"

curl ${API_BASE}/chat/completions \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "你好"}]
  }'`}</CodeBlock>

                <CodeBlock language='python'>{`from openai import OpenAI

client = OpenAI(
    api_key="sk-your-key",
    base_url="${API_BASE}",
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "你好"}],
)
print(response.choices[0].message.content)`}</CodeBlock>

                <Note tone='warn'>
                  <strong>密钥安全：</strong>
                  不要把密钥写进代码、文档或提交到 Git 仓库；不要放在浏览器端或移动端应用里。密钥泄露等同于额度泄露，请立即在控制台吊销并重建。
                </Note>
              </section>

              {/* ── 02 身份认证 ─────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading
                  id='authentication'
                  eyebrow='02'
                  title='身份认证'
                />
                <p className='text-muted-foreground leading-7'>
                  所有接口使用 HTTPS，通过请求头携带 Bearer Token 鉴权。前往{' '}
                  <Link to='/keys' className='text-primary hover:underline'>
                    API 密钥
                  </Link>{' '}
                  页面创建密钥，可以为每个应用单独签发一个密钥，便于分别限额和吊销。
                </p>

                <CodeBlock language='http'>{`Authorization: Bearer sk-your-key
Content-Type: application/json`}</CodeBlock>

                <p className='text-muted-foreground leading-7'>
                  Anthropic 兼容接口同时接受{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    x-api-key
                  </code>{' '}
                  头（Claude SDK 的默认方式），此时还需要带上{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    anthropic-version
                  </code>
                  。Gemini 兼容接口接受{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    x-goog-api-key
                  </code>{' '}
                  头或{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    ?key=
                  </code>{' '}
                  查询参数。
                </p>

                <div className='grid gap-3 sm:grid-cols-3'>
                  {[
                    ['Bearer', 'Authorization: Bearer sk-...', '所有 OpenAI 兼容接口'],
                    ['x-api-key', 'x-api-key: sk-...', 'Claude 兼容接口'],
                    ['x-goog-api-key', 'x-goog-api-key: sk-...', 'Gemini 兼容接口'],
                  ].map(([label, header, scope]) => (
                    <div
                      key={label}
                      className='border-border/60 bg-card rounded-xl border p-4'
                    >
                      <div className='font-mono text-sm font-semibold'>
                        {label}
                      </div>
                      <code className='text-muted-foreground mt-1 block font-mono text-xs break-all'>
                        {header}
                      </code>
                      <p className='text-muted-foreground mt-2 text-xs leading-5'>
                        {scope}
                      </p>
                    </div>
                  ))}
                </div>
              </section>

              {/* ── 03 接口总览 ─────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='endpoints' eyebrow='03' title='接口总览' />
                <p className='text-muted-foreground leading-7'>
                  下表是本网关实际开放的接口。所有路径都相对于根地址{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    {API_HOST}
                  </code>
                  ；请求体为 UTF-8 JSON（上传接口除外）。
                </p>

                <div className='border-border/60 overflow-hidden rounded-xl border'>
                  <table className='w-full border-collapse text-left text-sm'>
                    <thead className='bg-transparent'>
                      <tr className='text-muted-foreground text-xs uppercase'>
                        <th className='px-4 py-2.5 font-medium'>方法</th>
                        <th className='px-4 py-2.5 font-medium'>路径</th>
                        <th className='px-4 py-2.5 font-medium'>说明</th>
                      </tr>
                    </thead>
                    <tbody className='divide-border/50 divide-y'>
                      <EndpointRow
                        method='GET'
                        path='/v1/models'
                        desc='列出当前密钥可用的模型'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/chat/completions'
                        desc='对话补全，OpenAI Chat Completions 兼容'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/completions'
                        desc='文本补全（旧版 completions）'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/responses'
                        desc='OpenAI Responses 接口'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/responses/compact'
                        desc='Responses 上下文压缩'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/messages'
                        desc='Anthropic Messages 兼容'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1beta/models/*path'
                        desc='Gemini 原生协议兼容'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/embeddings'
                        desc='文本向量'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/rerank'
                        desc='重排序'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/images/generations'
                        desc='图像生成'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/images/edits'
                        desc='图像编辑'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/audio/speech'
                        desc='文本转语音'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/audio/transcriptions'
                        desc='语音转写'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/audio/translations'
                        desc='语音翻译'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/moderations'
                        desc='内容审核'
                      />
                      <EndpointRow
                        method='POST'
                        path='/v1/videos'
                        desc='提交视频生成任务'
                      />
                      <EndpointRow
                        method='GET'
                        path='/v1/videos/{task_id}'
                        desc='查询视频任务状态'
                      />
                      <EndpointRow
                        method='GET'
                        path='/v1/videos/{task_id}/content'
                        desc='下载视频内容'
                      />
                      <EndpointRow
                        method='GET'
                        path='/v1/realtime'
                        desc='实时语音 WebSocket'
                      />
                    </tbody>
                  </table>
                </div>

                <Note>
                  路径大小写敏感，且必须带{' '}
                  <code className='bg-muted rounded-md px-1 font-mono text-xs'>/v1</code>{' '}
                  前缀。漏掉前缀会命中前端路由而不是接口，表现为 404 或返回 HTML。
                </Note>
              </section>

              {/* ── 04 对话补全 ─────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading
                  id='chat-completions'
                  eyebrow='04'
                  title='对话补全'
                />
                <p className='text-muted-foreground leading-7'>
                  与 OpenAI Chat Completions 一致。把{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    model
                  </code>{' '}
                  设成模型列表里返回的名字，网关会按当前分组和负载把请求转发到对应上游渠道。
                </p>

                <CodeBlock language='json'>{`{
  "model": "gpt-4o",
  "messages": [
    {"role": "system", "content": "你是一个有帮助的助手。"},
    {"role": "user", "content": "解释一下量子计算"}
  ],
  "temperature": 0.7,
  "max_tokens": 1024,
  "stream": false
}`}</CodeBlock>

                <ParamTable
                  rows={[
                    ['model', 'string', '必填。模型名，取值见 GET /v1/models。'],
                    [
                      'messages',
                      'array',
                      '必填。消息数组，role 为 system / user / assistant / tool。content 可以是字符串，也可以是分块数组（文本 + 图片）。',
                    ],
                    [
                      'stream',
                      'boolean',
                      '为 true 时以 SSE 逐块返回，见「流式输出」。',
                    ],
                    [
                      'max_tokens',
                      'integer',
                      '单次回复的最大生成 token 数。实际上限受上游模型约束。',
                    ],
                    [
                      'temperature',
                      'number',
                      '采样温度，0 更确定，2 更随机。与 top_p 二选一调整。',
                    ],
                    [
                      'top_p',
                      'number',
                      '核采样阈值，默认 1。',
                    ],
                    [
                      'tools',
                      'array',
                      '函数调用定义。模型返回 tool_calls 后，由调用方执行并把结果作为 role=tool 的消息追加回来。',
                    ],
                    [
                      'tool_choice',
                      'string | object',
                      'auto / none / required，或指定某个函数。',
                    ],
                    [
                      'response_format',
                      'object',
                      '设为 {"type":"json_object"} 可要求返回 JSON。',
                    ],
                    [
                      'stop',
                      'string | array',
                      '遇到这些字符串停止生成。',
                    ],
                  ]}
                />

                <p className='text-muted-foreground leading-7'>
                  <strong className='text-foreground'>图像输入。</strong>
                  支持视觉的模型可以直接把图片放进 content 分块数组：
                </p>
                <CodeBlock language='json'>{`{
  "model": "gpt-4o",
  "messages": [{
    "role": "user",
    "content": [
      {"type": "text", "text": "这张图里是什么？"},
      {"type": "image_url",
       "image_url": {"url": "https://example.com/cat.png"}}
    ]
  }]
}`}</CodeBlock>

                <p className='text-muted-foreground leading-7'>
                  <strong className='text-foreground'>函数调用。</strong>
                  传入 tools 后模型可能返回{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    finish_reason: "tool_calls"
                  </code>
                  ，此时把每个调用结果作为一条 role=tool 的消息发回，再请求一次即可得到最终回答。
                </p>

                <p className='text-muted-foreground leading-7'>
                  <strong className='text-foreground'>响应格式。</strong>
                  与 OpenAI 一致：{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    id
                  </code>
                  、{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    choices[]
                  </code>
                  、{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    usage
                  </code>{' '}
                  都会返回。{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    usage
                  </code>{' '}
                  用于计费核对，流式模式下需要显式请求才会出现在最后一个分块里。
                </p>
              </section>

              {/* ── 05 流式输出 ─────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='streaming' eyebrow='05' title='流式输出' />
                <p className='text-muted-foreground leading-7'>
                  请求体加{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    "stream": true
                  </code>{' '}
                  即开启 SSE。每个分块是一行{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    data:
                  </code>{' '}
                  ，以{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    data: [DONE]
                  </code>{' '}
                  结束。想做用量统计时补上{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    stream_options: {'{"include_usage": true}'}
                  </code>
                  ，网关会在末尾追加一个只带 usage 的分块。
                </p>

                <CodeBlock language='python'>{`stream = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "写一首关于春天的诗"}],
    stream=True,
    stream_options={"include_usage": True},
)

for chunk in stream:
    if chunk.choices and chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
    if chunk.usage:
        print("\\nusage:", chunk.usage.total_tokens)`}</CodeBlock>

                <CodeBlock language='bash'>{`curl -N ${API_BASE}/chat/completions \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","stream":true,
       "messages":[{"role":"user","content":"你好"}]}'`}</CodeBlock>

                <Note>
                  流式连接受网关的空闲超时约束。长时间不产出 token 的上游会被断开，客户端应当对中断做重试或降级处理，不要把流式响应当成一定会完整送达的原子结果。
                </Note>
              </section>

              {/* ── 06 Responses ────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading
                  id='responses'
                  eyebrow='06'
                  title='Responses 接口'
                />
                <p className='text-muted-foreground leading-7'>
                  面向新一代 OpenAI 模型（含 Codex 系列）的接口。请求体与官方一致，使用{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    input
                  </code>{' '}
                  而不是{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    messages
                  </code>
                  。
                </p>
                <CodeBlock language='json'>{`{
  "model": "gpt-5-codex",
  "input": [
    {"role": "user", "content": "把这个函数改成尾递归"}
  ],
  "stream": true
}`}</CodeBlock>
                <p className='text-muted-foreground leading-7'>
                  部分模型被限制为只能走 Responses（不提供 chat/completions 别名）。如果某个模型在 chat
                  接口返回「不支持该接口」类错误，改用 Responses 再试。
                </p>
              </section>

              {/* ── 07 Anthropic ────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading
                  id='anthropic'
                  eyebrow='07'
                  title='Claude 兼容'
                />
                <p className='text-muted-foreground leading-7'>
                  网关同时兼容 Anthropic Messages 协议，Claude 官方 SDK 和 Claude Code 可以直接接入，只需把
                  Base URL 指过来。
                </p>
                <CodeBlock language='bash'>{`export ANTHROPIC_BASE_URL=${API_HOST}
export ANTHROPIC_AUTH_TOKEN=sk-your-key

curl ${API_BASE}/messages \\
  -H "x-api-key: $ANTHROPIC_AUTH_TOKEN" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "你好"}]
  }'`}</CodeBlock>
                <ParamTable
                  rows={[
                    [
                      'model',
                      'string',
                      '必填。Claude 模型名。',
                    ],
                    [
                      'max_tokens',
                      'integer',
                      '必填。Anthropic 协议要求显式给出。',
                    ],
                    [
                      'system',
                      'string | array',
                      '系统提示，注意它是顶层字段而不是 messages 里的一条。',
                    ],
                    [
                      'messages',
                      'array',
                      'user / assistant 交替。图片走 content 分块并带 type: image。',
                    ],
                    [
                      'tools',
                      'array',
                      '函数定义，格式为 name / description / input_schema。',
                    ],
                    [
                      'stream',
                      'boolean',
                      'SSE 流式，事件类型为 message_start、content_block_delta、message_stop 等。',
                    ],
                  ]}
                />
                <Note>
                  Claude Code 需要同时设置{' '}
                  <code className='bg-muted rounded-md px-1 font-mono text-xs'>
                    ANTHROPIC_BASE_URL
                  </code>{' '}
                  和{' '}
                  <code className='bg-muted rounded-md px-1 font-mono text-xs'>
                    ANTHROPIC_AUTH_TOKEN
                  </code>
                  ，不要用默认的{' '}
                  <code className='bg-muted rounded-md px-1 font-mono text-xs'>
                    ANTHROPIC_API_KEY
                  </code>
                  。
                </Note>
              </section>

              {/* ── 08 Gemini ───────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='gemini' eyebrow='08' title='Gemini 兼容' />
                <p className='text-muted-foreground leading-7'>
                  Gemini 原生协议挂在{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    /v1beta
                  </code>{' '}
                  下。路径中的方法与模型名直接对应官方格式，鉴权用{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    x-goog-api-key
                  </code>
                  。
                </p>
                <CodeBlock language='bash'>{`curl "${API_HOST}/v1beta/models/gemini-2.5-pro:generateContent" \\
  -H "x-goog-api-key: $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"contents":[{"parts":[{"text":"你好"}]}]}'`}</CodeBlock>
                <p className='text-muted-foreground leading-7'>
                  同一批 Gemini 模型也可以通过 OpenAI 兼容的{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    /v1/chat/completions
                  </code>{' '}
                  调用，两种方式任选。
                </p>
              </section>

              {/* ── 09 向量与重排 ───────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading
                  id='embeddings-rerank'
                  eyebrow='09'
                  title='向量与重排'
                />
                <p className='text-muted-foreground leading-7'>
                  向量接口返回与 OpenAI 一致的{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    data[].embedding
                  </code>
                  。重排序接口把候选文档按与查询的相关性排序，适合做检索后的精排。
                </p>
                <CodeBlock language='json'>{`{
  "model": "bge-m3",
  "input": ["第一段文本", "第二段文本"]
}`}</CodeBlock>
                <CodeBlock language='json'>{`{
  "model": "bge-reranker",
  "query": "什么是向量检索",
  "documents": ["向量检索是一种……", "今天天气不错"],
  "top_n": 2
}`}</CodeBlock>
                <p className='text-muted-foreground leading-7'>
                  重排返回{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    results[].relevance_score
                  </code>
                  ，按分数降序即为排序结果。注意重排是「输入文档 + 查询」的成对打分，不要和向量相似度混用同一套阈值。
                </p>
              </section>

              {/* ── 10 图像 ────────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='images' eyebrow='10' title='图像生成' />
                <p className='text-muted-foreground leading-7'>
                  文生图走{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    /v1/images/generations
                  </code>
                  ，图生图 / 局部重绘走{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    /v1/images/edits
                  </code>
                  （multipart 上传）。不同上游支持的尺寸和参数不同，以模型说明为准。
                </p>
                <CodeBlock language='bash'>{`curl ${API_BASE}/images/generations \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-image-1",
    "prompt": "一只在服务器机架旁的红熊猫",
    "size": "1024x1024",
    "n": 1
  }'`}</CodeBlock>
                <ParamTable
                  rows={[
                    ['model', 'string', '必填。图像模型名。'],
                    ['prompt', 'string', '必填。画面描述。'],
                    [
                      'size',
                      'string',
                      '输出尺寸，如 1024x1024、1024x1536。并非所有模型都支持任意尺寸。',
                    ],
                    ['n', 'integer', '生成张数。张数越多计费越高。'],
                    [
                      'quality',
                      'string',
                      '质量档位（low / medium / high 等），影响耗时与计费。',
                    ],
                    [
                      'response_format',
                      'string',
                      'url 或 b64_json。url 形式有有效期，需要长期保存请自行转存。',
                    ],
                  ]}
                />
                <Note tone='warn'>
                  图像按张数和分辨率计费，<code className='bg-muted rounded-md px-1 font-mono text-xs'>n</code>{' '}
                  和尺寸都会成倍放大消耗。批量调用前先用小参数验证提示词和尺寸是否正确。
                </Note>
              </section>

              {/* ── 11 语音 ────────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='audio' eyebrow='11' title='语音接口' />
                <p className='text-muted-foreground leading-7'>
                  三个接口分别对应合成、转写和翻译。合成返回的是二进制音频流，不是 JSON，客户端必须把响应体写入文件或音频缓冲。
                </p>
                <CodeBlock language='bash'>{`# 文本转语音：响应体是二进制音频
curl ${API_BASE}/audio/speech \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"tts-1","input":"你好，世界","voice":"alloy"}' \\
  --output speech.mp3

# 语音转写：multipart 上传音频文件
curl ${API_BASE}/audio/transcriptions \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -F "model=whisper-1" \\
  -F "file=@speech.mp3"`}</CodeBlock>
                <ParamTable
                  rows={[
                    [
                      '/audio/speech',
                      'POST',
                      '参数为 model、input、voice、speed、response_format。返回音频二进制。',
                    ],
                    [
                      '/audio/transcriptions',
                      'POST',
                      'multipart：file 加 model，可选 language、prompt、response_format。返回转录文本。',
                    ],
                    [
                      '/audio/translations',
                      'POST',
                      '同上，但把非英语语音转成英语文本。',
                    ],
                  ]}
                />
              </section>

              {/* ── 12 视频与任务 ───────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading
                  id='video-tasks'
                  eyebrow='12'
                  title='视频与异步任务'
                />
                <p className='text-muted-foreground leading-7'>
                  视频生成是异步的：先提交拿到任务 ID，再轮询状态，完成后下载内容。轮询不要打得太密，几秒一次足够。
                </p>
                <CodeBlock language='bash'>{`# 1) 提交任务
curl ${API_BASE}/videos \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"veo-3","prompt":"海边日落，缓慢推镜"}'

# 2) 轮询状态（把 {task_id} 换成上一步返回的 id）
curl ${API_BASE}/videos/{task_id} \\
  -H "Authorization: Bearer $REX_API_KEY"

# 3) 完成后下载内容
curl ${API_BASE}/videos/{task_id}/content \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  --output video.mp4`}</CodeBlock>
                <ParamTable
                  rows={[
                    [
                      'status',
                      'string',
                      '任务状态，常见取值 queued / processing / succeeded / failed。',
                    ],
                    [
                      'progress',
                      'number',
                      '进度百分比（部分上游返回）。',
                    ],
                    [
                      'error / fail_reason',
                      'string',
                      '失败原因。上游风控拦截和参数错误都会出现在这里。',
                    ],
                  ]}
                />
                <Note>
                  任务失败时预扣的额度会退回账户，但部分上游会收取手续费。长时间排队属于上游状态，与本地配置无关。
                </Note>
              </section>

              {/* ── 13 实时语音 ─────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='realtime' eyebrow='13' title='实时语音' />
                <p className='text-muted-foreground leading-7'>
                  实时对话走 WebSocket，路径为{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    /v1/realtime
                  </code>
                  ，协议与 OpenAI Realtime 一致。连接需要鉴权，并在查询参数里指定模型。
                </p>
                <CodeBlock language='javascript'>{`const ws = new WebSocket(
  "wss://ai.j1ao.vip/v1/realtime?model=gpt-4o-realtime-preview",
  []
);

ws.onopen = () => {
  ws.send(JSON.stringify({
    type: "session.update",
    session: { input_audio_format: "pcm16" },
  }));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === "response.done") {
    console.log("usage:", msg.response?.usage);
  }
};`}</CodeBlock>
                <Note tone='warn'>
                  空闲连接会被网关主动关闭。客户端需要按协议周期性发送 ping
                  或保持有音频帧流动；长时间静默的连接会被回收，重连即可。
                </Note>
              </section>

              {/* ── 14 模型列表 ─────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='models' eyebrow='14' title='查询模型列表' />
                <p className='text-muted-foreground leading-7'>
                  在写死任何模型名之前，先查一次这个接口。它返回的是
                  <strong className='text-foreground'>当前密钥实际可用</strong>
                  的模型，随分组、渠道状态和管理员配置变化。
                </p>
                <CodeBlock language='bash'>{`curl ${API_BASE}/models \\
  -H "Authorization: Bearer $REX_API_KEY"`}</CodeBlock>
                <CodeBlock language='json'>{`{
  "object": "list",
  "data": [
    { "id": "gpt-4o", "object": "model", "owned_by": "openai" },
    { "id": "claude-sonnet-4-5", "object": "model", "owned_by": "anthropic" }
  ]
}`}</CodeBlock>
                <Note>
                  模型广场展示的是全站目录，而本接口返回的是你的密钥范围内的子集。以本接口为准。
                </Note>
              </section>

              {/* ── 15 客户端配置 ───────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='clients' eyebrow='15' title='客户端配置' />

                <h3 className='text-lg font-semibold'>OpenAI SDK（Python / Node）</h3>
                <CodeBlock language='python'>{`from openai import OpenAI

client = OpenAI(api_key="sk-your-key", base_url="${API_BASE}")`}</CodeBlock>
                <CodeBlock language='javascript'>{`import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk-your-key",
  baseURL: "${API_BASE}",
});`}</CodeBlock>

                <h3 className='text-lg font-semibold'>Claude Code</h3>
                <CodeBlock language='bash'>{`export ANTHROPIC_BASE_URL=${API_HOST}
export ANTHROPIC_AUTH_TOKEN=sk-your-key
claude`}</CodeBlock>

                <h3 className='text-lg font-semibold'>Codex CLI</h3>
                <CodeBlock language='toml'>{`# ~/.codex/config.toml
model_provider = "rex"

[model_providers.rex]
name = "ReX API"
base_url = "${API_BASE}"
env_key = "REX_API_KEY"`}</CodeBlock>
                <CodeBlock language='bash'>{`export REX_API_KEY="sk-your-key"
codex`}</CodeBlock>

                <h3 className='text-lg font-semibold'>OpenCode</h3>
                <CodeBlock language='json'>{`{
  "provider": {
    "rex": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "${API_BASE}",
        "apiKey": "sk-your-key"
      }
    }
  }
}`}</CodeBlock>
              </section>

              {/* ── 16 错误码 ──────────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='errors' eyebrow='16' title='错误码' />
                <p className='text-muted-foreground leading-7'>
                  失败响应统一为 JSON 错误体，先看 HTTP 状态码判断类别，再看{' '}
                  <code className='bg-muted rounded-md px-1.5 py-0.5 font-mono text-sm'>
                    error.message
                  </code>{' '}
                  定位具体原因。
                </p>
                <CodeBlock language='json'>{`{
  "error": {
    "message": "可读的错误描述",
    "type": "错误类型",
    "code": "错误代码"
  }
}`}</CodeBlock>
                <div className='grid gap-3 sm:grid-cols-2'>
                  {[
                    ['400', '请求参数错误', '必填字段缺失、类型不对或取值越界。'],
                    ['401', '认证失败', '密钥无效、已吊销或请求头格式不对。'],
                    ['403', '无权限', '密钥无权访问该模型，或账户被禁用。'],
                    [
                      '404',
                      '模型或路径不存在',
                      '模型名不在密钥范围内；或路径漏了 /v1 前缀。',
                    ],
                    [
                      '413',
                      '请求体过大',
                      '超出请求体上限，检查是否夹带了超大 base64 媒体。',
                    ],
                    ['429', '触发限流', '请求过频或配额耗尽，稍后重试。'],
                    [
                      '500 / 502',
                      '上游异常',
                      '上游返回异常；网关通常会自动重试其它渠道。',
                    ],
                    [
                      '504',
                      '上游超时',
                      '上游长时间无响应，多见于长上下文或高峰排队。',
                    ],
                  ].map(([code, title, desc]) => (
                    <div
                      key={code}
                      className='border-border/60 bg-card rounded-xl border p-4'
                    >
                      <code className='text-primary font-mono text-sm font-bold'>
                        {code}
                      </code>
                      <p className='mt-1 text-sm font-medium'>{title}</p>
                      <p className='text-muted-foreground mt-0.5 text-xs leading-5'>
                        {desc}
                      </p>
                    </div>
                  ))}
                </div>
              </section>

              {/* ── 17 限额与配额 ──────────────────────────────────── */}
              <section className='space-y-5'>
                <SectionHeading id='limits' eyebrow='17' title='限额与配额' />
                <div className='grid gap-4 sm:grid-cols-2'>
                  {[
                    [
                      '并发与速率',
                      '同一密钥的并发请求过高会返回 429。需要更高并发请拆分多个密钥，并让客户端对 429 做指数退避。',
                    ],
                    [
                      '额度按 token 计费',
                      '按输入与输出 token 分别计价，缓存命中的输入 token 通常有折扣。以账单明细为准。',
                    ],
                    [
                      '上下文长度',
                      '每个模型有独立的上下文上限。超限一般返回 400，而不是静默截断，遇到就缩短历史消息。',
                    ],
                    [
                      '渠道切换',
                      '同一模型可能有多个上游渠道。单个渠道故障时网关会自动重试其它渠道，但重试也计入等待时间。',
                    ],
                  ].map(([title, desc]) => (
                    <div
                      key={title}
                      className='border-border/60 bg-card rounded-xl border p-5'
                    >
                      <h3 className='font-medium'>{title}</h3>
                      <p className='text-muted-foreground mt-2 text-sm leading-6'>
                        {desc}
                      </p>
                    </div>
                  ))}
                </div>

                <Note>
                  遇到问题反馈时请附上：分组、模型名、出错时间、完整报错文本。带上这几项才能对上服务端日志。
                </Note>
              </section>
            </main>
          </div>
        </div>
      </div>
    </PublicLayout>
  )
}
