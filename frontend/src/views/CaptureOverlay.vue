<script setup lang="ts">
/*
 * CaptureOverlay — Snipaste-style region picker.
 *
 * Three layers, painted in order:
 *   1. Dark mask + selection  : a single SVG that draws a 0.4 alpha
 *                               black rectangle over the whole screen
 *                               with the selection cut out via fill-rule
 *                               evenodd. The Wails window itself is
 *                               transparent, so the cut-out shows the live
 *                               desktop rather than a stale screenshot.
 *   2. Toolbar                : floats just outside the bottom-right of
 *                               the selection rectangle (Snipaste rule).
 *
 * Interaction model (kept minimal per user choice):
 *   • If no selection yet: mousedown drags out a new rectangle.
 *   • If selection exists: mousedown INSIDE moves it; OUTSIDE re-drags.
 *   • Esc / Cancel button → emit('cancel')
 *   • Enter / Upload button → emit('confirm', rect)
 */
import { computed, onMounted, onUnmounted, ref } from 'vue'

interface Props {
  /** Logical (CSS) width of the primary display. */
  width: number
  /** Logical (CSS) height of the primary display. */
  height: number
}
const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'confirm', rect: { x: number; y: number; w: number; h: number }): void
  (e: 'cancel'): void
}>()

// Selection rect stored in CSS pixels. null = nothing selected yet.
interface Rect {
  x: number
  y: number
  w: number
  h: number
}
const rect = ref<Rect | null>(null)

// Drag state machine.
type DragMode = 'idle' | 'creating' | 'moving'
const dragMode = ref<DragMode>('idle')
// For 'creating': the anchor where mousedown started.
// For 'moving' : the mouse offset relative to the rect's top-left.
const dragAnchor = ref({ x: 0, y: 0 })

// SVG path for "the entire screen with the selection rect cut out".
// We rely on fill-rule:evenodd to subtract the inner rectangle.
const maskPath = computed(() => {
  const outer = `M0 0 H${props.width} V${props.height} H0 Z`
  if (!rect.value) return outer
  const r = rect.value
  // Inner rect drawn in the OPPOSITE winding order so evenodd cuts it.
  const inner = `M${r.x} ${r.y} H${r.x + r.w} V${r.y + r.h} H${r.x} Z`
  return outer + ' ' + inner
})

// Toolbar placement: anchor to bottom-right of the selection, but shove
// it to the inside top-right if the selection is too close to the screen
// edge to keep the buttons visible.
const TOOLBAR_W = 220
const TOOLBAR_H = 40
const GAP = 8
const toolbarPos = computed(() => {
  if (!rect.value) return null
  const r = rect.value
  let x = r.x + r.w - TOOLBAR_W
  let y = r.y + r.h + GAP
  if (y + TOOLBAR_H > props.height) {
    // No room below: put it inside the selection's bottom-right.
    y = r.y + r.h - TOOLBAR_H - GAP
  }
  if (x < 0) x = 0
  if (x + TOOLBAR_W > props.width) x = props.width - TOOLBAR_W
  return { x, y }
})

const insideRect = (px: number, py: number, r: Rect | null) => {
  if (!r) return false
  return px >= r.x && px <= r.x + r.w && py >= r.y && py <= r.y + r.h
}

function onMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  const px = e.clientX
  const py = e.clientY
  if (rect.value && insideRect(px, py, rect.value)) {
    // Move existing selection.
    dragMode.value = 'moving'
    dragAnchor.value = { x: px - rect.value.x, y: py - rect.value.y }
  } else {
    // Start a new selection.
    dragMode.value = 'creating'
    dragAnchor.value = { x: px, y: py }
    rect.value = { x: px, y: py, w: 0, h: 0 }
  }
}

function onMouseMove(e: MouseEvent) {
  if (dragMode.value === 'idle') return
  const px = e.clientX
  const py = e.clientY
  if (dragMode.value === 'creating') {
    const ax = dragAnchor.value.x
    const ay = dragAnchor.value.y
    rect.value = {
      x: Math.min(ax, px),
      y: Math.min(ay, py),
      w: Math.abs(px - ax),
      h: Math.abs(py - ay),
    }
  } else if (dragMode.value === 'moving' && rect.value) {
    let nx = px - dragAnchor.value.x
    let ny = py - dragAnchor.value.y
    // Clamp inside the screen so the selection cannot be dragged off-canvas.
    nx = Math.max(0, Math.min(nx, props.width - rect.value.w))
    ny = Math.max(0, Math.min(ny, props.height - rect.value.h))
    rect.value = { ...rect.value, x: nx, y: ny }
  }
}

function onMouseUp() {
  if (dragMode.value === 'creating' && rect.value) {
    // Discard zero/tiny selections — the user probably clicked accidentally.
    if (rect.value.w < 4 || rect.value.h < 4) {
      rect.value = null
    }
  }
  dragMode.value = 'idle'
}

function onConfirm() {
  if (!rect.value) return
  emit('confirm', { ...rect.value })
}

function onCancel() {
  emit('cancel')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    onCancel()
  } else if (e.key === 'Enter' && rect.value) {
    e.preventDefault()
    onConfirm()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})

// Selection size pill (Snipaste shows "WxH" near the top-left of selection).
const sizeLabel = computed(() => {
  if (!rect.value) return ''
  return `${Math.round(rect.value.w)} × ${Math.round(rect.value.h)}`
})
</script>

<template>
  <div
    class="overlay-root"
    :style="{ width: width + 'px', height: height + 'px' }"
    @mousedown="onMouseDown"
    @mousemove="onMouseMove"
    @mouseup="onMouseUp"
    @contextmenu.prevent="onCancel"
  >
    <!-- Layer 1: mask with selection cut out over the live desktop -->
    <svg
      class="mask"
      :width="width"
      :height="height"
      :viewBox="`0 0 ${width} ${height}`"
      preserveAspectRatio="none"
    >
      <path :d="maskPath" fill="rgba(0,0,0,0.45)" fill-rule="evenodd" />
      <!-- Selection border, drawn separately so it stays crisp. -->
      <rect
        v-if="rect"
        :x="rect.x"
        :y="rect.y"
        :width="rect.w"
        :height="rect.h"
        fill="none"
        stroke="#3b82f6"
        stroke-width="1.5"
        vector-effect="non-scaling-stroke"
      />
    </svg>

    <!-- Layer 3a: size readout floating above the selection -->
    <div
      v-if="rect && rect.w > 0 && rect.h > 0"
      class="size-pill"
      :style="{
        left: rect.x + 'px',
        top: Math.max(0, rect.y - 26) + 'px',
      }"
    >
      {{ sizeLabel }}
    </div>

    <!-- Layer 3b: action toolbar -->
    <div
      v-if="toolbarPos && rect && rect.w >= 4 && rect.h >= 4"
      class="toolbar"
      :style="{ left: toolbarPos.x + 'px', top: toolbarPos.y + 'px' }"
      @mousedown.stop
    >
      <button class="btn cancel" @click="onCancel" title="Esc">Cancel</button>
      <button class="btn primary" @click="onConfirm" title="Enter">
        Upload &amp; copy
      </button>
    </div>

    <!-- Hint shown before the user has dragged anything. -->
    <div v-if="!rect" class="hint">
      Drag to select an area &nbsp;·&nbsp; Esc to cancel
    </div>
  </div>
</template>

<style scoped>
.overlay-root {
  position: fixed;
  inset: 0;
  user-select: none;
  cursor: crosshair;
  background: transparent;
  overflow: hidden;
}

.mask {
  position: absolute;
  left: 0;
  top: 0;
  pointer-events: none;
}

.size-pill {
  position: absolute;
  padding: 2px 8px;
  font-size: 12px;
  font-weight: 500;
  color: #fff;
  background: rgba(15, 23, 42, 0.78);
  border-radius: 4px;
  pointer-events: none;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}

.toolbar {
  position: absolute;
  display: flex;
  gap: 6px;
  align-items: center;
  padding: 4px;
  background: rgba(28, 28, 32, 0.94);
  border-radius: 8px;
  box-shadow: 0 6px 22px rgba(0, 0, 0, 0.4);
  cursor: default;
}

.btn {
  border: 0;
  border-radius: 5px;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  font-family: inherit;
  transition: background-color 120ms ease;
}
.btn.cancel {
  background: transparent;
  color: #d1d5db;
}
.btn.cancel:hover {
  background: rgba(255, 255, 255, 0.08);
}
.btn.primary {
  background: #3b82f6;
  color: #fff;
}
.btn.primary:hover {
  background: #2563eb;
}

.hint {
  position: absolute;
  left: 50%;
  top: 24px;
  transform: translateX(-50%);
  padding: 6px 14px;
  font-size: 12px;
  color: #f3f4f6;
  background: rgba(0, 0, 0, 0.5);
  border-radius: 999px;
  pointer-events: none;
  letter-spacing: 0.02em;
}
</style>
