package host

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// startWebRuntime owns production browser bootstrap for W5A. A readiness probe
// may transiently return DEGRADED while Chrome starts; as long as the owned
// process is alive, the WebChatModel can later converge through Refresh.
func startWebRuntime(ctx context.Context, cfg bootstrap.Config, session *webai.SessionManager) (*webai.SessionManager, *bootstrap.ModelSet, error) {
	if !cfg.Web.Enabled {
		return nil, nil, fmt.Errorf("WEB-only runtime is not enabled")
	}
	if session == nil {
		session = webai.NewSessionManager(webai.SessionConfig{
			Site:        cfg.Web.Site,
			BrowserPath: cfg.Web.BrowserPath,
			ProfileName: cfg.Web.ProfileName,
			StartURL:    cfg.Web.StartURL,
		})
	}
	snap, startErr := session.Start(ctx)
	if startErr != nil {
		if snap.PID == 0 || snap.State == webai.SessionFailed || snap.State == webai.SessionStopped {
			_ = session.Stop()
			return nil, nil, fmt.Errorf("start WEB browser session: %w", startErr)
		}
		slog.Warn("WEB browser started before readiness settled", "module", "webai", "state", snap.State, "err", startErr)
	}
	models, err := bootstrap.NewWebModelSet(cfg, session)
	if err != nil {
		_ = session.Stop()
		return nil, nil, err
	}
	return session, models, nil
}
