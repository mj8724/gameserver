package steamcmd

import (
	"strings"
	"testing"
)

// A workshop download must use +workshop_download_item and must never also run
// an app update in the same invocation (M3.3 defect: the first implementation
// reused the app-update argv, so nothing was ever downloaded).
func TestBuildArgsWorkshopDownload(t *testing.T) {
	args := BuildArgs(InstallSpec{
		Executable: "steamcmd", SteamDir: "/steamcmd", InstallDir: "/games/pz_01/server_files",
		AppID: "380870", WorkshopID: "2169435993",
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "+workshop_download_item 380870 2169435993") {
		t.Fatalf("workshop argv missing: %v", args)
	}
	if strings.Contains(joined, "+app_update") {
		t.Fatalf("a workshop download must not update the app in the same run: %v", args)
	}
	if args[len(args)-1] != "+quit" {
		t.Fatalf("steamcmd argv must end with +quit: %v", args)
	}
	// The install path keeps using app_update.
	install := strings.Join(BuildArgs(InstallSpec{Executable: "steamcmd", InstallDir: "/games", AppID: "380870"}), " ")
	if !strings.Contains(install, "+app_update 380870") || strings.Contains(install, "workshop_download_item") {
		t.Fatalf("app install argv changed: %v", install)
	}
}
