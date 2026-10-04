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
 * Minimal declarations for webtorrent 3.x, which ships no types of its own.
 *
 * Only the surface the player uses is declared, so the compiler still checks
 * every call this feature makes while nothing is asserted about the parts it
 * does not touch.
 */
declare module 'webtorrent' {
  export interface TorrentFile {
    name: string
    length: number
    /** Streams the file into a media element, seeking on demand. */
    renderTo: (
      element: HTMLMediaElement,
      options?: {
        autoplay?: boolean
        controls?: boolean
        muted?: boolean
      }
    ) => void
    getBlobURL: (callback: (error: Error | null, url: string) => void) => void
  }

  export interface Torrent {
    infoHash: string
    magnetURI: string
    files: TorrentFile[]
    /** Peers currently connected over WebRTC. */
    numPeers: number
    /** Bytes per second, maintained by the client. */
    downloadSpeed: number
    /** Bytes per second, maintained by the client. */
    uploadSpeed: number
    /** Fraction downloaded, 0..1. */
    progress: number
    /** Bytes downloaded so far. */
    downloaded: number
    destroyed: boolean
    on: (event: string, listener: (...args: never[]) => void) => void
  }

  export interface WebTorrentOptions {
    /** Public WebRTC trackers to announce to, in addition to the torrent's own. */
    announce?: string[]
    /** Keeps the client from throwing when the store cannot be destroyed. */
    destroyStoreOnDestroy?: boolean
    /** In the browser the store is memory-backed; this bounds it. */
    maxConns?: number
  }

  export default class WebTorrent {
    constructor(options?: WebTorrentOptions)
    add: (
      torrentId: string,
      options?: WebTorrentOptions,
      callback?: (torrent: Torrent) => void
    ) => Torrent
    remove: (torrentId: string, callback?: () => void) => void
    destroy: (callback?: (error?: Error) => void) => void
    on: (event: string, listener: (...args: never[]) => void) => void
  }
}
