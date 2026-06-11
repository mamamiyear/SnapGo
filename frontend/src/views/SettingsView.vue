<script setup lang="ts">
/*
 * SettingsView — the home screen of SnapGo before the user triggers a
 * capture. The active configuration section is selected by the App-shell
 * sidebar and passed in via the `tab` prop, so this view only owns the
 * card content for the currently selected destination type:
 *
 *   • "general" — the global hotkey / capture parameters.
 *   • "s3"      — S3-compatible object-storage credentials.
 *   • "ssh"     — SSH/SCP destination for the "save remote" button.
 *
 * State flow: load() pulls config from Go on mount, save() pushes back.
 */
import { computed, onMounted, ref } from 'vue'
import {
  GetConfig,
  SaveConfig,
  TestConnection,
  TestSSHConnection,
  CaptureNow,
} from '../../wailsjs/go/main/App'

// Tab discriminator shared with the App shell. Kept as a string union so
// the parent can pass the value without importing a type from this view.
type TabId = 'general' | 's3' | 'ssh'

const props = defineProps<{
  tab: TabId
}>()

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

interface SSHConfig {
  host: string
  port: number
  user: string
  password: string
  pathPrefix: string
  strictHostKey: boolean
  knownHostsPath: string
  connectTimeoutSecs: number
}

interface AppConfig {
  hotkey: string
  s3: S3Config
  ssh: SSHConfig
}

// `defaultConfig` keeps the initial render in sync with the Go-side
// DefaultAppConfig so the UI never flashes empty fields before load()
// completes. Centralising the defaults also avoids subtle drift between
// frontend and backend.
function defaultConfig(): AppConfig {
  return {
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
    ssh: {
      host: '',
      port: 22,
      user: '',
      password: '',
      pathPrefix: 'snapgo/',
      strictHostKey: false,
      knownHostsPath: '',
      connectTimeoutSecs: 10,
    },
  }
}

const config = ref<AppConfig>(defaultConfig())

const saving = ref(false)
const testing = ref(false)
const testingSSH = ref(false)
const message = ref<{ kind: 'ok' | 'err'; text: string } | null>(null)

// The "remote path" in the UI is shown as "~/<pathPrefix>" so the user
// understands that everything is rooted at the remote home directory.
// We strip leading slashes / tilde here defensively in case someone pastes
// a full path.
const sshPathDisplay = computed({
  get: () => config.value.ssh.pathPrefix,
  set: (raw: string) => {
    let cleaned = raw.trim()
    cleaned = cleaned.replace(/^~/, '')
    cleaned = cleaned.replace(/^\/+/, '')
    config.value.ssh.pathPrefix = cleaned
  },
})

function flash(kind: 'ok' | 'err', text: string) {
  message.value = { kind, text }
  window.setTimeout(() => {
    if (message.value?.text === text) message.value = null
  }, 4000)
}

async function load() {
  const cfg = await GetConfig()
  if (cfg) {
    // Merge with defaults so a config file written before SSH support
    // existed does not leave ssh fields undefined.
    const merged = defaultConfig()
    Object.assign(merged, cfg)
    merged.s3 = { ...merged.s3, ...(cfg as any).s3 }
    merged.ssh = { ...merged.ssh, ...(cfg as any).ssh }
    config.value = merged
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

async function testSSH() {
  testingSSH.value = true
  try {
    await TestSSHConnection(config.value.ssh as any)
    flash('ok', 'SSH connection OK')
  } catch (err: any) {
    flash('err', `SSH test failed: ${err?.message ?? err}`)
  } finally {
    testingSSH.value = false
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

    <!-- General tab — only the shortcut configuration lives here. -->
    <section v-if="props.tab === 'general'" class="card">
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
      <div class="actions">
        <button class="btn primary" :disabled="saving" @click="save">
          {{ saving ? 'Saving…' : 'Save' }}
        </button>
      </div>
    </section>

    <!-- S3-Conf tab — unchanged content, moved out of General. -->
    <section v-if="props.tab === 's3'" class="card">
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

    <!-- SSH-Conf tab — destination for the save-remote action. -->
    <section v-if="props.tab === 'ssh'" class="card">
      <h2>SSH / SCP destination</h2>
      <div class="grid">
        <label class="field">
          <span>Host (IP or domain) *</span>
          <input v-model="config.ssh.host" placeholder="192.168.1.10" />
        </label>
        <label class="field">
          <span>Port</span>
          <input
            v-model.number="config.ssh.port"
            type="number"
            min="1"
            max="65535"
            placeholder="22"
          />
        </label>
        <label class="field">
          <span>User *</span>
          <input v-model="config.ssh.user" placeholder="ubuntu" />
        </label>
        <label class="field">
          <span>Password (optional)</span>
          <input v-model="config.ssh.password" type="password" />
        </label>
        <label class="field full">
          <span>Remote path (under home)</span>
          <div class="prefixed-input">
            <span class="input-prefix">~/</span>
            <input
              v-model="sshPathDisplay"
              placeholder="snapgo/"
              spellcheck="false"
            />
          </div>
        </label>
        <label class="field">
          <span>Connect timeout (sec)</span>
          <input
            v-model.number="config.ssh.connectTimeoutSecs"
            type="number"
            min="1"
            max="120"
            placeholder="10"
          />
        </label>
        <label class="field">
          <span>Known hosts path</span>
          <input
            v-model="config.ssh.knownHostsPath"
            placeholder="~/.ssh/known_hosts"
            :disabled="!config.ssh.strictHostKey"
          />
        </label>
        <label class="field checkbox">
          <input type="checkbox" v-model="config.ssh.strictHostKey" />
          <span>
            Strict host key checking (recommended for production hosts)
          </span>
        </label>
      </div>
      <p v-if="!config.ssh.password" class="hint warn">
        Password is empty — please make sure password-less SSH is configured on
        this machine (e.g. via <code>ssh-copy-id</code> or your <code>ssh-agent</code>).
      </p>

      <div class="actions">
        <button class="btn" :disabled="testingSSH" @click="testSSH">
          {{ testingSSH ? 'Testing…' : 'Test connection' }}
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
  margin-bottom: 16px;
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

/* Tab strip removed — sidebar in App.vue now drives section selection. */

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
.field input[type='number'],
.field input:not([type='checkbox']) {
  border: 1px solid #d1d5db;
  border-radius: 6px;
  padding: 7px 10px;
  font-size: 13px;
  background: #fff;
  color: inherit;
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
  width: 100%;
  box-sizing: border-box;
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
.field input:disabled {
  background: #f3f4f6;
  color: #9ca3af;
  cursor: not-allowed;
}
.prefixed-input {
  display: flex;
  align-items: stretch;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: #fff;
  overflow: hidden;
}
.prefixed-input:focus-within {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.18);
}
.input-prefix {
  padding: 7px 10px;
  background: #f3f4f6;
  color: #4b5563;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  border-right: 1px solid #d1d5db;
}
.prefixed-input input {
  border: 0 !important;
  border-radius: 0 !important;
  flex: 1;
  padding: 7px 10px;
}
.prefixed-input input:focus {
  box-shadow: none !important;
}
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: #6b7280;
}
.hint.warn {
  color: #b45309;
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
  .field input:disabled {
    background: #1f2937;
    color: #6b7280;
  }
  .prefixed-input {
    background: #111827;
    border-color: #374151;
  }
  .input-prefix {
    background: #1f2937;
    border-right-color: #374151;
    color: #d1d5db;
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
