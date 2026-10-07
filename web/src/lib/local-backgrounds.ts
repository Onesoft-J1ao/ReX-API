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
 * Bundled backdrop pool shared by the landing page and the public layout.
 *
 * One image is drawn at random per mount when an operator has not configured
 * an explicit `background_image`, so the site does not wear the same face on
 * every visit. Files live in `web/public/backgrounds/` and are served from the
 * site root, so a deployment that replaces them keeps the same paths.
 */
export const LOCAL_BACKGROUNDS = [
  '/backgrounds/bg-01.jpg',
  '/backgrounds/bg-02.jpg',
  '/backgrounds/bg-03.jpg',
  '/backgrounds/bg-04.jpg',
  '/backgrounds/bg-05.jpg',
  '/backgrounds/bg-06.jpg',
  '/backgrounds/bg-07.jpg',
  '/backgrounds/bg-08.jpg',
] as const

/** Remote stand-in used when the bundled pool is somehow unavailable. */
export const FALLBACK_BACKGROUND =
  'https://images.unsplash.com/photo-1515694346937-94d85e41e6f0?auto=format&fit=crop&w=2400&q=88'

export function pickLocalBackground(): string {
  const index = Math.floor(Math.random() * LOCAL_BACKGROUNDS.length)
  return LOCAL_BACKGROUNDS[index] ?? FALLBACK_BACKGROUND
}
