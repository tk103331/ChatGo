package ui

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

// updateCheckInterval is how often the app checks for a newer release in
// the background. The plugin's own daily check would pop the update window
// on its own, which conflicts with the tray-only prompt, so the app turns
// automatic checks off and runs its own timer instead.
const updateCheckInterval = 30 * time.Minute

// StartUpdateChecker turns the plugin's automatic pop-up checks off and
// starts the app's quieter timer: a found update shows as a tray menu entry
// until the user clicks it to open the update window.
func (cw *ChatWindow) StartUpdateChecker() {
	updater.SetAutomaticChecks(false)
	go func() {
		cw.checkForUpdate()
		ticker := time.NewTicker(updateCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			cw.checkForUpdate()
		}
	}()
}

// checkForUpdate asks the update feed once, silently. Development builds
// and builds without updates fail with ErrUpdatesDisabled and are skipped;
// so are network errors, so a transient outage never clears a known update.
func (cw *ChatWindow) checkForUpdate() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	up, err := mygo.Updater.Check(ctx)
	if err != nil {
		return
	}
	cw.postMain(func() {
		cw.updateVer = up.Version
		cw.updateTrayMenu()
		cw.invalidate()
	})
}
