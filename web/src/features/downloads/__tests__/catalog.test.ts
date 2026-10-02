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
import { describe, expect, it } from 'vitest'

import {
  CHATGPT_STORE_PRODUCT_ID,
  CHATGPT_STORE_PROTOCOL_URL,
  CHATGPT_STORE_WEB_URL,
  buildDownloadCatalog,
} from '../constants'
import type { CcSwitchRelease } from '../lib/cc-switch'
import type { ToolDownload } from '../types'

const release: CcSwitchRelease = {
  version: '1.2.3',
  source: 'fallback',
  assets: [],
}

function requireTool(tools: ToolDownload[], id: string): ToolDownload {
  const tool = tools.find((candidate) => candidate.id === id)
  if (!tool) throw new Error(`tool ${id} is missing from the catalog`)
  return tool
}

describe('download catalog', () => {
  it('offers every tool the page advertises', () => {
    const ids = buildDownloadCatalog(release).map((tool) => tool.id)
expect(ids).toEqual(['cc-switch', 'claude-desktop', 'chatgpt'])
  })
})

describe('ChatGPT store link', () => {
  // Windows ships ChatGPT through the Store, so the action has to open the
  // Store app on the product page rather than fetch a file.
  it('opens the Microsoft Store product page over the store protocol', () => {
    const tool = requireTool(buildDownloadCatalog(release), 'chatgpt')
    expect(tool.links).toHaveLength(1)

    const [link] = tool.links
    expect(link.platform).toBe('windows')
    expect(link.url).toBe(
      `ms-windows-store://pdp/?ProductId=${CHATGPT_STORE_PRODUCT_ID}`
    )
    expect(link.url).toBe(CHATGPT_STORE_PROTOCOL_URL)
  })

  it('labels the action for the store rather than calling it a download', () => {
    const tool = requireTool(buildDownloadCatalog(release), 'chatgpt')
    expect(tool.links[0].actionKey).toBe('Open ChatGPT in the Store')
  })

  it('keeps a web destination for platforms without the store protocol', () => {
    const tool = requireTool(buildDownloadCatalog(release), 'chatgpt')
    expect(tool.backupLinks).toEqual([
      {
        labelKey: 'Microsoft Store page',
        url: `https://apps.microsoft.com/detail/${CHATGPT_STORE_PRODUCT_ID}`,
      },
    ])
    expect(CHATGPT_STORE_WEB_URL).toBe(
      `https://apps.microsoft.com/detail/${CHATGPT_STORE_PRODUCT_ID}`
    )
  })

  it('points both destinations at the same product', () => {
    const protocolId = new URL(CHATGPT_STORE_PROTOCOL_URL).searchParams.get(
      'ProductId'
    )
    const webId = CHATGPT_STORE_WEB_URL.split('/').at(-1)
    expect(protocolId).toBe(webId)
    expect(protocolId).toBe(CHATGPT_STORE_PRODUCT_ID)
  })

  // Copying `ms-windows-store://…` into a browser only works on Windows, so the
  // row still needs the description to explain where the app comes from.
  it('explains the store in the tool description', () => {
    const tool = requireTool(buildDownloadCatalog(release), 'chatgpt')
    expect(tool.descriptionKey).toContain('Microsoft Store')
  })
})
