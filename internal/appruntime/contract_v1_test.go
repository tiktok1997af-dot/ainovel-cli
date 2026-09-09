package appruntime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrs "github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestContractVersionValidation(t *testing.T) {
	if err := validateContractVersion(""); err != nil {
		t.Fatalf("empty migration version rejected: %v", err)
	}
	if err := validateContractVersion(ContractVersion); err != nil {
		t.Fatalf("current version rejected: %v", err)
	}
	clientVersion := `desktop.v999-C:/private/profile/token`
	err := validateContractVersion(clientVersion)
	app := normalizeAppError(err)
	if app.Code != ErrorCodeContractMismatch || app.Category != ErrorCategoryValidation {
		t.Fatalf("mismatch mapping = %+v", app)
	}
	if strings.Contains(app.Message, clientVersion) || app.Message != safeErrorMessage(ErrorCodeContractMismatch) {
		t.Fatalf("contract mismatch echoed unsafe client version: %q", app.Message)
	}
}

func TestAppErrorJSONDoesNotLeakCause(t *testing.T) {
	cause := errors.New("secret internal path C:/private/profile/token")
	app := normalizeAppError(cause)
	data, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "private/profile/token") {
		t.Fatalf("cause leaked into JSON: %s", text)
	}
	var roundTrip AppError
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Code != app.Code || roundTrip.Category != app.Category || roundTrip.Message != app.Message {
		t.Fatalf("round trip mismatch: got %+v want %+v", roundTrip, app)
	}
}

func TestExistingAppErrorIsClonedAndResanitized(t *testing.T) {
	raw := &AppError{
		Code:      ErrorCodeCommandNotAllowed,
		Category:  ErrorCategoryConflict,
		Message:   "unsafe internal path C:/private/profile/token",
		Retryable: true,
		Cause:     ErrCommandNotAllowed,
	}
	got := normalizeAppError(raw)
	if got == raw {
		t.Fatal("boundary normalization must clone an existing AppError")
	}
	if got.Message != safeErrorMessage(ErrorCodeCommandNotAllowed) || strings.Contains(got.Message, "private/profile") {
		t.Fatalf("existing AppError was not re-sanitized: %+v", got)
	}
	if !errors.Is(got, ErrCommandNotAllowed) {
		t.Fatal("re-sanitized AppError must preserve the Go cause chain")
	}
	if raw.Message != "unsafe internal path C:/private/profile/token" {
		t.Fatal("normalization must not mutate the source AppError")
	}
}

func TestStructuredErrorMappings(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		code     ErrorCode
		category ErrorCategory
	}{
		{"validation", ErrInvalidCommand, ErrorCodeInvalidArgument, ErrorCategoryValidation},
		{"conflict", ErrCommandNotAllowed, ErrorCodeCommandNotAllowed, ErrorCategoryConflict},
		{"store-read", apperrs.ErrStoreRead, ErrorCodeStoreRead, ErrorCategoryStore},
		{"store-write", apperrs.ErrStoreWrite, ErrorCodeStoreWrite, ErrorCategoryStore},
		{"browser-auth", &webai.Error{Kind: webai.ErrorAuthRequired}, ErrorCodeBrowserAuth, ErrorCategoryBrowser},
		{"browser-timeout", &webai.Error{Kind: webai.ErrorTimeout, Retry: true}, ErrorCodeBrowserTimeout, ErrorCategoryBrowser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeAppError(tc.err)
			if got.Code != tc.code || got.Category != tc.category {
				t.Fatalf("mapping = %+v", got)
			}
		})
	}
}

func TestRecoveryMappingIsExplicit(t *testing.T) {
	app := recoveryError(errors.New("resume failed"))
	if app.Code != ErrorCodeRecoveryFailed || app.Category != ErrorCategoryRecovery || !app.Retryable {
		t.Fatalf("recovery mapping = %+v", app)
	}
}

func TestStableBrowserStatusProjection(t *testing.T) {
	if got := normalizeBrowserStatus(webai.SessionReady); got != BrowserReady {
		t.Fatalf("ready = %q", got)
	}
	if got := normalizeBrowserStatus(webai.SessionState("FUTURE_STATE")); got != BrowserUnknown {
		t.Fatalf("unknown = %q", got)
	}
}

func TestTransportEnvelopesRoundTrip(t *testing.T) {
	appErr := &AppError{Code: ErrorCodeCommandRejected, Category: ErrorCategoryRuntime, Message: "rejected", Retryable: true}
	values := []any{
		DesktopSnapshot{Contract: CurrentContract()},
		CommandRequest{ContractVersion: ContractVersion, ID: "cmd-1", Kind: CommandPause},
		CommandResult{ContractVersion: ContractVersion, CommandID: "cmd-1", Error: appErr},
		QueryRequest{ContractVersion: ContractVersion, Kind: QueryKind("project")},
		QueryResult{ContractVersion: ContractVersion, Kind: QueryKind("project"), Error: appErr},
		DesktopEvent{ContractVersion: ContractVersion, Category: "LIFECYCLE", Type: "state", Error: appErr},
		EventCursor{ContractVersion: ContractVersion, AfterSeq: 9},
	}
	for _, value := range values {
		if _, err := json.Marshal(value); err != nil {
			t.Fatalf("marshal %T: %v", value, err)
		}
	}
}

func TestDesktopErrorEventDropsRawPayload(t *testing.T) {
	sub := newDesktopSubscription(1, nil)
	sub.offer(DesktopEvent{
		Level:    "error",
		Category: "ERROR",
		Summary:  "C:/private/profile/token leaked",
		Payload:  json.RawMessage(`{"detail":"secret"}`),
	})
	ev := <-sub.Events()
	if ev.ContractVersion != ContractVersion || ev.Error == nil {
		t.Fatalf("missing structured error/version: %+v", ev)
	}
	if ev.Payload != nil || strings.Contains(ev.Summary, "private/profile") {
		t.Fatalf("raw error data leaked: %+v", ev)
	}
}

func TestAppErrorPreservesErrorsIsInsideGo(t *testing.T) {
	app := normalizeAppError(ErrCommandNotAllowed)
	if !errors.Is(app, ErrCommandNotAllowed) {
		t.Fatal("AppError must preserve errors.Is for Go callers")
	}
}
