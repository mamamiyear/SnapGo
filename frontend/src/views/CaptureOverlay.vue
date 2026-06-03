<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'

interface Props {
  width: number
  height: number
}
const props = defineProps<Props>()

interface Rect {
  x: number
  y: number
  w: number
  h: number
}

type Tool = 'pen' | 'rect' | 'ellipse'
interface Point {
  x: number
  y: number
}
interface Annotation {
  tool: Tool
  color: string
  points: Point[]
}

const emit = defineEmits<{
  (
    e: 'confirm',
    payload: { rect: Rect; annotations: Annotation[] }
  ): void
  (
    e: 'copy',
    payload: { rect: Rect; annotations: Annotation[] }
  ): void
  (
    e: 'save',
    payload: { rect: Rect; annotations: Annotation[] }
  ): void
  (e: 'cancel'): void
}>()

const rect = ref<Rect | null>(null)
const annotations = ref<Annotation[]>([])
const draftAnnotation = ref<Annotation | null>(null)
const activeTool = ref<Tool>('pen')
const activeColor = ref('#ef4444')
const paletteOpen = ref(false)

type ResizeHandle = 'n' | 's' | 'e' | 'w' | 'nw' | 'ne' | 'sw' | 'se'
type DragMode = 'idle' | 'creating' | 'moving' | 'resizing' | 'annotating'
const dragMode = ref<DragMode>('idle')
const resizeHandle = ref<ResizeHandle | null>(null)
const dragAnchor = ref({ x: 0, y: 0 })
const startRect = ref<Rect | null>(null)

const colors = [
  '#ef4444',
  '#f97316',
  '#facc15',
  '#22c55e',
  '#06b6d4',
  '#3b82f6',
  '#8b5cf6',
  '#ec4899',
  '#ffffff',
  '#111827',
]

const maskPath = computed(() => {
  const outer = `M0 0 H${props.width} V${props.height} H0 Z`
  if (!rect.value) return outer
  const r = rect.value
  const inner = `M${r.x} ${r.y} H${r.x + r.w} V${r.y + r.h} H${r.x} Z`
  return outer + ' ' + inner
})

const allAnnotations = computed(() => {
  return draftAnnotation.value
    ? [...annotations.value, draftAnnotation.value]
    : annotations.value
})

const selectionAnnotations = computed(() => {
  if (!rect.value) return []
  return allAnnotations.value.map((annotation) => ({
    ...annotation,
    points: annotation.points.map((point) => ({
      x: rect.value!.x + point.x,
      y: rect.value!.y + point.y,
    })),
  }))
})

const sizeLabel = computed(() => {
  if (!rect.value) return ''
  return `${Math.round(rect.value.w)} × ${Math.round(rect.value.h)}`
})

const rightToolbarPos = computed(() => {
  if (!rect.value) return null
  return placeToolbar(rect.value, 304, 40, 'right')
})

const leftToolbarPos = computed(() => {
  if (!rect.value) return null
  return placeToolbar(rect.value, 190, 40, 'left')
})

const handles = computed(() => {
  if (!rect.value) return []
  const r = rect.value
  const cx = r.x + r.w / 2
  const cy = r.y + r.h / 2
  return [
    { name: 'nw', x: r.x, y: r.y },
    { name: 'n', x: cx, y: r.y },
    { name: 'ne', x: r.x + r.w, y: r.y },
    { name: 'e', x: r.x + r.w, y: cy },
    { name: 'se', x: r.x + r.w, y: r.y + r.h },
    { name: 's', x: cx, y: r.y + r.h },
    { name: 'sw', x: r.x, y: r.y + r.h },
    { name: 'w', x: r.x, y: cy },
  ] as Array<{ name: ResizeHandle; x: number; y: number }>
})

function placeToolbar(
  r: Rect,
  toolbarW: number,
  toolbarH: number,
  side: 'left' | 'right'
) {
  const gap = 8
  let x =
    side === 'right'
      ? r.x + r.w - toolbarW
      : r.x
  let y = r.y + r.h + gap
  if (y + toolbarH > props.height) {
    y = r.y + r.h - toolbarH - gap
  }
  x = clamp(x, 0, props.width - toolbarW)
  return { x, y }
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(value, max))
}

function normalizeRect(a: Point, b: Point): Rect {
  return {
    x: clamp(Math.min(a.x, b.x), 0, props.width),
    y: clamp(Math.min(a.y, b.y), 0, props.height),
    w: Math.abs(b.x - a.x),
    h: Math.abs(b.y - a.y),
  }
}

function pointFromEvent(e: MouseEvent): Point {
  return {
    x: clamp(e.clientX, 0, props.width),
    y: clamp(e.clientY, 0, props.height),
  }
}

function localPoint(p: Point) {
  if (!rect.value) return p
  return {
    x: clamp(p.x - rect.value.x, 0, rect.value.w),
    y: clamp(p.y - rect.value.y, 0, rect.value.h),
  }
}

function insideRect(p: Point, r: Rect | null) {
  if (!r) return false
  return p.x >= r.x && p.x <= r.x + r.w && p.y >= r.y && p.y <= r.y + r.h
}

function hitHandle(p: Point): ResizeHandle | null {
  if (!rect.value) return null
  const tolerance = 9
  for (const handle of handles.value) {
    if (
      Math.abs(p.x - handle.x) <= tolerance &&
      Math.abs(p.y - handle.y) <= tolerance
    ) {
      return handle.name
    }
  }
  const r = rect.value
  const nearX = p.x >= r.x - tolerance && p.x <= r.x + r.w + tolerance
  const nearY = p.y >= r.y - tolerance && p.y <= r.y + r.h + tolerance
  if (nearX && Math.abs(p.y - r.y) <= tolerance) return 'n'
  if (nearX && Math.abs(p.y - (r.y + r.h)) <= tolerance) return 's'
  if (nearY && Math.abs(p.x - r.x) <= tolerance) return 'w'
  if (nearY && Math.abs(p.x - (r.x + r.w)) <= tolerance) return 'e'
  return null
}

function onMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  paletteOpen.value = false
  const p = pointFromEvent(e)
  const handle = hitHandle(p)
  if (handle && rect.value) {
    dragMode.value = 'resizing'
    resizeHandle.value = handle
    dragAnchor.value = p
    startRect.value = { ...rect.value }
    return
  }
  if (rect.value && insideRect(p, rect.value)) {
    dragMode.value = 'moving'
    dragAnchor.value = { x: p.x - rect.value.x, y: p.y - rect.value.y }
    return
  }
  dragMode.value = 'creating'
  dragAnchor.value = p
  rect.value = { x: p.x, y: p.y, w: 0, h: 0 }
  annotations.value = []
  draftAnnotation.value = null
}

function onSelectionMouseDown(e: MouseEvent) {
  if (e.button !== 0 || !rect.value) return
  e.stopPropagation()
  paletteOpen.value = false
  if (e.detail >= 2) {
    removeSinglePointAnnotation()
    onCopy()
    return
  }
  const p = pointFromEvent(e)
  const handle = hitHandle(p)
  if (handle) {
    dragMode.value = 'resizing'
    resizeHandle.value = handle
    dragAnchor.value = p
    startRect.value = { ...rect.value }
    return
  }
  dragMode.value = 'annotating'
  const local = localPoint(p)
  draftAnnotation.value = {
    tool: activeTool.value,
    color: activeColor.value,
    points: [local],
  }
}

function onMouseMove(e: MouseEvent) {
  if (dragMode.value === 'idle') return
  const p = pointFromEvent(e)
  if (dragMode.value === 'creating') {
    rect.value = normalizeRect(dragAnchor.value, p)
  } else if (dragMode.value === 'moving' && rect.value) {
    rect.value = {
      ...rect.value,
      x: clamp(p.x - dragAnchor.value.x, 0, props.width - rect.value.w),
      y: clamp(p.y - dragAnchor.value.y, 0, props.height - rect.value.h),
    }
  } else if (dragMode.value === 'resizing') {
    resizeSelection(p)
  } else if (dragMode.value === 'annotating' && draftAnnotation.value) {
    if (activeTool.value === 'pen') {
      draftAnnotation.value.points.push(localPoint(p))
    } else {
      draftAnnotation.value.points = [
        draftAnnotation.value.points[0],
        localPoint(p),
      ]
    }
  }
}

function onMouseUp() {
  if (dragMode.value === 'creating' && rect.value) {
    if (rect.value.w < 4 || rect.value.h < 4) {
      rect.value = null
    }
  }
  if (dragMode.value === 'annotating' && draftAnnotation.value) {
    if (draftAnnotation.value.points.length > 0) {
      annotations.value.push(draftAnnotation.value)
    }
    draftAnnotation.value = null
  }
  dragMode.value = 'idle'
  resizeHandle.value = null
  startRect.value = null
}

function resizeSelection(p: Point) {
  if (!startRect.value || !resizeHandle.value) return
  const r = startRect.value
  let left = r.x
  let top = r.y
  let right = r.x + r.w
  let bottom = r.y + r.h
  if (resizeHandle.value.includes('w')) left = p.x
  if (resizeHandle.value.includes('e')) right = p.x
  if (resizeHandle.value.includes('n')) top = p.y
  if (resizeHandle.value.includes('s')) bottom = p.y

  left = clamp(left, 0, props.width)
  right = clamp(right, 0, props.width)
  top = clamp(top, 0, props.height)
  bottom = clamp(bottom, 0, props.height)
  rect.value = {
    x: Math.min(left, right),
    y: Math.min(top, bottom),
    w: Math.abs(right - left),
    h: Math.abs(bottom - top),
  }
}

function selectTool(tool: Tool) {
  activeTool.value = tool
}

function chooseColor(color: string) {
  activeColor.value = color
  paletteOpen.value = false
}

function onConfirm() {
  emitAction('confirm')
}

function onCopy() {
  emitAction('copy')
}

function onSave() {
  emitAction('save')
}

function emitAction(action: 'confirm' | 'copy' | 'save') {
  if (!rect.value) return
  const payload = {
    rect: { ...rect.value },
    annotations: annotations.value,
  }
  if (action === 'confirm') emit('confirm', payload)
  if (action === 'copy') emit('copy', payload)
  if (action === 'save') emit('save', payload)
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

function undoAnnotation() {
  annotations.value.pop()
}

function removeSinglePointAnnotation() {
  const last = annotations.value[annotations.value.length - 1]
  if (last && last.tool === activeTool.value && last.points.length <= 1) {
    annotations.value.pop()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
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
    <svg
      class="mask"
      :width="width"
      :height="height"
      :viewBox="`0 0 ${width} ${height}`"
      preserveAspectRatio="none"
    >
      <path :d="maskPath" fill="rgba(0,0,0,0.45)" fill-rule="evenodd" />
      <rect
        v-if="rect"
        :x="rect.x"
        :y="rect.y"
        :width="rect.w"
        :height="rect.h"
        fill="transparent"
        stroke="#3b82f6"
        stroke-width="1.5"
        vector-effect="non-scaling-stroke"
      />
      <g v-for="(annotation, index) in selectionAnnotations" :key="index">
        <polyline
          v-if="annotation.tool === 'pen'"
          :points="annotation.points.map((p) => `${p.x},${p.y}`).join(' ')"
          :stroke="annotation.color"
          stroke-width="3"
          stroke-linecap="round"
          stroke-linejoin="round"
          fill="none"
        />
        <rect
          v-else-if="annotation.tool === 'rect' && annotation.points.length >= 2"
          :x="Math.min(annotation.points[0].x, annotation.points[1].x)"
          :y="Math.min(annotation.points[0].y, annotation.points[1].y)"
          :width="Math.abs(annotation.points[1].x - annotation.points[0].x)"
          :height="Math.abs(annotation.points[1].y - annotation.points[0].y)"
          :stroke="annotation.color"
          stroke-width="3"
          fill="none"
        />
        <ellipse
          v-else-if="annotation.tool === 'ellipse' && annotation.points.length >= 2"
          :cx="(annotation.points[0].x + annotation.points[1].x) / 2"
          :cy="(annotation.points[0].y + annotation.points[1].y) / 2"
          :rx="Math.abs(annotation.points[1].x - annotation.points[0].x) / 2"
          :ry="Math.abs(annotation.points[1].y - annotation.points[0].y) / 2"
          :stroke="annotation.color"
          stroke-width="3"
          fill="none"
        />
      </g>
    </svg>

    <div
      v-if="rect"
      class="selection-hit-area"
      :style="{
        left: rect.x + 'px',
        top: rect.y + 'px',
        width: rect.w + 'px',
        height: rect.h + 'px',
      }"
      @mousedown="onSelectionMouseDown"
    />

    <div
      v-for="handle in handles"
      :key="handle.name"
      class="resize-handle"
      :class="`handle-${handle.name}`"
      :style="{ left: handle.x + 'px', top: handle.y + 'px' }"
      @mousedown.stop="
        (event) => {
          dragMode = 'resizing'
          resizeHandle = handle.name
          dragAnchor = pointFromEvent(event)
          startRect = rect ? { ...rect } : null
        }
      "
    />

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

    <div
      v-if="leftToolbarPos && rect && rect.w >= 4 && rect.h >= 4"
      class="toolbar mark-toolbar"
      :style="{ left: leftToolbarPos.x + 'px', top: leftToolbarPos.y + 'px' }"
      @mousedown.stop
    >
      <button
        class="icon-btn"
        :class="{ active: activeTool === 'pen' }"
        title="划线标记"
        @click="selectTool('pen')"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d="M4 20c4-1 6-3 8-7l5-9 3 2-5 9c-2 4-5 6-9 7z" />
          <path d="M14 5l5 3" />
        </svg>
      </button>
      <button
        class="icon-btn"
        :class="{ active: activeTool === 'rect' }"
        title="矩形标记"
        @click="selectTool('rect')"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="5" y="6" width="14" height="12" rx="1.5" />
        </svg>
      </button>
      <button
        class="icon-btn"
        :class="{ active: activeTool === 'ellipse' }"
        title="圆形标记"
        @click="selectTool('ellipse')"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="7" />
        </svg>
      </button>
      <div class="color-wrap">
        <button
          class="icon-btn color-btn"
          title="选择标记颜色"
          @click="paletteOpen = !paletteOpen"
        >
          <span :style="{ background: activeColor }" />
        </button>
        <div v-if="paletteOpen" class="palette">
          <button
            v-for="color in colors"
            :key="color"
            class="swatch"
            :class="{ selected: color === activeColor }"
            :style="{ background: color }"
            :title="color"
            @click="chooseColor(color)"
          />
        </div>
      </div>
      <button
        class="icon-btn"
        :disabled="annotations.length === 0"
        title="撤销上一处标记"
        @click="undoAnnotation"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d="M9 7H4v5" />
          <path d="M4 7c3-3 8-4 12-1 4 3 4 9 0 12-2 1-4 2-7 1" />
        </svg>
      </button>
    </div>

    <div
      v-if="rightToolbarPos && rect && rect.w >= 4 && rect.h >= 4"
      class="toolbar action-toolbar"
      :style="{ left: rightToolbarPos.x + 'px', top: rightToolbarPos.y + 'px' }"
      @mousedown.stop
    >
      <button class="btn cancel" @click="onCancel" title="Esc">Cancel</button>
      <button class="btn secondary" @click="onSave" title="Save to folder">
        Save
      </button>
      <button class="btn primary" @click="onConfirm" title="Enter">
        Upload &amp; copy
      </button>
    </div>

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

.selection-hit-area {
  position: absolute;
  cursor: crosshair;
}

.resize-handle {
  position: absolute;
  width: 10px;
  height: 10px;
  transform: translate(-50%, -50%);
  border: 1px solid #fff;
  background: #3b82f6;
  box-shadow: 0 1px 5px rgba(0, 0, 0, 0.35);
  z-index: 3;
}
.handle-n,
.handle-s {
  cursor: ns-resize;
}
.handle-e,
.handle-w {
  cursor: ew-resize;
}
.handle-nw,
.handle-se {
  cursor: nwse-resize;
}
.handle-ne,
.handle-sw {
  cursor: nesw-resize;
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
}

.toolbar {
  position: absolute;
  display: flex;
  gap: 6px;
  align-items: center;
  height: 32px;
  padding: 4px;
  background: rgba(28, 28, 32, 0.94);
  border-radius: 8px;
  box-shadow: 0 6px 22px rgba(0, 0, 0, 0.4);
  cursor: default;
  z-index: 4;
}

.mark-toolbar {
  width: 190px;
}
.action-toolbar {
  width: 304px;
}

.btn,
.icon-btn,
.swatch {
  font-family: inherit;
}

.btn {
  border: 0;
  border-radius: 5px;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
}
.btn.cancel {
  background: transparent;
  color: #d1d5db;
}
.btn.cancel:hover {
  background: rgba(255, 255, 255, 0.08);
}
.btn.secondary {
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
}
.btn.secondary:hover {
  background: rgba(255, 255, 255, 0.18);
}
.btn.primary {
  background: #3b82f6;
  color: #fff;
}
.btn.primary:hover {
  background: #2563eb;
}

.icon-btn {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 5px;
  color: #d1d5db;
  background: transparent;
  cursor: pointer;
}
.icon-btn:hover:not(:disabled),
.icon-btn.active {
  color: #fff;
  background: rgba(255, 255, 255, 0.12);
}
.icon-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.icon-btn svg {
  width: 18px;
  height: 18px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.color-wrap {
  position: relative;
}
.color-btn span {
  width: 16px;
  height: 16px;
  border: 1px solid rgba(255, 255, 255, 0.7);
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.18);
}
.palette {
  position: absolute;
  left: 0;
  bottom: 36px;
  display: grid;
  grid-template-columns: repeat(5, 22px);
  gap: 6px;
  padding: 8px;
  background: rgba(28, 28, 32, 0.96);
  border-radius: 8px;
  box-shadow: 0 8px 26px rgba(0, 0, 0, 0.45);
}
.swatch {
  width: 22px;
  height: 22px;
  border: 1px solid rgba(255, 255, 255, 0.55);
  border-radius: 4px;
  cursor: pointer;
}
.swatch.selected {
  outline: 2px solid #fff;
  outline-offset: 2px;
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
}
</style>
