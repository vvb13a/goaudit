package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// openInBrowserCmd opens the given URL in the user's default browser. If the
// browser cannot be launched, the returned command emits a danger
// notification instead.
func openInBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := openWithDefaultApp(url); err != nil {
			return NotifyDanger(fmt.Sprintf("Failed to open '%s': %v", url, err))
		}
		return nil
	}
}

// openWithDefaultApp launches the target (a URL or a file path) in the
// default application of the current platform and waits for the launcher to
// hand it over.
func openWithDefaultApp(target string) error {
	var name string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{target}
	case "windows":
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			name, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
		} else {
			name, args = "cmd", []string{"/c", "start", "", target}
		}
	default:
		name, args = "xdg-open", []string{target}
	}

	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}
