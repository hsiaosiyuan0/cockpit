//go:build !darwin || !cgo

package macosfocus

func Bundle(bundleID string) error {
	return nil
}
