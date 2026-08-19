//go:build windows

package llm

import (
	"golang.org/x/sys/windows/registry"
)

const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func openInternetSettingsKey() (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.READ)
}

func readDWORD(key registry.Key, name string) (uint32, error) {
	val, _, err := key.GetIntegerValue(name)
	return uint32(val), err
}

func readString(key registry.Key, name string) (string, error) {
	val, _, err := key.GetStringValue(name)
	return val, err
}
