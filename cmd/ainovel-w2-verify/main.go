package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("ainovel-w2-verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", "", "Đường dẫn Chrome (để trống để tự tìm)")
	profile := fs.String("profile", "", "Tên profile xác minh mới")
	profileDir := fs.String("profile-dir", "", "Thư mục profile xác minh")
	evidence := fs.String("evidence", "", "Đường dẫn evidence JSON")
	loginTimeout := fs.Duration("login-timeout", 15*time.Minute, "Thời gian chờ đăng nhập")
	restartTimeout := fs.Duration("restart-timeout", 30*time.Second, "Thời gian chờ READY sau restart")
	restartDelay := fs.Duration("restart-delay", 2*time.Second, "Khoảng nghỉ trước khi mở lại Chrome")
	poll := fs.Duration("poll", 2*time.Second, "Chu kỳ kiểm tra readiness")
	watchUserAction := fs.Duration("watch-user-action", 0, "Tùy chọn: tiếp tục quan sát AUTH_REQUIRED sau khi restart READY")
	reuse := fs.Bool("reuse-profile", false, "Cho phép dùng profile đã tồn tại (không dùng cho clean-profile gate)")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "ainovel-w2-verify không nhận tham số vị trí")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Println("W2E — LOCAL REAL-BROWSER VERIFICATION")
	fmt.Println("WEB-ONLY / NO-API — Tool sẽ mở Chrome và chỉ kiểm tra trạng thái đăng nhập/readiness.")
	fmt.Println("Khi thấy AUTH_REQUIRED, hãy đăng nhập Google/Gemini trong cửa sổ Chrome được mở. Tool sẽ tự làm phần còn lại.")
	fmt.Println()

	result, err := webai.RunBrowserVerification(ctx, webai.VerificationConfig{
		Site:                   "gemini-web",
		BrowserPath:            *browser,
		ProfileName:            *profile,
		ProfileDir:             *profileDir,
		EvidencePath:           *evidence,
		PollInterval:           *poll,
		LoginTimeout:           *loginTimeout,
		RestartTimeout:         *restartTimeout,
		RestartDelay:           *restartDelay,
		WatchUserActionTimeout: *watchUserAction,
		AllowExistingProfile:   *reuse,
		OnEvent: func(event webai.VerificationEvent) {
			fmt.Printf("[W2E] %-18s %-14s %s\n", event.Phase, event.State, event.Reason)
			if event.State == webai.SessionAuthRequired {
				fmt.Println("[W2E] Cần thao tác người dùng: đăng nhập trong Chrome. Không cần nhập gì vào cửa sổ này.")
			}
			if event.Phase == "login_ready" && event.State == webai.SessionReady {
				fmt.Println("[W2E] Đã nhận READY. Tool sẽ tự restart Chrome để kiểm tra session persistence.")
			}
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nW2E FAILED: %v\n", err)
		if result.EvidencePath != "" {
			fmt.Fprintf(os.Stderr, "Evidence: %s\n", result.EvidencePath)
		}
		return 1
	}
	fmt.Println("\nW2E PASS: clean AUTH_REQUIRED -> login READY -> restart READY")
	fmt.Printf("Evidence: %s\n", result.EvidencePath)
	if !result.Evidence.UserActionObserved {
		fmt.Println("Logout/security real-browser branch chưa được ép thực hiện; Tool không tự đăng xuất tài khoản Google của bạn.")
	}
	return 0
}
