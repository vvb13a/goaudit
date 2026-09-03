package tui

import (
	"fmt"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// openInBrowserCmd opens the given URL in the user's default browser. If the
// browser cannot be launched, the returned command emits a danger
// notification instead.
func openInBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}

		if err := cmd.Start(); err != nil {
			return NotifyDanger(fmt.Sprintf("Failed to open '%s': %v", url, err))
		}
		if err := cmd.Wait(); err != nil {
			return NotifyDanger(fmt.Sprintf("Failed to open '%s': %v", url, err))
		}
		return nil
	}
}
