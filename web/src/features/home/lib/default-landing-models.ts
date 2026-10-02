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

/**
 * Shape of the landing page's data.
 *
 * Deliberately carries no shipped content: the hook fills it from the
 * `LandingModels` option and from what the site actually serves, so there is
 * nothing here to go stale.
 */
export type RouteKind = 'priority' | 'free' | 'value'

export type RouteGroup = {
  id: string
  kind: RouteKind
  group: string
  description: string
  models: string[]
}

export type CoverageRow = {
  mark: string
  family: string
  description: string
  routeType: string
}

export type LandingModelsConfig = {
  serviceRoutes: RouteGroup[]
  coverage: CoverageRow[]
}
