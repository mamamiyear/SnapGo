<script setup lang="ts">
/*
 * SettingsView — the home screen of SnapGo before the user triggers a
 * capture. It exposes:
 *   1. Hotkey configuration
 *   2. S3-compatible OSS configuration
 *   3. Manual capture button (useful when developing or when the global
 *      hotkey conflicts with another app)
 *
 * State flow: load() pulls config from Go on mount, save() pushes back.
 */
import { onMounted, ref } from 'vue'
import {
  GetConfig,
  SaveConfig,
  TestConnection,
  CaptureNow,
} from '../../wailsjs/go/main/App'

interface S3Config {
  endpoint: string
  region: string
  bucket: string
  accessKeyId: string
  secretAccessKey: string
  pathPrefix: string
  publicUrlBase: string
  usePathStyle: boolean
}

interface AppConfig {
  hotkey: string
  s3: S3Config
}

const config = ref<AppConfig>({
  hotkey: 'cmd+shift+a',
  s3: {
    endpoint: '',
    region: 'us-east-1',
    bucket: '',
    accessKeyId: '',
    secretAccessKey: '',
    pathPrefix: 'snapgo/',
    publicUrlBase: '',
    usePathStyle: true,
  },
})

const saving = ref(false)
const testing = ref(false)
const message = ref<{ kind: 'ok' | 'err'; text: string } | null>(null)

function flash(kind: 'ok' | 'err', text: string) {
  message.value = { kind, text }
  window.setTimeout(() => {
    if (message.value?.text === text) message.value = null
  }, 4000)
}

async function load() {
  const cfg = await GetConfig()
  if (cfg) {
    config.value = cfg as unknown as AppConfig
  }
}

async function save() {
  saving.value = true
  try {
    await SaveConfig(config.value as any)
    flash('ok', 'Settings saved')
  } catch (err: any) {
    flash('err', `Save failed: ${err?.message ?? err}`)
  } finally {
    saving.value = false
  }
}

async function test() {
  testing.value = true
  try {
    await TestConnection(config.value.s3 as any)
    flash('ok', 'S3 connection OK — bucket is writable')
  } catch (err: any) {
    flash('err', `Test failed: ${err?.message ?? err}`)
  } finally {
    testing.value = false
  }
}

async function manualCapture() {
  await CaptureNow()
}

onMounted(load)
</script>

<template>
  <div class="settings">
    <header class="hero">
      <div>
        <h1>SnapGo</h1>
        <p class="subtitle">
          Press <kbd>{{ config.hotkey }}</kbd> anywhere to capture, upload, and
          copy the URL to your clipboard.
        </p>
      </div>
      <button class="btn primary" @click="manualCapture">Capture now</button>
    </header>

    <section class="card">
      <h2>Shortcut</h2>
      <label class="field">
        <span>Global hotkey</span>
        <input
          v-model="config.hotkey"
          placeholder="cmd+shift+a"
          spellcheck="false"
        />
      </label>
      <p class="hint">
        Tokens (case-insensitive, "+"-separated): cmd / ctrl / option / shift +
        a–z or 0–9. Example: <code>cmd+shift+a</code>.
      </p>
    </section>

    <section class="card">
      <h2>S3-compatible storage</h2>
      <div class="grid">
        <label class="field">
          <span>Endpoint *</span>
          <input
            v-model="config.s3.endpoint"
            placeholder="https://s3.us-east-005.backblazeb2.com"
          />
        </label>
        <label class="field">
          <span>Region</span>
          <input v-model="config.s3.region" placeholder="us-east-1 / auto" />
        </label>
        <label class="field">
          <span>Bucket *</span>
          <input v-model="config.s3.bucket" placeholder="my-screenshots" />
        </label>
        <label class="field">
          <span>Access Key ID *</span>
          <input v-model="config.s3.accessKeyId" />
        </label>
        <label class="field">
          <span>Secret Access Key *</span>
          <input v-model="config.s3.secretAccessKey" type="password" />
        </label>
        <label class="field">
          <span>Path prefix</span>
          <input v-model="config.s3.pathPrefix" placeholder="snapgo/" />
        </label>
        <label class="field full">
          <span>Public URL base (optional, e.g. CDN)</span>
          <input
            v-model="config.s3.publicUrlBase"
            placeholder="https://cdn.example.com/screenshots"
          />
        </label>
        <label class="field checkbox">
          <input type="checkbox" v-model="config.s3.usePathStyle" />
          <span
            >Use path-style addressing (recommended for MinIO, R2, custom
            domains)</span
          >
        </label>
      </div>

      <div class="actions">
        <button class="btn" :disabled="testing" @click="test">
          {{ testing ? 'Testing…' : 'Test connection' }}
        </button>
        <button class="btn primary" :disabled="saving" @click="save">
          {{ saving ? 'Saving…' : 'Save' }}
        </button>
      </div>
    </section>

    <transition name="fade">
      <div
        v-if="message"
        class="banner"
        :class="message.kind === 'ok' ? 'ok' : 'err'"
      >
        {{ message.text }}
      </div>
    </transition>
  </div>
</template>

<style scoped>
.settings {
  max-width: 760px;
  margin: 0 auto;
  padding: 28px 32px 60px;
  color: #1f2937;
}
.hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
}
.hero h1 {
  margin: 0 0 6px;
  font-size: 24px;
  letter-spacing: -0.01em;
}
.subtitle {
  margin: 0;
  color: #6b7280;
  font-size: 13px;
}
kbd {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  background: #f3f4f6;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 12px;
}
.card {
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 18px 20px;
  margin-bottom: 18px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.card h2 {
  margin: 0 0 12px;
  font-size: 14px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #6b7280;
}
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 16px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 13px;
}
.field.full {
  grid-column: 1 / -1;
}
.field span {
  color: #374151;
  font-weight: 500;
}
.field input[type='text'],
.field input:not([type='checkbox']) {
  border: 1px solid #d1d5db;
  border-radius: 6px;
  padding: 7px 10px;
  font-size: 13px;
  background: #fff;
  color: inherit;
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.field input:focus {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.18);
}
.field.checkbox {
  flex-direction: row;
  align-items: center;
  gap: 8px;
  grid-column: 1 / -1;
}
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: #6b7280;
}
.hint code {
  background: #f3f4f6;
  padding: 1px 6px;
  border-radius: 4px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}
.btn {
  border: 1px solid #d1d5db;
  background: #fff;
  border-radius: 6px;
  padding: 7px 14px;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s, color 0.15s;
}
.btn:hover:not(:disabled) {
  border-color: #9ca3af;
  background: #f9fafb;
}
.btn.primary {
  background: #3b82f6;
  border-color: #3b82f6;
  color: #fff;
}
.btn.primary:hover:not(:disabled) {
  background: #2563eb;
  border-color: #2563eb;
}
.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.banner {
  position: fixed;
  bottom: 18px;
  left: 50%;
  transform: translateX(-50%);
  padding: 8px 14px;
  border-radius: 6px;
  font-size: 13px;
  box-shadow: 0 6px 24px rgba(15, 23, 42, 0.18);
}
.banner.ok {
  background: #10b981;
  color: #fff;
}
.banner.err {
  background: #ef4444;
  color: #fff;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
@media (prefers-color-scheme: dark) {
  .settings {
    color: #e5e7eb;
  }
  .card {
    background: #1f2937;
    border-color: #374151;
  }
  .field input {
    background: #111827;
    border-color: #374151;
    color: #e5e7eb;
  }
  .btn {
    background: #1f2937;
    border-color: #374151;
    color: #e5e7eb;
  }
  .btn:hover:not(:disabled) {
    background: #374151;
  }
  .subtitle {
    color: #9ca3af;
  }
}
</style>
