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
import { createFileRoute, redirect } from '@tanstack/react-router'

/**
 * Retired. The relay-traffic summary and the probe view listed the same models
 * with the same latency and availability figures, so keeping two pages meant
 * reading the same numbers twice.
 *
 * The perf payload is already merged into /status (every model row there
 * carries passive traffic, so an upstream-sampled model still reports). This
 * route stays as a redirect so old links and bookmarks land somewhere useful.
 */
export const Route = createFileRoute('/_authenticated/status/perf')({
  beforeLoad: () => {
    throw redirect({ to: '/status', replace: true })
  },
})
