//go:build windows

package override

import "errors"

func issueLaunchCapability(LaunchCapability) error {
	return errors.New("root-attested launch capabilities are not implemented on Windows")
}

func ConsumeLaunchCapability(LaunchCapability) error {
	return errors.New("root-attested launch capabilities are not implemented on Windows")
}

func LoadLaunchCapability(string) (LaunchCapability, error) {
	return LaunchCapability{}, errors.New("root-attested launch capabilities are not implemented on Windows")
}
