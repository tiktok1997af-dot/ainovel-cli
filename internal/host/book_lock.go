package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

const bookLockFile = ".ainovel.lock"

// ErrBookInUse 表示同一小说目录已被另一个进程占用。
var ErrBookInUse = errors.New("小说目录已被另一个 ainovel-cli 实例占用")

// bookLease 在 Host 的完整生命周期内持有小说目录的跨进程独占权。
// 锁文件会保留在目录中；真正的占用状态由操作系统管理，进程异常退出也会自动释放。
// W5A 同时把 WEB-only browser runtime 绑定到相同生命周期：Host.Close 最终关闭
// bookLease，因此 Chrome session 也会在此处被确定性停止。
type bookLease struct {
	lock *flock.Flock
	dir  string
}

func acquireBookLease(dir string) (*bookLease, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("解析小说目录: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建小说目录: %w", err)
	}
	fileLock := flock.New(filepath.Join(absDir, bookLockFile), flock.SetPermissions(0o600))
	locked, err := fileLock.TryLock()
	if err != nil {
		return nil, closeBookLockAfterFailure(fileLock, fmt.Errorf("占用小说目录 %q: %w", absDir, err))
	}
	if !locked {
		return nil, closeBookLockAfterFailure(fileLock, fmt.Errorf(
			"%w：%s；请关闭正在操作此目录的另一个终端，或使用不同的小说目录",
			ErrBookInUse,
			absDir,
		))
	}
	return &bookLease{lock: fileLock, dir: absDir}, nil
}

func closeBookLockAfterFailure(fileLock *flock.Flock, cause error) error {
	if err := fileLock.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("关闭小说目录锁: %w", err))
	}
	return cause
}

func (l *bookLease) Close() error {
	if l == nil {
		return nil
	}
	var closeErr error
	if l.dir != "" {
		if err := bootstrap.CloseWebRuntimeForOutputDir(l.dir); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	if l.lock != nil {
		if err := l.lock.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
		l.lock = nil
	}
	return closeErr
}
