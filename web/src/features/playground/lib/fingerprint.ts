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
// The playground free allowance is accounted per visitor rather than per
// account, so the browser sends a stable id it keeps in local storage. Clearing
// storage only resets the id: the server counts the caller IP as well, so the
// allowance is not restored.
const FINGERPRINT_STORAGE_KEY = 'playground_client_fingerprint'

export const PLAYGROUND_FINGERPRINT_HEADER = 'X-Client-Fingerprint'

export function getPlaygroundFingerprint(): string {
  try {
    const existing = window.localStorage.getItem(FINGERPRINT_STORAGE_KEY)
    if (existing) {
      return existing
    }
    const created = crypto.randomUUID()
    window.localStorage.setItem(FINGERPRINT_STORAGE_KEY, created)
    return created
  } catch {
    return ''
  }
}
