package webai

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// BrowserLaunchConfig describes one visible Chrome session. The profile is a
// dedicated persistent browser profile owned by ainovel, never project data.
type BrowserLaunchConfig struct {
	Executable string
	ProfileDir string
	StartURL   string
	ExtraArgs  []string
}

// BrowserProcess is the process lifecycle needed by SessionManager.
type BrowserProcess interface {
	PID() int
	Done() <-chan error
	Stop() error
}

// BrowserLauncher lets W2 tests prove lifecycle behavior without launching a
// real desktop browser in CI.
type BrowserLauncher interface {
	Launch(ctx context.Context, cfg BrowserLaunchConfig) (BrowserProcess, error)
}

// ExecBrowserLauncher launches a visible local Chrome process.
type ExecBrowserLauncher struct{}

func (ExecBrowserLauncher) Launch(ctx context.Context, cfg BrowserLaunchConfig) (BrowserProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Executable) == "" {
		return nil, fmt.Errorf("webai: browser executable is required")
	}
	if strings.TrimSpace(cfg.ProfileDir) == "" {
		return nil, fmt.Errorf("webai: browser profile directory is required")
	}
	if err := os.MkdirAll(cfg.ProfileDir, 0o700); err != nil {
		return nil, fmt.Errorf("webai: create browser profile: %w", err)
	}

	args := []string{
		"--user-data-dir=" + cfg.ProfileDir,
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
		"--new-window",
	}
	args = append(args, cfg.ExtraArgs...)
	if strings.TrimSpace(cfg.StartURL) != "" {
		args = append(args, cfg.StartURL)
	}

	cmd := exec.Command(cfg.Executable, args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("webai: launch Chrome: %w", err)
	}
	p := &execBrowserProcess{cmd: cmd, done: make(chan error, 1)}
	go func() {
		p.done <- cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

type execBrowserProcess struct {
	cmd      *exec.Cmd
	done     chan error
	stopOnce sync.Once
	stopErr  error
}

func (p *execBrowserProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *execBrowserProcess) Done() <-chan error {
	if p == nil {
		ch := make(chan error)
		close(ch)
		return ch
	}
	return p.done
}

func (p *execBrowserProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.stopOnce.Do(func() {
		p.stopErr = p.cmd.Process.Kill()
	})
	return p.stopErr
}

// ResolveChromeExecutable resolves an explicit path/name first, then common
// Chrome installations for the current OS. It never silently falls back to an
// AI API or a headless cloud browser.
func ResolveChromeExecutable(override string) (string, error) {
	if value := strings.TrimSpace(override); value != "" {
		return resolveExecutable(value)
	}
	for _, candidate := range chromeCandidates() {
		if candidate == "" {
			continue
		}
		if path, err := resolveExecutable(candidate); err == nil {
			return path, nil
		}
	}
	return "", &Error{Kind: ErrorTransport, Op: "resolve Chrome", Cause: fmt.Errorf("Chrome executable not found")}
}

func resolveExecutable(value string) (string, error) {
	if !strings.ContainsAny(value, `/\\`) {
		if path, err := exec.LookPath(value); err == nil {
			return filepath.Clean(path), nil
		}
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	return filepath.Clean(path), nil
}

func chromeCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			"chrome.exe",
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			filepath.Join(os.Getenv("HOME"), "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
			"google-chrome",
		}
	default:
		return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
	}
}

// DefaultBrowserProfileDir returns a profile outside the novel/project tree so
// browser cookies and login storage are not accidentally committed with a book.
func DefaultBrowserProfileDir(profileName string) (string, error) {
	name := strings.TrimSpace(profileName)
	if name == "" {
		name = "default"
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("webai: invalid browser profile name %q", profileName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("webai: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".ainovel", "browser", "profiles", name), nil
}
