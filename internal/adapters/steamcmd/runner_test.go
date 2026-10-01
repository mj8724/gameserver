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

// Credentials are optional: the default session stays anonymous, and when
// configured both values are separate argv elements (never a joined string).
func TestBuildArgsCredentialedLoginIsOptionalAndInert(t *testing.T) {
	anonymous := BuildArgs(InstallSpec{Executable: "steamcmd", InstallDir: "/g", AppID: "380870", WorkshopID: "2169435993"})
	if !strings.Contains(strings.Join(anonymous, " "), "+login anonymous") {
		t.Fatalf("default session must stay anonymous: %v", anonymous)
	}
	authed := BuildArgs(InstallSpec{Executable: "steamcmd", InstallDir: "/g", AppID: "380870", WorkshopID: "2169435993", Login: "captain", Password: "p@ss word"})
	joined := strings.Join(authed, " ")
	if !strings.Contains(joined, "+login captain p@ss word") {
		t.Fatalf("credentials must be separate argv elements: %v", authed)
	}
	if strings.Contains(joined, "+login captain:p@ss") {
		t.Fatal("credentials must never be joined into one token")
	}
}

// The progress reader redacts an authenticated password from log lines.
func TestProgressReaderRedactsConfiguredPassword(t *testing.T) {
	reader := &progressReader{secret: "", extraSecrets: []string{"p@ss word"}}
	var lines []string
	reader.onLine = func(line string) { lines = append(lines, line) }
	if _, err := reader.Write([]byte("Logging in as captain with password p@ss word ...\n")); err != nil {
		t.Fatal(err)
	}
	reader.Flush()
	if len(lines) == 0 {
		t.Fatal("no lines captured")
	}
	for _, line := range lines {
		if strings.Contains(line, "p@ss word") {
			t.Fatalf("password leaked into logs: %q", line)
		}
	}
}
