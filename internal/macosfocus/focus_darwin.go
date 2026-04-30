//go:build darwin && cgo

package macosfocus

import (
	"errors"
	"runtime"

	"github.com/progrium/darwinkit/macos/appkit"
	"github.com/progrium/darwinkit/objc"
)

var ErrApplicationNotRunning = errors.New("application is not running")

func Bundle(bundleID string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	activated := false
	objc.WithAutoreleasePool(func() {
		apps := appkit.RunningApplication_RunningApplicationsWithBundleIdentifier(bundleID)
		for _, app := range apps {
			if app.IsTerminated() {
				continue
			}
			app.Unhide()
			if app.ActivateWithOptions(appkit.ApplicationActivateAllWindows | appkit.ApplicationActivateIgnoringOtherApps) {
				activated = true
				return
			}
		}
	})

	if !activated {
		return ErrApplicationNotRunning
	}
	return nil
}
