// Package ssh provides an SCP-based remote upload adapter for SnapGo.
//
// Design rationale:
//   - We deliberately implement SCP "sink mode" by hand on top of an SSH
//     session instead of pulling in an extra dependency. The protocol is
//     trivial (one control line + payload + null byte) and we only need
//     write support, so a custom implementation keeps the dependency
//     surface small and auditable.
//   - The adapter is split into a Client (connection lifetime) and a
//     CopyFile method (single transfer) so future use-cases such as listing
//     or deleting remote files can extend the same SSH session.
//   - All public methods accept a context so the application layer can
//     enforce a timeout that matches the user-perceived capture latency.
package ssh

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/mmmy/snapgo/internal/domain"
)

// logger returns a component-scoped slog logger bound to the CURRENT
// default handler.
//
// 为什么是函数而非包级 slog.With 变量:
//   - 包级变量在 main() 调用 logging.Init() 之前就初始化, 那一刻
//     slog.Default() 还是 bootstrap handler (它转发给标准 log 包).
//   - logging.Init() 里的 slog.SetDefault() 会把标准 log 包重定向回新的
//     TextHandler, 于是旧 handler 先把整行格式化成 "INFO msg component=ssh..."
//     再被新 handler 当成一个 msg 二次包裹, 产生双前缀日志.
//   - 每次惰性读取 slog.Default() 即可始终拿到正确的目标 handler.
func sshLog() *slog.Logger { return slog.Default().With("component", "ssh") }

// Client is a thin wrapper around *ssh.Client whose lifetime maps 1:1
// to a single upload operation. We do not pool connections because the
// expected cadence (a few uploads per minute at most) does not justify
// the additional complexity of managing keep-alives.
type Client struct {
	client *ssh.Client
	cfg    domain.SSHConfig
}

// Dial establishes an SSH connection using the supplied configuration.
//
// Authentication priority:
//  1. Password, if non-empty.
//  2. Local ssh-agent ($SSH_AUTH_SOCK), if present.
//  3. ~/.ssh/id_ed25519 → id_rsa fallback files.
//
// Host-key verification follows cfg.StrictHostKey. When strict, the user-
// supplied known_hosts file is honoured (defaulting to ~/.ssh/known_hosts).
// When false the call uses ssh.InsecureIgnoreHostKey, which is a deliberate
// trade-off that surfaces in the UI — the settings page warns the user.
func Dial(ctx context.Context, cfg domain.SSHConfig) (*Client, error) {
	if cfg.Host == "" || cfg.User == "" {
		return nil, fmt.Errorf("ssh: host and user are required")
	}
	port := cfg.Port
	if port <= 0 {
		port = 22
	}
	timeout := time.Duration(cfg.ConnectTimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	authMethods, authSummary, err := buildAuthMethods(cfg)
	if err != nil {
		return nil, err
	}
	if len(authMethods) == 0 {
		sshLog().Warn("dial aborted: no auth methods",
			"host", cfg.Host, "user", cfg.User, "auth_summary", authSummary)
		return nil, fmt.Errorf("ssh: no authentication methods available (set password or ensure ssh-agent / ~/.ssh/id_* exists)")
	}

	hostKeyCallback, hostKeyMode, err := buildHostKeyCallback(cfg)
	if err != nil {
		sshLog().Error("host key callback failed",
			"host", cfg.Host, "strict", cfg.StrictHostKey,
			"known_hosts", cfg.KnownHostsPath, "err", err)
		return nil, err
	}

	clientCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", port))

	sshLog().Info("dial start",
		"addr", addr, "user", cfg.User,
		"timeout", timeout, "auth_summary", authSummary, "host_key", hostKeyMode)

	dialStart := time.Now()
	// Honour ctx by dialing through net.Dialer so an early ctx cancel
	// surfaces here instead of after the (long) ssh handshake.
	dialer := net.Dialer{Timeout: timeout}
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		sshLog().Error("tcp dial failed",
			"addr", addr, "elapsed", time.Since(dialStart), "err", err)
		return nil, fmt.Errorf("ssh: dial %s: %w", addr, err)
	}
	sshLog().Debug("tcp connected", "addr", addr, "elapsed", time.Since(dialStart))

	hsStart := time.Now()
	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
	if err != nil {
		_ = tcpConn.Close()
		sshLog().Error("ssh handshake failed",
			"addr", addr, "user", cfg.User,
			"elapsed", time.Since(hsStart), "err", err)
		return nil, fmt.Errorf("ssh: handshake: %w", err)
	}
	sshLog().Info("ssh handshake ok",
		"addr", addr, "user", cfg.User,
		"server_version", string(sshConn.ServerVersion()),
		"elapsed", time.Since(hsStart))
	return &Client{
		client: ssh.NewClient(sshConn, chans, reqs),
		cfg:    cfg,
	}, nil
}

// Close shuts down the underlying SSH connection.
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// CopyFile uploads `data` to remoteRelativePath under the user's home
// directory. The caller passes a path *relative to $HOME*; the adapter
// strips any leading "/" or "~" so users cannot escape their home
// directory through misconfigured PathPrefix values.
//
// The upload uses the SCP "sink mode" protocol:
//
//	$ scp -t <remote-dir>
//	  ← we send: D0755 0 <subdir>\n      (mkdir -p analogue, repeated)
//	  ← we send: C0644 <size> <basename>\n
//	  ← we send: <bytes...>\0
//	  ← we send: E\n                     (close each directory)
//
// Each line we write is acknowledged by a single 0x00 byte from the remote
// `scp -t` process; a non-zero ack indicates an error.
func (c *Client) CopyFile(ctx context.Context, remoteRelativePath string, data []byte, mode os.FileMode) error {
	cleaned := normaliseRemotePath(remoteRelativePath)
	if cleaned == "" {
		return fmt.Errorf("ssh: remote path is empty")
	}

	dir, base := path.Split(cleaned)
	dir = strings.Trim(dir, "/")

	sshLog().Info("scp copy start",
		"remote_path", cleaned, "dir", dir, "file", base,
		"size", len(data), "mode", fmt.Sprintf("%#o", mode.Perm()))

	session, err := c.client.NewSession()
	if err != nil {
		sshLog().Error("scp new session failed", "err", err)
		return fmt.Errorf("ssh: new session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("ssh: stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ssh: stdout pipe: %w", err)
	}

	// `-t` puts the remote scp into "sink mode" rooted at $HOME. `-r`
	// allows us to feed `D` directives so we can create directory trees
	// in one round-trip rather than running a separate `mkdir -p`.
	cmd := fmt.Sprintf("scp -tr %s", shellQuote("./"))
	sshLog().Debug("scp remote command", "cmd", cmd)
	if err := session.Start(cmd); err != nil {
		sshLog().Error("scp session start failed", "cmd", cmd, "err", err)
		return fmt.Errorf("ssh: start scp: %w", err)
	}

	transferStart := time.Now()
	errCh := make(chan error, 1)
	go func() {
		errCh <- writeSCPStream(stdin, stdout, dir, base, data, mode)
	}()

	// Wait for either ctx, the writer goroutine, or the remote command.
	select {
	case <-ctx.Done():
		sshLog().Warn("scp cancelled by context",
			"remote_path", cleaned, "elapsed", time.Since(transferStart), "err", ctx.Err())
		_ = session.Signal(ssh.SIGTERM)
		_ = session.Close()
		return ctx.Err()
	case writeErr := <-errCh:
		if writeErr != nil {
			sshLog().Error("scp protocol failed",
				"remote_path", cleaned, "elapsed", time.Since(transferStart), "err", writeErr)
			_ = session.Close()
			return writeErr
		}
	}

	if err := session.Wait(); err != nil {
		sshLog().Error("scp session wait failed",
			"remote_path", cleaned, "elapsed", time.Since(transferStart), "err", err)
		return fmt.Errorf("ssh: scp wait: %w", err)
	}
	sshLog().Info("scp copy ok",
		"remote_path", cleaned, "size", len(data), "elapsed", time.Since(transferStart))
	return nil
}

// writeSCPStream drives the SCP sink-mode dialogue described above. It is
// extracted from CopyFile for two reasons:
//   1. It contains the only blocking IO so we can run it in a goroutine
//      and select on ctx.Done().
//   2. It makes the protocol-level steps testable in isolation (the unit
//      tests pipe an in-memory bytes.Buffer into expectAck).
func writeSCPStream(stdin io.WriteCloser, stdout io.Reader, dir, base string, data []byte, mode os.FileMode) error {
	defer stdin.Close()

	// First ack: the remote scp -t emits an initial 0x00 once it is ready.
	if err := expectAck(stdout); err != nil {
		return fmt.Errorf("scp: initial ack: %w", err)
	}
	sshLog().Debug("scp initial ack received")

	// Walk `dir` segment by segment, opening each level with `D0755 0 <name>`.
	dirs := splitNonEmpty(dir, "/")
	for _, segment := range dirs {
		line := fmt.Sprintf("D0755 0 %s\n", segment)
		if _, err := io.WriteString(stdin, line); err != nil {
			return fmt.Errorf("scp: write D-line: %w", err)
		}
		if err := expectAck(stdout); err != nil {
			return fmt.Errorf("scp: ack D-line %q: %w", segment, err)
		}
		sshLog().Debug("scp dir opened", "segment", segment)
	}

	// File header: `C<perm> <size> <name>\n`
	header := fmt.Sprintf("C%04o %d %s\n", mode.Perm(), len(data), base)
	if _, err := io.WriteString(stdin, header); err != nil {
		return fmt.Errorf("scp: write C-line: %w", err)
	}
	if err := expectAck(stdout); err != nil {
		return fmt.Errorf("scp: ack C-line: %w", err)
	}
	sshLog().Debug("scp header acked", "header", strings.TrimSpace(header))

	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("scp: write payload: %w", err)
	}
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("scp: write trailing null: %w", err)
	}
	if err := expectAck(stdout); err != nil {
		return fmt.Errorf("scp: ack payload: %w", err)
	}
	sshLog().Debug("scp payload acked", "bytes", len(data))

	// Pop each directory we opened earlier with an `E` line so the remote
	// scp finishes cleanly.
	for range dirs {
		if _, err := io.WriteString(stdin, "E\n"); err != nil {
			return fmt.Errorf("scp: write E-line: %w", err)
		}
		if err := expectAck(stdout); err != nil {
			return fmt.Errorf("scp: ack E-line: %w", err)
		}
	}
	return nil
}

// expectAck reads exactly one byte from the remote scp and treats anything
// other than 0x00 as a protocol-level error. When the remote signals an
// error (0x01 = warning, 0x02 = fatal) it is followed by a textual reason
// terminated with '\n', which we surface to the caller.
func expectAck(r io.Reader) error {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	if buf[0] == 0 {
		return nil
	}
	// Read the rest of the message until newline so the user sees a
	// meaningful "permission denied"-style reason.
	msg := readUntilNewline(r)
	return fmt.Errorf("scp remote error (%d): %s", buf[0], strings.TrimSpace(msg))
}

func readUntilNewline(r io.Reader) string {
	var sb strings.Builder
	buf := make([]byte, 1)
	for i := 0; i < 1024; i++ {
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		if buf[0] == '\n' {
			break
		}
		sb.WriteByte(buf[0])
	}
	return sb.String()
}

// TestConnection performs a minimal handshake + `pwd` round trip so the
// settings UI can confirm the credentials work without writing a file.
func TestConnection(ctx context.Context, cfg domain.SSHConfig) error {
	sshLog().Info("test connection start", "host", cfg.Host, "user", cfg.User, "port", cfg.Port)
	start := time.Now()
	client, err := Dial(ctx, cfg)
	if err != nil {
		sshLog().Error("test connection: dial failed", "err", err, "elapsed", time.Since(start))
		return err
	}
	defer client.Close()
	session, err := client.client.NewSession()
	if err != nil {
		sshLog().Error("test connection: new session failed", "err", err)
		return fmt.Errorf("ssh: new session: %w", err)
	}
	defer session.Close()
	if err := session.Run("true"); err != nil {
		sshLog().Error("test connection: probe failed", "err", err)
		return fmt.Errorf("ssh: probe command failed: %w", err)
	}
	sshLog().Info("test connection ok", "elapsed", time.Since(start))
	return nil
}

// normaliseRemotePath strips any leading "/" or "~" so the path is always
// interpreted relative to the remote $HOME, matching the UI promise.
func normaliseRemotePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "~")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "./")
	return path.Clean(p)
}

// splitNonEmpty splits `s` on `sep` and discards empty segments so the
// caller does not have to defend against double slashes etc.
func splitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

// shellQuote is a defensive single-quote escaper used when interpolating
// user-controlled strings (currently only the relative root ".") into the
// remote `scp` command line. We do NOT quote arbitrary user paths because
// SCP itself receives them through the protocol channel above.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildAuthMethods chooses authentication methods given the supplied
// configuration. See Dial's docstring for the priority order.
//
// Returns a human-readable summary alongside the method slice so the
// caller can include "password+agent+ed25519" (and similar) in its dial
// log without leaking secret material.
func buildAuthMethods(cfg domain.SSHConfig) ([]ssh.AuthMethod, string, error) {
	var methods []ssh.AuthMethod
	var sources []string
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
		sources = append(sources, "password")
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			// 关键: 仅在 agent 真正持有 key 时才把它加入 auth methods.
			//
			// macOS 的 launchd ssh-agent 即便没有任何 identity 也会响应,
			// 此时若仍注册 PublicKeysCallback, x/crypto 会把它当作一次空的
			// publickey 尝试. 配合服务器的 MaxAuthTries 计数, 这次"空尝试"
			// 会挤占后续基于磁盘私钥的认证机会, 最终导致
			// "[none publickey] no supported methods remain" —— 即便磁盘上
			// 的 key 本身完全可用. 因此空 agent 必须跳过.
			ag := agent.NewClient(conn)
			if keys, lerr := ag.List(); lerr == nil && len(keys) > 0 {
				methods = append(methods, ssh.PublicKeysCallback(ag.Signers))
				sources = append(sources, fmt.Sprintf("agent(%d)", len(keys)))
			} else {
				sshLog().Debug("ssh-agent has no identities; skipping",
					"sock", sock, "list_err", lerr)
			}
		} else {
			sshLog().Debug("ssh-agent dial failed", "sock", sock, "err", err)
		}
	}
	home, err := os.UserHomeDir()
	if err == nil {
		// Probe the common default keys; any unreadable / missing file is
		// silently skipped so the user never sees noise about keys they did
		// not set up.
		for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
			signer, err := loadPrivateKey(path.Join(home, ".ssh", name))
			if err == nil && signer != nil {
				methods = append(methods, ssh.PublicKeys(signer))
				sources = append(sources, name)
			}
		}
	} else {
		sshLog().Debug("home dir unavailable for ssh keys", "err", err)
	}
	if len(sources) == 0 {
		return methods, "none", nil
	}
	return methods, strings.Join(sources, "+"), nil
}

// loadPrivateKey reads and parses a single OpenSSH private key file.
// Returns (nil, nil) when the file does not exist so the caller can keep
// scanning the standard key list.
func loadPrivateKey(p string) (ssh.Signer, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		// Encrypted keys without a passphrase are skipped silently for now;
		// passphrase support is left to a follow-up spec.
		return nil, nil
	}
	return signer, nil
}

// buildHostKeyCallback returns either a strict known_hosts-backed callback
// or an InsecureIgnoreHostKey fallback, depending on cfg.StrictHostKey.
//
// The returned mode string ("insecure" or "known_hosts:<path>") is logged
// at dial time so a "rejected by host key" failure is easy to diagnose.
func buildHostKeyCallback(cfg domain.SSHConfig) (ssh.HostKeyCallback, string, error) {
	if !cfg.StrictHostKey {
		// Deliberate trade-off: matches `scp -o StrictHostKeyChecking=no` and
		// keeps the first-launch experience friction-free for personal LANs.
		return ssh.InsecureIgnoreHostKey(), "insecure", nil
	}
	knownHosts := cfg.KnownHostsPath
	if knownHosts == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", fmt.Errorf("ssh: resolve home for known_hosts: %w", err)
		}
		knownHosts = path.Join(home, ".ssh", "known_hosts")
	}
	cb, err := knownhosts.New(knownHosts)
	if err != nil {
		return nil, "", fmt.Errorf("ssh: load known_hosts %q: %w", knownHosts, err)
	}
	return cb, "known_hosts:" + knownHosts, nil
}
