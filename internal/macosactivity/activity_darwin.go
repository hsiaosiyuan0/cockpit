//go:build darwin && cgo

package macosactivity

import (
	"runtime"
	"sync"

	"github.com/progrium/darwinkit/macos/foundation"
	"github.com/progrium/darwinkit/objc"
)

var (
	mu       sync.Mutex
	activity objc.Object
)

func Begin(reason string) bool {
	mu.Lock()
	defer mu.Unlock()
	if !activity.IsNil() {
		return true
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	options := foundation.ActivityUserInitiatedAllowingIdleSystemSleep |
		foundation.ActivitySuddenTerminationDisabled |
		foundation.ActivityAutomaticTerminationDisabled
	objc.WithAutoreleasePool(func() {
		activity = foundation.ProcessInfo_ProcessInfo().BeginActivityWithOptionsReason(options, reason)
		if !activity.IsNil() {
			activity = activity.Retain()
		}
	})
	return !activity.IsNil()
}

func End() {
	mu.Lock()
	defer mu.Unlock()
	if activity.IsNil() {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	foundation.ProcessInfo_ProcessInfo().EndActivity(activity)
	activity.Release()
	activity = objc.Object{}
}
