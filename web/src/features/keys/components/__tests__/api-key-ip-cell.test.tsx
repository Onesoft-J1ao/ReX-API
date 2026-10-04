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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ApiKeyIpCell } = await import('../api-key-ip-cell')
const { apiKeySchema } = await import('../../types')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        IP: 'IP',
        Tokens: 'Tokens',
        Requests: 'Requests',
        'Last Seen': 'Last Seen',
        'Not recorded': 'Not recorded',
        'No recorded usage': 'No recorded usage',
        'Caller IPs': 'Caller IPs',
        '{{count}} IP': '{{count}} IP',
        '{{count}} tokens from unrecorded IPs':
          '{{count}} tokens from unrecorded IPs',
        'Caller IP usage: {{summary}}': 'Caller IP usage: {{summary}}',
        'Token usage by caller IP': 'Token usage by caller IP',
        'Showing the top {{shown}} of {{total}} IP addresses by tokens consumed.':
          'Showing the top {{shown}} of {{total}} IP addresses by tokens consumed.',
        'All {{total}} recorded IP addresses are listed.':
          'All {{total}} recorded IP addresses are listed.',
        'Usage recorded before IP logging was enabled has no address and cannot be attributed. It is counted above but cannot be traced to a caller.':
          'Usage recorded before IP logging was enabled has no address and cannot be attributed. It is counted above but cannot be traced to a caller.',
      },
    },
  },
})

const apiKey = apiKeySchema.parse({
  id: 1,
  name: 'free promo key',
  key: 'sk-test',
  status: 1,
  remain_quota: 0,
  used_quota: 1000,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1700000000,
  accessed_time: 1700000000,
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
})

function CellHarness(props: { stat?: Parameters<typeof ApiKeyIpCell>[0]['stat'] }) {
  return (
    <I18nextProvider i18n={i18n}>
      <ApiKeyIpCell apiKey={apiKey} stat={props.stat} />
    </I18nextProvider>
  )
}

describe('API key caller IP cell', () => {
  test('reports no recorded usage when the token has no log rows', () => {
    render(<CellHarness />)
    expect(screen.getByText('No recorded usage')).toBeInTheDocument()
  })

  test('leads with the distinct IP count and total tokens', () => {
    render(
      <CellHarness
        stat={{
          token_id: 1,
          total_ips: 340,
          total_tokens: 500_000_000,
          total_requests: 900,
          unknown_tokens: 0,
          unknown_requests: 0,
          max_ips_per_token: 20,
          ips: [],
        }}
      />
    )
    expect(screen.getByText(/340 IP/)).toBeInTheDocument()
    expect(screen.getByText(/500M Tokens/)).toBeInTheDocument()
  })

  test('lists each address with its own tokens inside the popover', async () => {
    const user = userEvent.setup()
    render(
      <CellHarness
        stat={{
          token_id: 1,
          total_ips: 2,
          total_tokens: 300,
          total_requests: 3,
          unknown_tokens: 0,
          unknown_requests: 0,
          max_ips_per_token: 20,
          ips: [
            {
              token_id: 1,
              ip: '198.51.100.4',
              prompt_tokens: 200,
              completion_tokens: 100,
              requests: 2,
              quota: 25,
              first_seen: 1700000000,
              last_seen: 1700000900,
            },
            {
              token_id: 1,
              ip: '203.0.113.5',
              prompt_tokens: 0,
              completion_tokens: 0,
              requests: 1,
              quota: 0,
              first_seen: 1700000000,
              last_seen: 1700000000,
            },
          ],
        }}
      />
    )

    await user.click(screen.getByRole('button'))

    expect(screen.getByText('198.51.100.4')).toBeInTheDocument()
    expect(screen.getByText('203.0.113.5')).toBeInTheDocument()
    expect(screen.getByText('300')).toBeInTheDocument()
  })

  test('says the list is capped instead of hiding the remaining addresses', async () => {
    const user = userEvent.setup()
    render(
      <CellHarness
        stat={{
          token_id: 1,
          total_ips: 340,
          total_tokens: 1000,
          total_requests: 500,
          unknown_tokens: 0,
          unknown_requests: 0,
          max_ips_per_token: 20,
          ips: [
            {
              token_id: 1,
              ip: '198.51.100.4',
              prompt_tokens: 1000,
              completion_tokens: 0,
              requests: 500,
              quota: 1,
              first_seen: 1700000000,
              last_seen: 1700000900,
            },
          ],
        }}
      />
    )

    await user.click(screen.getByRole('button'))

    expect(
      screen.getByText(
        'Showing the top 20 of 340 IP addresses by tokens consumed.'
      )
    ).toBeInTheDocument()
  })

  test('states plainly that pre-recording usage cannot be traced to an address', async () => {
    const user = userEvent.setup()
    render(
      <CellHarness
        stat={{
          token_id: 1,
          total_ips: 1,
          total_tokens: 500_000_000,
          total_requests: 400,
          unknown_tokens: 500_000_000,
          unknown_requests: 400,
          max_ips_per_token: 20,
          ips: [
            {
              token_id: 1,
              ip: '',
              prompt_tokens: 500_000_000,
              completion_tokens: 0,
              requests: 400,
              quota: 0,
              first_seen: 1700000000,
              last_seen: 1700000900,
            },
          ],
        }}
      />
    )

    // The unrecorded total is surfaced in the cell, before the popover opens,
    // so a clean-looking row cannot be mistaken for "no abuse".
    expect(
      screen.getByText('500M tokens from unrecorded IPs')
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button'))

    expect(screen.getByText('Not recorded')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Usage recorded before IP logging was enabled has no address and cannot be attributed. It is counted above but cannot be traced to a caller.'
      )
    ).toBeInTheDocument()
  })
})
