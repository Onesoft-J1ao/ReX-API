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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy } from 'lucide-react'

import { AnimateInView } from '@/components/animate-in-view'
import { Button } from '@/components/ui/button'

const CHAT_SNIPPET = `curl https://your-host/v1/chat/completions \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`

const IMAGE_SNIPPET = `curl https://your-host/v1/images/generations \\
  -H "Authorization: Bearer $REX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-image-1",
    "prompt": "A red panda fixing a server rack"
  }'`

function SnippetBlock(props: { title: string; code: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(props.code)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      // Clipboard unavailable; leave the code selectable.
    }
  }

  return (
    <div className='border-border/60 bg-card/40 overflow-hidden rounded-2xl border'>
      <div className='border-border/60 flex items-center justify-between gap-3 border-b px-5 py-3'>
        <span className='font-mono text-[11px] tracking-[0.18em] text-muted-foreground uppercase'>
          {props.title}
        </span>
        <Button
          variant='ghost'
          size='sm'
          onClick={copy}
          className='h-8 gap-1.5 px-2.5 text-xs'
        >
          {copied ? (
            <Check className='size-3.5' />
          ) : (
            <Copy className='size-3.5' />
          )}
          {copied ? t('Copied') : t('Copy')}
        </Button>
      </div>
      <pre className='overflow-x-auto p-5 font-mono text-xs leading-6 text-foreground/90'>
        <code>{props.code}</code>
      </pre>
    </div>
  )
}

export function Quickstart() {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 px-6 py-14 sm:px-10 md:py-20 lg:px-14'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView className='mb-10 max-w-2xl md:mb-12'>
          <p className='text-primary mb-5 font-mono text-[11px] tracking-[0.28em] uppercase'>
            {t('Quickstart')}
          </p>
          <h2 className='text-foreground font-serif text-4xl leading-[1.05] font-medium tracking-[-0.02em] md:text-5xl'>
            {t('One key,')}
            <br />
            {t('every model.')}
          </h2>
          <p className='text-muted-foreground mt-6 max-w-xl text-base leading-7'>
            {t(
              'Point any OpenAI-compatible client at this gateway and swap the model name. No SDK changes.'
            )}
          </p>
        </AnimateInView>

        <div className='grid gap-4 lg:grid-cols-2'>
          <AnimateInView delay={0} animation='fade-up'>
            <SnippetBlock title='Chat' code={CHAT_SNIPPET} />
          </AnimateInView>
          <AnimateInView delay={80} animation='fade-up'>
            <SnippetBlock title='Image' code={IMAGE_SNIPPET} />
          </AnimateInView>
        </div>
      </div>
    </section>
  )
}
