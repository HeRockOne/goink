//go:build !windows

package llm

import "errors"

func openInternetSettingsKey() (interface{ Close() error }, error) {
	return nil, errors.New("not windows")
}

func readDWORD(_ interface{}, _ string) (uint32, error) {
	return 0, errors.New("not windows")
}

func readString(_ interface{}, _ string) (string, error) {
	return "", errors.New("not windows")
}
