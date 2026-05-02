//go:build !darwin || !cgo

package macosactivity

func Begin(reason string) bool {
	return false
}

func End() {}
