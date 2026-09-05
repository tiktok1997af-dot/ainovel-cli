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
	loginTimeout := fs.Duration("login-timeout", 15*time.Minute, "Thời gian chờ đăng nhập và đóng Chrome bình thường")
	restartTimeout := fs.Duration("restart-timeout", 30*time.Second, "Thời gian chờ READY sau khi mở lại")
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
	fmt.Println("WEB-ONLY / NO-API — Tool chỉ kiểm tra Chrome/Gemini, không gửi prompt.")
	fmt.Println("Verifier sẽ kiểm tra AUTH_REQUIRED trước, rồi tự mở lại Chrome BÌNH THƯỜNG để bạn đăng nhập.")
	fmt.Println("Chỉ đăng nhập ở cửa sổ Chrome bình thường đó. Khi Gemini đã vào được, hãy ĐÓNG cửa sổ Chrome để Tool tự tiếp tục.")
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
			fmt.Printf("[W2E] %-22s %-14s %s\n", event.Phase, event.State, event.Reason)
			switch event.Phase {
			case "initial":
				if event.State == webai.SessionAuthRequired {
					fmt.Println("[W2E] Clean-profile AUTH_REQUIRED đã xác nhận. KHÔNG đăng nhập ở cửa sổ này; Tool sẽ tự đóng nó.")
				}
			case "login_normal_open":
				fmt.Println("[W2E] BÂY GIỜ hãy đăng nhập Google/Gemini trong Chrome bình thường.")
				fmt.Println("[W2E] Khi Gemini đã mở ở trạng thái đăng nhập, hãy ĐÓNG cửa sổ Chrome. Tool sẽ tự kiểm tra phần còn lại.")
			case "login_ready":
				if event.State == webai.SessionReady {
					fmt.Println("[W2E] Đã nhận READY sau đăng nhập. Tool sẽ tự restart Chrome để kiểm tra session persistence.")
				}
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
	fmt.Println("\nW2E PASS: clean AUTH_REQUIRED -> normal Chrome login -> READY -> restart READY")
	fmt.Printf("Evidence: %s\n", result.EvidencePath)
	if !result.Evidence.UserActionObserved {
		fmt.Println("Logout/security real-browser branch chưa được ép thực hiện; Tool không tự đăng xuất tài khoản Google của bạn.")
	}
	return 0
}
