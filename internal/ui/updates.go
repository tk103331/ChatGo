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
	cw.applyUpdate(up)
}

// applyUpdate records the result of a check. A nil update means the app is
// up to date, which clears a prompt left over from an update the user has
// since installed.
func (cw *ChatWindow) applyUpdate(up *mygo.Update) {
	ver := ""
	if up != nil {
		ver = up.Version
	}
	cw.postMain(func() {
		if cw.updateVer == ver {
			return
		}
		cw.updateVer = ver
		cw.updateTrayMenu()
		cw.invalidate()
	})
}
