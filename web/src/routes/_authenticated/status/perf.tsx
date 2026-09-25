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
import { createFileRoute } from '@tanstack/react-router'

import { StatusCheck } from '@/features/status-check'

/**
 * The relay-traffic view of channel health: cache hit rate and TTFT history
 * read from real traffic, as opposed to /status which reads live probe results
 * and the mirrored upstream page. The sidebar has always linked here as "Status
 * Check"; the route itself was missing, so the link 404'd.
 */
export const Route = createFileRoute('/_authenticated/status/perf')({
  component: StatusCheck,
})
