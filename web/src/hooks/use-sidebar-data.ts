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
import {
  Activity,
  Box,
  ClipboardList,
  CreditCard,
  Cpu,
  Download,
  ExternalLink,
  FileText,
  FlaskConical,
  Key,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  PlugZap,
  Radio,
  RotateCcw,
  ServerCog,
  Settings,
  ShieldCheck,
  Ticket,
  User,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { useStatus } from '@/hooks/use-status'
import { getCustomTabIcon, parseCustomTabs } from '@/lib/custom-tabs'
import { parseSidebarAdminLinksFromStatus } from '@/lib/nav-modules'
import { ROLE } from '@/lib/roles'

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const { status } = useStatus()
  const customTabs = parseCustomTabs(status?.custom_tabs)

  const customItems = (category: string) =>
    customTabs
      .filter((tab) => tab.category === category)
      .map((tab) => ({
        title: tab.label,
        url: tab.external ? tab.url : `/custom-tab/${tab.id}`,
        icon: getCustomTabIcon(tab.icon),
        external: tab.external,
      }))

  // External tools the operator runs outside this console. Deliberately read
  // from configuration rather than written here: the addresses are
  // deployment-specific, so a hardcoded one would leak one deployment's
  // internal host into every other.
  const adminLinks = parseSidebarAdminLinksFromStatus(
    status as Record<string, unknown> | null
  )

  return {
    navGroups: [
      {
        id: 'chat',
        title: t('Chat'),
        items: [
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
          },
          {
            title: t('Chat'),
            icon: MessageSquare,
            type: 'chat-presets',
          },
          ...customItems('chat'),
        ],
      },
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Status Check'),
            url: '/status/perf',
            icon: Cpu,
          },
          {
            // Two views of channel health: the perf view above reads relay
            // traffic (cache hit rate, TTFT history), this one reads live probe
            // results and the mirrored upstream page.
            title: t('Channel Status'),
            url: '/status',
            icon: Radio,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          {
            title: t('Audit Logs'),
            url: '/usage-logs/audit',
            icon: ClipboardList,
            // The audit trail records the operator's address and every
            // account's sign-in history. Ordinary users do not need it, and
            // exposing it republishes the operator's own address.
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
          },
          ...customItems('general'),
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Wallet'),
            url: '/wallet',
            icon: Wallet,
          },
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
          {
            title: t('Security & Access'),
            url: '/security',
            icon: ShieldCheck,
          },
          ...customItems('personal'),
        ],
      },
      {
        id: 'resources',
        title: t('Resources'),
        items: [
          {
            title: t('Tool Downloads'),
            url: '/downloads',
            icon: Download,
          },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          // Titles come from the operator's own configuration, so they are
          // rendered as written rather than passed through t().
          ...adminLinks.map((link) => ({
            title: link.title,
            url: link.url,
            icon: ExternalLink,
            requiredRole: ROLE.ADMIN,
          })),
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
          },
          {
            title: t('Models'),
            url: '/models/metadata',
            icon: Box,
          },
          {
            title: t('Users'),
            url: '/users',
            icon: Users,
          },
          {
            title: t('Redemption Codes'),
            url: '/redemption-codes',
            icon: Ticket,
          },
          {
            title: t('Subscriptions'),
            url: '/subscriptions',
            icon: CreditCard,
          },
          {
            title: t('Refund Requests'),
            url: '/refunds',
            icon: RotateCcw,
          },
          {
            title: t('System Info'),
            url: '/system-info',
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('Task Plugins'),
            url: '/task-plugins',
            icon: PlugZap,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('System Settings'),
            url: '/system-settings/site',
            activeUrls: ['/system-settings'],
            icon: Settings,
          },
          ...customItems('admin'),
        ],
      },
    ],
  }
}
