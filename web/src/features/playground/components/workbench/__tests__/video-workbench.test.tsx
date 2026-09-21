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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  fetchVideoTask,
  getUserGroups,
  getUserModels,
  submitVideoTask,
} from '../../../api'
import { VideoWorkbench } from '../video-workbench'

vi.mock('../../../api', () => ({
  getUserGroups: vi.fn(),
  getUserModels: vi.fn(),
  submitVideoTask: vi.fn(),
  fetchVideoTask: vi.fn(),
}))

const workbenchLabels = {
  Download: 'Download',
  'Add reference image': 'Add reference image',
  'Generate video': 'Generate video',
  Prompt: 'Prompt',
  Queued: 'Queued',
  'Reference images': 'Reference images',
}

const groups = [{ label: 'default', value: 'default', ratio: 1 }]
const models = [
  { label: 'agnes-video-2.5', value: 'agnes-video-2.5' },
  { label: 'agnes-video-v2.0', value: 'agnes-video-v2.0' },
]

function renderWorkbench() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <VideoWorkbench />
    </QueryClientProvider>
  )
}

describe('VideoWorkbench submission', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', workbenchLabels)
  })

  beforeEach(() => {
    vi.mocked(getUserGroups).mockResolvedValue(groups)
    vi.mocked(getUserModels).mockResolvedValue(models)
    vi.mocked(submitVideoTask).mockResolvedValue({
      id: 'task_video_1',
      task_id: 'task_video_1',
    })
    vi.mocked(fetchVideoTask).mockResolvedValue({
      code: 'success',
      data: {
        task_id: 'task_video_1',
        status: 'IN_PROGRESS',
        progress: '30%',
      },
    })
  })

  test('keeps the submit button disabled until a prompt is entered', async () => {
    const user = userEvent.setup()
    renderWorkbench()

    const submit = await screen.findByRole('button', {
      name: 'Generate video',
    })
    await waitFor(() => expect(submit).toBeDisabled())

    await user.type(
      screen.getByLabelText('Prompt'),
      'a calico cat playing piano'
    )

    await waitFor(() => expect(submit).toBeEnabled())
  })

  test('submits the prompt, mode and duration of a text to video request', async () => {
    const user = userEvent.setup()
    renderWorkbench()

    const submit = await screen.findByRole('button', {
      name: 'Generate video',
    })
    await user.type(
      screen.getByLabelText('Prompt'),
      'a calico cat playing piano'
    )
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(submit)

    await waitFor(() =>
      expect(vi.mocked(submitVideoTask)).toHaveBeenCalledWith(
        { model: 'agnes-video-2.5', group: 'default', duration: 5 },
        {
          mode: 'text',
          prompt: 'a calico cat playing piano',
          referenceImages: [],
          firstFrame: '',
          lastFrame: '',
        }
      )
    )
  })

  test('requires a reference image before submitting in reference mode', async () => {
    const user = userEvent.setup()
    renderWorkbench()

    const submit = await screen.findByRole('button', {
      name: 'Generate video',
    })
    await user.type(screen.getByLabelText('Prompt'), 'smoke rising')
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(screen.getByRole('button', { name: 'Reference images' }))

    expect(
      screen.getByRole('button', { name: 'Add reference image' })
    ).toBeInTheDocument()
    await waitFor(() => expect(submit).toBeDisabled())
  })

  test('shows the finished video once the task settles', async () => {
    vi.mocked(fetchVideoTask).mockResolvedValue({
      code: 'success',
      data: {
        task_id: 'task_video_1',
        status: 'SUCCESS',
        progress: '100%',
        result_url: 'https://cdn.example/video.mp4',
      },
    })
    const user = userEvent.setup()
    renderWorkbench()

    const submit = await screen.findByRole('button', {
      name: 'Generate video',
    })
    await user.type(screen.getByLabelText('Prompt'), 'a calm ocean')
    await waitFor(() => expect(submit).toBeEnabled())

    // The poll timer is armed when the task is submitted, so the clock is
    // faked only for the click and the timer it schedules.
    vi.useFakeTimers()
    try {
      fireEvent.click(submit)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      expect(screen.getByText('Queued')).toBeInTheDocument()

      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000)
      })

      expect(vi.mocked(fetchVideoTask)).toHaveBeenCalledWith('task_video_1')
      expect(screen.getByText('Download').closest('a')).toHaveAttribute(
        'href',
        'https://cdn.example/video.mp4'
      )
    } finally {
      vi.useRealTimers()
    }
  })
})
