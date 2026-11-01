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
import { render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { PricingModel } from '../types'
import { PricingSidebar } from '../components/pricing-sidebar'

function pricingModel(
  name: string,
  groups: string[],
  ratio = 1
): PricingModel {
  return {
    id: name.length,
    model_name: name,
    quota_type: 0,
    model_ratio: ratio,
    completion_ratio: 1,
    enable_groups: groups,
  } as PricingModel
}

function renderSidebar(overrides: { models: PricingModel[]; groups: string[] }) {
  return render(
    <PricingSidebar
      quotaTypeFilter='all'
      endpointTypeFilter='all'
      vendorFilter='all'
      groupFilter='all'
      tagFilter='all'
      onQuotaTypeChange={vi.fn()}
      onEndpointTypeChange={vi.fn()}
      onVendorChange={vi.fn()}
      onGroupChange={vi.fn()}
      onTagChange={vi.fn()}
      vendors={[]}
      groups={overrides.groups}
      groupRatios={{ default: 1, premium: 2 }}
      tags={[]}
      models={overrides.models}
      hasActiveFilters={false}
      onClearFilters={vi.fn()}
    />
  )
}

// Every other facet in this panel says how many models it holds, and a group
// that serves nothing currently looks identical to one serving half the page.
// The count is what tells a buyer whether picking the group will leave them
// with an empty square.
describe('pricing sidebar group counts', () => {
  it('counts the models each group serves and puts the ratio in the badge', () => {
    renderSidebar({
      groups: ['default', 'premium'],
      models: [
        pricingModel('shared-model', ['default', 'premium']),
        pricingModel('default-only', ['default']),
      ],
    })

    // The collapsible is the smallest ancestor holding both the heading and
    // the chips, so it scopes the queries to the group facet.
    const groupSection = screen
      .getByText('Groups')
      .closest('[data-slot="collapsible"]') as HTMLElement
    expect(groupSection).not.toBeNull()
    const groups = within(groupSection)

    // The chip carries label, count and ratio; the ratio badge stays the badge.
    const defaultChip = groups.getByRole('button', { name: /default/ })
    expect(defaultChip.textContent).toContain('2')
    expect(defaultChip.textContent).toContain('x1')

    const premiumChip = groups.getByRole('button', { name: /premium/ })
    expect(premiumChip.textContent).toContain('1')
    expect(premiumChip.textContent).toContain('x2')
  })

  it('shows zero for a group that serves none of the current listing', () => {
    renderSidebar({
      groups: ['default', 'empty-group'],
      models: [pricingModel('default-only', ['default'])],
    })

    const groupSection = screen
      .getByText('Groups')
      .closest('[data-slot="collapsible"]') as HTMLElement
    const emptyChip = within(groupSection).getByRole('button', {
      name: /empty-group/,
    })
    expect(emptyChip.textContent).toContain('0')
  })
})
