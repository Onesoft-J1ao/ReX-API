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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  fetchThreeDTask,
  getUserGroups,
  getUserModels,
  submitThreeDTask,
} from '../../../api'
import { ThreeDWorkbench } from '../three-d-workbench'

vi.mock('../../../api', () => ({
  getUserGroups: vi.fn(),
  getUserModels: vi.fn(),
  submitThreeDTask: vi.fn(),
  fetchThreeDTask: vi.fn(),
}))

const workbenchLabels = {
  'Generate 3D model': 'Generate 3D model',
  Prompt: 'Prompt',
  Queued: 'Queued',
  'Source task ID': 'Source task ID',
  'Reuse a completed draft task to add textures.':
    'Reuse a completed draft task to add textures.',
}

const groups = [{ label: 'default', value: 'default', ratio: 1 }]
const geometryModels = [{ label: 'meshy-6-draft', value: 'meshy-6-draft' }]
const textureModels = [{ label: 'meshy-6-texture', value: 'meshy-6-texture' }]

function renderWorkbench() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ThreeDWorkbench />
    </QueryClientProvider>
  )
}

describe('ThreeDWorkbench submission', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', workbenchLabels)
  })

  beforeEach(() => {
    vi.mocked(getUserGroups).mockResolvedValue(groups)
    vi.mocked(getUserModels).mockResolvedValue(geometryModels)
    vi.mocked(submitThreeDTask).mockResolvedValue({
      id: 'task_3d_1',
      object: '3d',
      status: 'queued',
    })
    vi.mocked(fetchThreeDTask).mockResolvedValue({
      id: 'task_3d_1',
      status: 'in_progress',
      progress: 40,
    })
  })

  test('keeps the submit button disabled until the model can be submitted', async () => {
    const user = userEvent.setup()
    renderWorkbench()

    const submit = await screen.findByRole('button', {
      name: 'Generate 3D model',
    })
    await waitFor(() => expect(submit).toBeDisabled())

    await user.type(screen.getByLabelText('Prompt'), 'a low poly fox')

    await waitFor(() => expect(submit).toBeEnabled())
  })

  test('submits the prompt and the art style of a geometry request', async () => {
    const user = userEvent.setup()
    renderWorkbench()

    const submit = await screen.findByRole('button', {
      name: 'Generate 3D model',
    })
    await user.type(screen.getByLabelText('Prompt'), 'a low poly fox')
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(submit)

    await waitFor(() =>
      expect(vi.mocked(submitThreeDTask)).toHaveBeenCalledWith(
        { model: 'meshy-6-draft', group: 'default', artStyle: '' },
        'a low poly fox',
        '',
        ''
      )
    )
    expect(screen.getByText('Queued')).toBeInTheDocument()
  })

  test('asks for a source task instead of a prompt for texture models', async () => {
    vi.mocked(getUserModels).mockResolvedValue(textureModels)
    renderWorkbench()

    expect(await screen.findByLabelText('Source task ID')).toBeInTheDocument()
    expect(
      screen.getByText('Reuse a completed draft task to add textures.')
    ).toBeInTheDocument()
    expect(screen.queryByLabelText('Prompt')).not.toBeInTheDocument()
  })

  test('keeps the submit button disabled until a texture source task is given', async () => {
    vi.mocked(getUserModels).mockResolvedValue(textureModels)
    const user = userEvent.setup()
    renderWorkbench()

    const submit = await screen.findByRole('button', {
      name: 'Generate 3D model',
    })
    await waitFor(() => expect(submit).toBeDisabled())

    await user.type(screen.getByLabelText('Source task ID'), 'task_draft_1')

    await waitFor(() => expect(submit).toBeEnabled())
  })
})
