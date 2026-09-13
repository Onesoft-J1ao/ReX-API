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

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { synthesizeSpeech, transcribeAudio } from '../api'
import { DEFAULT_SPEECH_CONFIG } from '../constants'
import type { SpeechWorkbenchConfig } from '../types'

export type SpeechWorkbenchMode = 'synthesize' | 'transcribe'

export function useSpeechGeneration() {
  const { t } = useTranslation()
  const [config, setConfig] = useState<SpeechWorkbenchConfig>({
    ...DEFAULT_SPEECH_CONFIG,
  })
  const [mode, setMode] = useState<SpeechWorkbenchMode>('synthesize')
  const [text, setText] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [audioUrl, setAudioUrl] = useState<string | null>(null)
  const [transcript, setTranscript] = useState('')
  const [isWorking, setIsWorking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const audioUrlRef = useRef<string | null>(null)

  // Object URLs are held by the browser until they are revoked, so the previous
  // one is released as soon as it is replaced and when the panel goes away.
  useEffect(
    () => () => {
      if (audioUrlRef.current) {
        URL.revokeObjectURL(audioUrlRef.current)
      }
    },
    []
  )

  const updateConfig = useCallback(
    <K extends keyof SpeechWorkbenchConfig>(
      key: K,
      value: SpeechWorkbenchConfig[K]
    ) => {
      setConfig((previous) => ({ ...previous, [key]: value }))
    },
    []
  )

  const canRun =
    !isWorking &&
    config.model.length > 0 &&
    (mode === 'synthesize' ? text.trim().length > 0 : Boolean(file))

  const run = useCallback(async () => {
    if (!canRun) {
      return
    }
    setIsWorking(true)
    setError(null)
    try {
      if (mode === 'synthesize') {
        const blob = await synthesizeSpeech(config, text)
        if (audioUrlRef.current) {
          URL.revokeObjectURL(audioUrlRef.current)
        }
        const nextUrl = URL.createObjectURL(blob)
        audioUrlRef.current = nextUrl
        setAudioUrl(nextUrl)
        setTranscript('')
      } else if (file) {
        const result = await transcribeAudio(config, file)
        setTranscript(result?.text ?? '')
        if (audioUrlRef.current) {
          URL.revokeObjectURL(audioUrlRef.current)
          audioUrlRef.current = null
        }
        setAudioUrl(null)
      }
    } catch (requestError) {
      // The speech endpoint answers with audio, so a failure can arrive as a
      // blob too: read it back as text to recover the provider's message.
      const failure = requestError as {
        response?: { data?: unknown }
        message?: string
      }
      const payload = failure?.response?.data
      let message = failure?.message ?? t('Request failed')
      if (payload instanceof Blob) {
        try {
          const parsed = JSON.parse(await payload.text()) as {
            error?: { message?: string }
            message?: string
          }
          message = parsed?.error?.message ?? parsed?.message ?? message
        } catch {
          // Keep the transport message when the body is not JSON.
        }
      }
      setError(message)
    } finally {
      setIsWorking(false)
    }
  }, [canRun, config, file, mode, t, text])

  return {
    config,
    updateConfig,
    mode,
    setMode,
    text,
    setText,
    file,
    setFile,
    audioUrl,
    transcript,
    isWorking,
    error,
    canRun,
    run,
  }
}
