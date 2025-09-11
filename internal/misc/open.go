package misc

import (
	"os/exec"
	"runtime"
	"testing"
)

func Open(url string) {
	if testing.Testing() {
		return
	}

	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Run()
	case "windows":
		_ = exec.Command("start", url).Run()
	default:
		_ = exec.Command("xdg-open", url).Run()
	}
}
