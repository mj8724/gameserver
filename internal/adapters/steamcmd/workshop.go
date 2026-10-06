package steamcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

// DownloadWorkshopItem downloads one Steam Workshop item into the instance
// install directory and verifies the content directory appeared. Works through
// the same single in-flight gate as installs (busy/startGate), so install and
// workshop traffic can never run concurrently on one Runner.
func (r *Runner) DownloadWorkshopItem(ctx context.Context, instance domain.InstanceID, workshopID string, onProgress func(ports.Progress)) (string, error) {
	if r.config == nil {
		return "", errors.New("SteamCMD installer is not configured")
	}
	if strings.TrimSpace(workshopID) == "" || !allDigits(workshopID) {
		return "", errors.New("workshop id must be numeric")
	}
	if strings.ContainsRune(workshopID, '\x00') {
		return "", errors.New("invalid workshop id")
	}
	installDir, err := r.config.InstallPath(instance)
	if err != nil {
		return "", fmt.Errorf("resolve install directory: %w", err)
	}
	// Keep the SteamCMD output so a failed download can explain itself instead
	// of only reporting the missing directory.
	var tail []string
	if err := r.InstallSpec(ctx, InstallSpec{
		Executable: r.config.Executable,
		SteamDir:   r.config.SteamDir,
		InstallDir: installDir,
		AppID:      r.config.AppID,
		WorkshopID: workshopID,
	}, func(line string) {
		tail = append(tail, line)
		if len(tail) > 12 {
			tail = tail[len(tail)-12:]
		}
		r.setStatus("Downloading workshop item " + workshopID)
	}, func(progress ports.Progress) {
		progress.Message = "workshop: " + progress.Message
		if onProgress != nil {
			onProgress(progress)
		}
	}); err != nil {
		return "", err
	}
	contentDir := filepath.Join(installDir, "steamapps", "workshop", "content", r.config.AppID, workshopID)
	if info, statErr := os.Stat(contentDir); statErr == nil && info.IsDir() {
		return filepath.ToSlash(contentDir), nil
	}
	// SteamCMD could not fetch it (typically no authenticated account). Fall
	// back to the operator's Steam client cache when one is configured: the
	// content is already on disk, and copying it keeps the anonymous boundary.
	if r.config.LocalCache.Enabled() {
		// Stage appends <appID>/<itemID> itself, so hand it the content root.
		contentRoot := filepath.Join(installDir, "steamapps", "workshop", "content")
		staged, stageErr := r.config.LocalCache.Stage(r.config.AppID, workshopID, contentRoot)
		if stageErr == nil {
			return staged, nil
		}
		if len(tail) > 0 {
			return "", fmt.Errorf("workshop item %s not fetched by steamcmd (%s) and not staged from the local cache (%v)", workshopID, strings.Join(tail, " | "), stageErr)
		}
		return "", fmt.Errorf("workshop item %s not staged from the local cache: %w", workshopID, stageErr)
	}
	if len(tail) > 0 {
		return "", fmt.Errorf("workshop item %s did not land under %s; steamcmd said: %s", workshopID, contentDir, strings.Join(tail, " | "))
	}
	return "", fmt.Errorf("workshop item %s did not land under %s", workshopID, contentDir)
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(value) > 0
}

// modInfoExists performs the structural content check: the item directory must
// contain a mod.info file with a parseable seed line. No executable content is
// ever read or executed (Workshop content is untrusted).
func modInfoExists(contentDir string) bool {
	info := filepath.Join(contentDir, "mod.info")
	raw, err := os.ReadFile(info)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "name=") {
			return true
		}
	}
	return false
}
