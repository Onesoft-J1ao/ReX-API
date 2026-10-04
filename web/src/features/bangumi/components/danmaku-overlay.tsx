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
import { useEffect, useRef } from 'react'

import {
  type DanmakuComment,
  commentsInWindow,
  danmakuDuration,
} from '../lib/danmaku'
/**
 * The longest a comment can stay on screen, and therefore how far back the
 * renderer has to look for one that is still visible.
 */
const MAX_DURATION_SECONDS = 12

const FONT = '20px sans-serif'
const LANE_HEIGHT = 30
const HORIZONTAL_GAP = 24
/** Fraction of the canvas height the top and bottom rows may occupy. */
const STATIC_ROW_INSET = 0.12

/**
 * Draws the bullet comments over the video.
 *
 * Canvas rather than positioned elements: the comments have to move in step
 * with `video.currentTime`, and reading that inside a requestAnimationFrame
 * loop means pausing, seeking and stuttering all stay correct without any
 * pause/seek plumbing. Positioned elements animated by CSS would need the
 * animation state reset by hand on every seek.
 */
export function DanmakuOverlay(props: {
  comments: DanmakuComment[]
  video: HTMLVideoElement | null
  enabled: boolean
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)

  const { comments, video, enabled } = props

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || !video || !enabled) return

    const context = canvas.getContext('2d')
    if (!context) return

    let frame = 0

    const resize = () => {
      const ratio = window.devicePixelRatio || 1
      const rect = canvas.getBoundingClientRect()
      canvas.width = Math.max(1, Math.floor(rect.width * ratio))
      canvas.height = Math.max(1, Math.floor(rect.height * ratio))
      context.setTransform(ratio, 0, 0, ratio, 0, 0)
    }
    resize()

    const observer = new ResizeObserver(resize)
    observer.observe(canvas)

    const draw = () => {
      frame = window.requestAnimationFrame(draw)

      const width = canvas.clientWidth
      const height = canvas.clientHeight
      context.clearRect(0, 0, width, height)
      if (width === 0 || height === 0) return

      const now = video.currentTime
      const visible = commentsInWindow(comments, now, MAX_DURATION_SECONDS)
      if (visible.length === 0) return

      context.font = FONT
      context.textBaseline = 'top'

      const laneCount = Math.max(1, Math.floor(height / LANE_HEIGHT))
      const topLanes = Math.max(1, Math.floor(laneCount * STATIC_ROW_INSET))
      // Right edge of the last comment placed in each scrolling lane. A lane
      // only accepts a new comment once its previous one has moved far enough
      // left that the two cannot touch.
      const laneRightEdge: number[] = []

      for (const comment of visible) {
        const textWidth = context.measureText(comment.text).width
        const elapsed = now - comment.time
        const duration = danmakuDuration(comment.text)

        if (comment.mode === 4 || comment.mode === 5) {
          const y =
            comment.mode === 5
              ? topLanes * LANE_HEIGHT
              : height - (topLanes + 1) * LANE_HEIGHT
          drawText(context, comment, (width - textWidth) / 2, y)
          continue
        }

        // Modes 1, 2 and 6 all scroll; only the direction differs, and 6 is rare
        // enough that treating it as a right-to-left scroll is not worth a
        // second layout path.
        const travel = width + textWidth
        const progress = Math.min(Math.max(elapsed / duration, 0), 1)
        const x = width - progress * travel

        let lane = laneRightEdge.findIndex(
          (right) => right <= width - HORIZONTAL_GAP
        )
        if (lane === -1) {
          lane = laneRightEdge.length
          if (lane >= laneCount - topLanes) continue
        }
        laneRightEdge[lane] = x + textWidth

        drawText(context, comment, x, lane * LANE_HEIGHT)
      }
    }

    frame = window.requestAnimationFrame(draw)
    return () => {
      window.cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [comments, video, enabled])

  if (!enabled) return null

  return (
    <canvas
      ref={canvasRef}
      aria-hidden='true'
      className='pointer-events-none absolute inset-0 h-full w-full'
    />
  )
}

/**
 * Draws one comment with the outline that keeps it legible over both a bright
 * sky and a dark room, which no single fill colour manages on its own.
 */
function drawText(
  context: CanvasRenderingContext2D,
  comment: DanmakuComment,
  x: number,
  y: number
) {
  context.lineWidth = 3
  context.strokeStyle = 'rgba(0, 0, 0, 0.75)'
  context.strokeText(comment.text, x, y)
  context.fillStyle = comment.color
  context.fillText(comment.text, x, y)
}
