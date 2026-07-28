package recoveryreadiness

import "os"

func writeTestFile(path string, contents []byte) error {
	return os.WriteFile(path, contents, 0o600)
}
