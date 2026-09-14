package flow

import (
	"fmt"
	"sync"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

var runtimeReviewIntervals sync.Map // map[*store.Store]int

// BindRuntimeReviewInterval binds the process-effective D07 checkpoint cadence
// to one Store instance. Desktop settings are next-process settings, so callers
// bind from the Host runtime snapshot rather than rereading the persisted file.
func BindRuntimeReviewInterval(store *storepkg.Store, interval int) error {
	if store == nil {
		return fmt.Errorf("review checkpoint store is required")
	}
	if interval <= 0 {
		return fmt.Errorf("review checkpoint interval must be positive")
	}
	runtimeReviewIntervals.Store(store, interval)
	return nil
}

// UnbindRuntimeReviewInterval removes one process-scoped binding during desktop
// runtime shutdown. It does not mutate persisted project configuration.
func UnbindRuntimeReviewInterval(store *storepkg.Store) {
	if store != nil {
		runtimeReviewIntervals.Delete(store)
	}
}

func runtimeReviewInterval(store *storepkg.Store) int {
	if store != nil {
		if value, ok := runtimeReviewIntervals.Load(store); ok {
			if interval, ok := value.(int); ok && interval > 0 {
				return interval
			}
		}
	}
	return domain.ReviewInterval
}
