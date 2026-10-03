// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://ai.j1ao.vip',
  integrations: [
    starlight({
      title: 'ReX API',
      description: 'ReX API 用户使用文档：语言模型、语音合成与 3D 生成能力接入指南。',
      // The logo is the REX wordmark, so it already carries the site name and
      // the text title next to it would only repeat it. The brand blue
      // (#0030b0) only reaches 1.47:1 against the dark header, so dark mode
      // gets the same wordmark in a lighter tint of the same hue (5.06:1).
      logo: {
        light: './src/assets/logo.png',
        dark: './src/assets/logo-dark.png',
        alt: 'ReX API',
        replacesTitle: true,
      },
      defaultLocale: 'root',
      locales: {
        root: { label: '简体中文', lang: 'zh-CN' },
      },
      sidebar: [
        {
          label: '开始使用',
          items: [{ label: '快速开始', link: '/' }],
        },
        {
          label: '客户端配置',
          items: [
            { label: 'Codex', slug: 'clients/codex' },
            { label: 'OpenCode', slug: 'clients/opencode' },
            { label: 'Claude Code', slug: 'clients/claude-code' },
          ],
        },
        {
          label: 'API 调用',
          items: [
            { label: 'API 概览', slug: 'api/overview' },
            { label: '语言模型 API', slug: 'api/chat-completions' },
            { label: 'Jev / System One API', slug: 'api/system-one' },
            { label: 'Speech API', slug: 'api/speech' },
            { label: 'IndexTTS2 API', slug: 'api/indextts2' },
            { label: 'MiniMax H3 视频生成', slug: 'api/video-generation' },
            { label: '3D 生成 API', slug: 'api/text-to-3d' },
          ],
        },
        {
          label: '参考',
          items: [
            { label: '模型与能力', slug: 'reference/models' },
            { label: '查询模型列表', slug: 'reference/model-list' },
            { label: '环境变量', slug: 'reference/env' },
            { label: '错误码', slug: 'reference/errors' },
          ],
        },
      ],
    }),
  ],
});
