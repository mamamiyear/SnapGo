<script setup lang="ts">
/*
 * App shell — switches between two distinct UI modes that share the same
 * Wails window:
 *
 *   • "settings" : full settings UI (self-drawn title bar + sidebar + form).
 *                  Self-drawn because the window is now Frameless to make
 *                  the overlay paint edge-to-edge.
 *   • "overlay"  : Snipaste-style region picker that fills the whole
 *                  primary display.
 *
 * The Go side resizes/positions/flag-flips the window for each mode and
 * emits a `capture:overlay` event with the screenshot payload so the
 * frontend knows when (and what) to render.
 */
import { onMounted, onUnmounted, ref } from 'vue'
import SettingsView from './views/SettingsView.vue'
import Toast from './components/Toast.vue'
import CaptureOverlay from './views/CaptureOverlay.vue'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import {
  RetryRegisterHotkey,
  ConfirmRegion,
  CopyRegionImage,
  SaveRegionImage,
  CancelRegion,
} from '../wailsjs/go/main/App'

type HotkeyStatus =
  | { state: 'unknown' }
  | { state: 'ok'; spec: string }
  | { state: 'error'; reason: string }
const hotkeyStatus = ref<HotkeyStatus>({ state: 'unknown' })

type Mode = 'settings' | 'overlay'
const mode = ref<Mode>('settings')

interface OverlayPayload {
  cssWidth: number
  cssHeight: number
  scale: number
}
const overlayPayload = ref<OverlayPayload | null>(null)

const capturing = ref(false)

const toast = ref<{ kind: 'success' | 'error'; text: string } | null>(null)
let toastTimer: number | undefined

function showToast(kind: 'success' | 'error', text: string) {
  toast.value = { kind, text }
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => {
    toast.value = null
  }, 3000)
}

async function retryHotkey() {
  try {
    await RetryRegisterHotkey()
  } catch {
    /* Surfaced via hotkey:error */
  }
}

async function onOverlayConfirm(rect: {
  rect: {
    x: number
    y: number
    w: number
    h: number
  }
  annotations: Array<{
    tool: string
    color: string
    points: Array<{ x: number; y: number }>
  }>
}) {
  // Optimistically swap back so the window does not visually lag the
  // Go-side hide. If upload fails, the toast surfaces the reason.
  mode.value = 'settings'
  overlayPayload.value = null
  try {
    await ConfirmRegion(rect as any)
  } catch {
    /* Surfaced via upload:failure */
  }
}

async function onOverlayCopy(rect: {
  rect: {
    x: number
    y: number
    w: number
    h: number
  }
  annotations: Array<{
    tool: string
    color: string
    points: Array<{ x: number; y: number }>
  }>
}) {
  mode.value = 'settings'
  overlayPayload.value = null
  try {
    await CopyRegionImage(rect as any)
  } catch {
    /* Surfaced via upload:failure */
  }
}

async function onOverlaySave(rect: {
  rect: {
    x: number
    y: number
    w: number
    h: number
  }
  annotations: Array<{
    tool: string
    color: string
    points: Array<{ x: number; y: number }>
  }>
}) {
  mode.value = 'settings'
  overlayPayload.value = null
  try {
    await SaveRegionImage(rect as any)
  } catch {
    /* Surfaced via upload:failure */
  }
}

async function onOverlayCancel() {
  mode.value = 'settings'
  overlayPayload.value = null
  try {
    await CancelRegion()
  } catch {
    /* Best-effort */
  }
}

onMounted(() => {
  EventsOn('capture:start', () => {
    capturing.value = true
  })
  EventsOn('capture:end', () => {
    capturing.value = false
  })
  EventsOn('capture:cancelled', () => {
    capturing.value = false
  })
  EventsOn('capture:overlay', (payload: OverlayPayload) => {
    overlayPayload.value = payload
    mode.value = 'overlay'
    capturing.value = false
  })
  EventsOn('upload:success', (url: string) => {
    showToast('success', `Copied: ${url}`)
  })
  EventsOn('upload:failure', (reason: string) => {
    showToast('error', reason)
  })
  EventsOn('hotkey:ready', (spec: string) => {
    hotkeyStatus.value = { state: 'ok', spec }
  })
  EventsOn('hotkey:error', (reason: string) => {
    hotkeyStatus.value = { state: 'error', reason }
  })
})

onUnmounted(() => {
  EventsOff('capture:start')
  EventsOff('capture:end')
  EventsOff('capture:cancelled')
  EventsOff('capture:overlay')
  EventsOff('upload:success')
  EventsOff('upload:failure')
  EventsOff('hotkey:ready')
  EventsOff('hotkey:error')
})
</script>

<template>
  <!-- Overlay mode: full-screen Snipaste-style picker. -->
  <CaptureOverlay
    v-if="mode === 'overlay' && overlayPayload"
    :width="overlayPayload.cssWidth"
    :height="overlayPayload.cssHeight"
    @confirm="onOverlayConfirm"
    @copy="onOverlayCopy"
    @save="onOverlaySave"
    @cancel="onOverlayCancel"
  />

  <!-- Settings mode: standard desktop window; macOS title bar is native. -->
  <div v-else class="app-root">
    <div v-if="hotkeyStatus.state === 'error'" class="permission-banner">
      <div>
        <strong>Global hotkey is not active.</strong>
        Reason: {{ hotkeyStatus.reason }}
        <br />
        On macOS open
        <em>System Settings → Privacy &amp; Security → Accessibility</em>,
        enable <strong>SnapGo</strong>, then click "Retry".
      </div>
      <button class="btn" @click="retryHotkey">Retry</button>
    </div>

    <main class="main">
      <aside class="sidebar">
        <div class="sidebar-item active"><span class="dot" /> Settings</div>
      </aside>
      <section class="content">
        <SettingsView />
      </section>
    </main>

    <Toast v-if="toast" :kind="toast.kind" :text="toast.text" />
  </div>
</template>

<style scoped>
.app-root {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  background: #f6f7fa;
  color: #111827;
  font-size: 13px;
}

.status-pill {
  font-size: 11px;
  padding: 3px 9px;
  border-radius: 999px;
  font-weight: 500;
}
.status-pill.ok {
  color: #047857;
  background: rgba(16, 185, 129, 0.12);
}
.status-pill.err {
  color: #b91c1c;
  background: rgba(239, 68, 68, 0.14);
  cursor: help;
}
.status-pill.muted {
  color: #6b7280;
  background: rgba(107, 114, 128, 0.14);
}

.permission-banner {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin: 12px 16px 0;
  padding: 10px 14px;
  background: #fff7ed;
  border: 1px solid #fdba74;
  border-radius: 8px;
  color: #7c2d12;
  font-size: 12px;
  line-height: 1.55;
}
.permission-banner em {
  font-style: normal;
  background: rgba(124, 45, 18, 0.08);
  padding: 0 4px;
  border-radius: 3px;
}
.permission-banner .btn {
  flex: 0 0 auto;
  border: 1px solid #fdba74;
  background: #fff;
  color: #7c2d12;
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 12px;
  cursor: pointer;
}
.permission-banner .btn:hover {
  background: #fed7aa;
}

.main {
  flex: 1;
  display: grid;
  grid-template-columns: 180px 1fr;
  min-height: 0;
}
.sidebar {
  border-right: 1px solid #e5e7eb;
  padding: 14px 8px;
  background: rgba(255, 255, 255, 0.4);
}
.sidebar-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-radius: 6px;
  font-size: 13px;
  color: #374151;
  cursor: default;
}
.sidebar-item.active {
  background: rgba(59, 130, 246, 0.12);
  color: #1d4ed8;
  font-weight: 500;
}
.sidebar-item .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.content {
  overflow-y: auto;
  min-width: 0;
}

@media (prefers-color-scheme: dark) {
  .app-root {
    background: #1c1d22;
    color: #e5e7eb;
  }
  .titlebar {
    background: rgba(28, 29, 34, 0.7);
    border-bottom-color: #2c2f36;
  }
  .titlebar-title {
    color: #f3f4f6;
  }
  .sidebar {
    background: rgba(0, 0, 0, 0.15);
    border-right-color: #2c2f36;
  }
  .sidebar-item {
    color: #d1d5db;
  }
  .sidebar-item.active {
    background: rgba(59, 130, 246, 0.18);
    color: #93c5fd;
  }
  .permission-banner {
    background: rgba(120, 53, 15, 0.3);
    border-color: rgba(253, 186, 116, 0.4);
    color: #fed7aa;
  }
}
</style>
