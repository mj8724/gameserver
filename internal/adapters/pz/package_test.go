package pz

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mj8724/gameserver/internal/domain"
	"github.com/mj8724/gameserver/internal/ports"
)

func TestINIReadWritePreservesCommentsUnknownKeysAndOnlyManagedChanges(t *testing.T) {
	root := t.TempDir()
	config, err := NewConfig(root, func(context.Context, domain.InstanceID) (string, error) { return "servertest", nil })
	if err != nil {
		t.Fatal(err)
	}
	path, err := config.Path("pz_01", "servertest")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("# PZ custom file\r\n\r\nPublic=false\r\nRCONPassword=keep-secret\r\nUnknownSetting=hello=world\r\n; semicolon comment\r\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Apply(context.Background(), "pz_01", map[string]string{"Public": "true", "Password": "new-password", "WorkshopItems": "1;2"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"# PZ custom file\r\n", "RCONPassword=keep-secret\r\n", "UnknownSetting=hello=world\r\n", "; semicolon comment\r\n", "Public=true\r\n", "Password=new-password\n", "WorkshopItems=1;2\n"} {
		if !bytes.Contains(got, []byte(required)) {
			t.Errorf("INI lost or omitted %q:\n%s", required, got)
		}
	}
	values, err := config.Read(context.Background(), "pz_01")
	if err != nil {
		t.Fatal(err)
	}
	if values["Public"] != "true" || values["UnknownSetting"] != "hello=world" {
		t.Fatalf("unexpected config values: %#v", values)
	}
	if runtime.GOOS != "windows" {
		for _, dir := range []string{filepath.Join(root, "servers"), filepath.Join(root, "servers", "pz_01"), filepath.Join(root, "servers", "pz_01", "Zomboid"), filepath.Dir(path)} {
			info, err := os.Stat(dir)
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Fatalf("directory %s mode/err = %v/%v", dir, info.Mode().Perm(), err)
			}
		}
		for _, item := range []string{path, path + ".bak"} {
			info, err := os.Stat(item)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("file %s mode/err = %v/%v", item, info.Mode().Perm(), err)
			}
		}
	}
}

func TestINIValidationRejectsTraversalUnknownKeysAndLineInjection(t *testing.T) {
	config, err := NewConfig(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "../bad", "has/slash", strings.Repeat("x", 65), "white space"} {
		if _, err := config.Path("pz_01", name); err == nil {
			t.Errorf("expected invalid server name %q", name)
		}
	}
	for _, id := range []domain.InstanceID{"../bad", "pz/01"} {
		if _, err := config.Path(id, "valid"); err == nil {
			t.Errorf("expected invalid instance id %q", id)
		}
	}
	if err := config.ApplyNamed(context.Background(), "pz_01", "valid", map[string]string{"RCONPassword": "overwrite"}); err == nil {
		t.Fatal("unknown/unmanaged keys must not be changed")
	}
	if err := config.ApplyNamed(context.Background(), "pz_01", "valid", map[string]string{"Password": "first\nInjected=yes"}); err == nil {
		t.Fatal("line injection must be rejected")
	}
}

func TestINIAtomicFailureDoesNotReturnSuccessAndPreservesOldOrNew(t *testing.T) {
	stages := []string{"mkdir", "read-old", "create-temp", "chmod", "write", "sync", "close", "backup.create", "backup.chmod", "backup.write", "backup.sync", "backup.close", "backup.rename", "backup.dirsync", "target.rename", "target.dirsync", "readback"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "servers", "pz_01", "Zomboid", "Server", "test.ini")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			old := []byte("#old\nPublic=false\nUnknown=old\n")
			if err := os.WriteFile(path, old, 0o600); err != nil {
				t.Fatal(err)
			}
			ops := &failingINIOps{base: osINIFileOps{}, failAt: stage}
			config, err := newConfig(root, nil, ops)
			if err != nil {
				t.Fatal(err)
			}
			err = config.ApplyNamed(context.Background(), "pz_01", "test", map[string]string{"Public": "true"})
			if err == nil {
				t.Fatalf("injected %s fault returned success", stage)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(got, old) && !bytes.Contains(got, []byte("Public=true")) {
				t.Fatalf("target is not old or new after failure %s: %q", stage, got)
			}
		})
	}
}

type failingINIOps struct {
	base    iniFileOps
	failAt  string
	tempN   int
	reads   int
	renames int
	dirsync int
}

func (f *failingINIOps) hit(stage string) error {
	if f.failAt == stage {
		return errors.New("injected " + stage)
	}
	return nil
}
func (f *failingINIOps) MkdirAll(path string, mode os.FileMode) error {
	if err := f.hit("mkdir"); err != nil {
		return err
	}
	return f.base.MkdirAll(path, mode)
}
func (f *failingINIOps) Lstat(path string) (os.FileInfo, error) { return f.base.Lstat(path) }
func (f *failingINIOps) ReadFile(path string) ([]byte, error) {
	f.reads++
	stage := "read-old"
	if strings.HasSuffix(path, ".bak") {
		stage = "read-old"
	}
	if f.reads > 2 {
		stage = "readback"
	}
	if err := f.hit(stage); err != nil {
		return nil, err
	}
	return f.base.ReadFile(path)
}
func (f *failingINIOps) CreateTemp(dir, pattern string) (iniTempFile, error) {
	f.tempN++
	stage := "create-temp"
	prefix := ""
	if strings.Contains(pattern, ".bak.") {
		stage = "backup.create"
		prefix = "backup."
	}
	if err := f.hit(stage); err != nil {
		return nil, err
	}
	file, err := f.base.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &failingINITemp{iniTempFile: file, ops: f, prefix: prefix}, nil
}
func (f *failingINIOps) Rename(oldPath, newPath string) error {
	f.renames++
	stage := "target.rename"
	if strings.HasSuffix(newPath, ".bak") {
		stage = "backup.rename"
	}
	if err := f.hit(stage); err != nil {
		return err
	}
	return f.base.Rename(oldPath, newPath)
}
func (f *failingINIOps) Remove(path string) error { return f.base.Remove(path) }
func (f *failingINIOps) SyncDir(path string) error {
	f.dirsync++
	stage := "target.dirsync"
	if f.dirsync == 1 {
		stage = "backup.dirsync"
	}
	if err := f.hit(stage); err != nil {
		return err
	}
	return f.base.SyncDir(path)
}

type failingINITemp struct {
	iniTempFile
	ops    *failingINIOps
	prefix string
}

func (f *failingINITemp) Chmod(mode os.FileMode) error {
	if err := f.ops.hit(f.prefix + "chmod"); err != nil {
		return err
	}
	return f.iniTempFile.Chmod(mode)
}
func (f *failingINITemp) Write(data []byte) (int, error) {
	if err := f.ops.hit(f.prefix + "write"); err != nil {
		return 0, err
	}
	return f.iniTempFile.Write(data)
}
func (f *failingINITemp) Sync() error {
	if err := f.ops.hit(f.prefix + "sync"); err != nil {
		return err
	}
	return f.iniTempFile.Sync()
}
func (f *failingINITemp) Close() error {
	err := f.iniTempFile.Close()
	if err != nil {
		return err
	}
	stage := f.prefix + "close"
	return f.ops.hit(stage)
}

func TestBuildLaunchSpecDoesNotUseTemplateAndPreservesAdversarialArgv(t *testing.T) {
	password := `p'"` + "`$; &|()<>^%!\n$(touch /tmp/SHOULD_NOT_EXIST) %VAR%" + `\\tail`
	spec, err := BuildLaunchSpec(LaunchConfig{
		InstanceID:       "pz_01",
		InstallDir:       filepath.Join(t.TempDir(), "server-files"),
		CacheDir:         filepath.Join(t.TempDir(), "cache space"),
		Platform:         "linux",
		ServerName:       "valid_server",
		AdminPass:        password,
		ArtifactEvidence: LaunchArtifactEvidence{Platform: "linux", ExecutableName: "ProjectZomboid64", FileType: "ELF synthetic helper", DirectExecutable: true, ManifestReference: "test-fixture:synthetic"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Executable != filepath.Join(filepath.Clean(spec.WorkDir), "ProjectZomboid64") {
		t.Fatalf("unexpected direct executable: %q", spec.Executable)
	}
	// PZ 42.21 requires two tokens: -adminpassword <value>.
	found := 0
	for index, arg := range spec.Args {
		if arg == "-adminpassword" {
			found++
			if index+1 >= len(spec.Args) || spec.Args[index+1] != password {
				t.Fatalf("admin password token missing after -adminpassword: %#v", spec.Args)
			}
		}
		if strings.HasPrefix(arg, "-adminpassword=") {
			t.Fatalf("single-token admin password form must not be used: %#v", arg)
		}
		if strings.Contains(arg, "start_arguments") {
			t.Fatal("template launch text was included")
		}
	}
	if found != 1 {
		t.Fatalf("expected one admin argv element, got %#v", spec.Args)
	}
	for _, arg := range spec.Args {
		if strings.Contains(arg, "{{") {
			t.Fatal("template environment string was used")
		}
	}
	// The adversarial value must stay one argv element (a space inside it does
	// not split it) and the password must never be joined with its flag.
	spaceKept := false
	for _, arg := range spec.Args {
		if strings.Contains(arg, " ") {
			spaceKept = true
		}
	}
	if !spaceKept {
		t.Fatalf("adversarial value lost its internal space: %#v", spec.Args)
	}
}

func TestBuildLaunchSpecValidatesNamesAndNeverAcceptsTemplate(t *testing.T) {
	base := LaunchConfig{InstallDir: t.TempDir(), CacheDir: t.TempDir(), Platform: "linux", ServerName: "valid", AdminPass: "pw", ArtifactEvidence: LaunchArtifactEvidence{Platform: "linux", ExecutableName: "ProjectZomboid64", FileType: "ELF synthetic helper", DirectExecutable: true, ManifestReference: "test-fixture:synthetic"}}
	invalidID := base
	invalidID.InstanceID = "../bad"
	_, err := BuildLaunchSpec(invalidID)
	if err == nil {
		t.Fatal("invalid instance id accepted")
	}
	invalidName := base
	invalidName.InstanceID = "pz_01"
	invalidName.ServerName = "../bad"
	_, err = BuildLaunchSpec(invalidName)
	if err == nil {
		t.Fatal("invalid server name accepted")
	}
	validWindows := base
	validWindows.InstanceID = "pz_01"
	validWindows.Platform = "windows"
	validWindows.ServerName = "good"
	validWindows.ArtifactEvidence = LaunchArtifactEvidence{Platform: "windows", ExecutableName: "ProjectZomboid64.exe", FileType: "PE synthetic helper", DirectExecutable: true, ManifestReference: "test-fixture:synthetic"}
	_, err = BuildLaunchSpec(validWindows)
	if err != nil {
		t.Fatal(err)
	}
	unsupported := validWindows
	unsupported.Platform = "freebsd"
	unsupported.ArtifactEvidence.Platform = "freebsd"
	_, err = BuildLaunchSpec(unsupported)
	if err == nil {
		t.Fatal("unsupported platform accepted")
	}
	unattested := base
	unattested.InstanceID = "pz_01"
	unattested.ArtifactEvidence = LaunchArtifactEvidence{Platform: "linux", ExecutableName: "start-server.sh", FileType: "script", DirectExecutable: false, ManifestReference: "test-fixture:synthetic"}
	if _, err := BuildLaunchSpec(unattested); err == nil {
		t.Fatal("interpreter/script vector must be blocked without approved platform evidence")
	}
}

func TestReadinessUsesSERVERPORTAndInjectedProbeAndTimeout(t *testing.T) {
	probes := 0
	var selectedPort int
	probe := Readiness{Ports: map[string]int{"SERVER_PORT": 17200, "DIRECT_PORT": 17201}, Mode: ReadinessA2S, Timeout: 100 * time.Millisecond, Interval: time.Millisecond,
		Probe: func(_ context.Context, mode ReadinessMode, host string, port int, _ string) (bool, error) {
			probes++
			selectedPort = port
			if mode != ReadinessA2S || host != "127.0.0.1" {
				t.Fatalf("unexpected probe selection: mode=%s host=%s", mode, host)
			}
			return probes >= 2, nil
		}, Alive: func(context.Context, domain.InstanceID) (bool, error) { return true, nil },
	}
	ready, err := probe.Await(context.Background(), "pz_01")
	if err != nil || !ready || selectedPort != 17200 {
		t.Fatalf("ready=%v err=%v port=%d", ready, err, selectedPort)
	}

	never := Readiness{Port: 17200, Timeout: 5 * time.Millisecond, Interval: time.Millisecond, Probe: func(context.Context, ReadinessMode, string, int, string) (bool, error) { return false, nil }, Alive: func(context.Context, domain.InstanceID) (bool, error) { return true, nil }}
	ready, err = never.Await(context.Background(), "pz_01")
	if err != nil || ready {
		t.Fatalf("timeout should return false, err nil; got %v %v", ready, err)
	}
}

func TestReadinessRequiresManifestMarkerAndRunningProcess(t *testing.T) {
	probe := Readiness{Port: 16261, Mode: ReadinessLogMarker, Timeout: time.Second, Probe: func(_ context.Context, mode ReadinessMode, _ string, _ int, marker string) (bool, error) {
		return mode == ReadinessLogMarker && marker == "Synthetic Ready", nil
	}, Alive: func(context.Context, domain.InstanceID) (bool, error) { return false, nil }}
	if _, err := probe.Await(context.Background(), "pz_01"); err == nil {
		t.Fatal("missing marker must fail closed")
	}
	probe.Marker = "Synthetic Ready"
	ready, err := probe.Await(context.Background(), "pz_01")
	if err != nil || ready {
		t.Fatalf("dead process cannot be ready, got %v err=%v", ready, err)
	}
}

func TestReadinessPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := Readiness{Port: 16261, Probe: func(context.Context, ReadinessMode, string, int, string) (bool, error) { return false, nil }, Alive: func(context.Context, domain.InstanceID) (bool, error) { return true, nil }}
	if _, err := probe.Await(ctx, "pz_01"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

var _ ports.GameConfig = (*Config)(nil)
var _ ports.ReadinessProbe = Readiness{}

// The options path may write catalogue-authorised keys (for example the RCON
// password), while the legacy variables path keeps its code-owned allow-list.
func TestApplyNamedAllowedUsesCallerAuthorisation(t *testing.T) {
	root := t.TempDir()
	config, err := NewConfig(root, func(context.Context, domain.InstanceID) (string, error) { return "servertest", nil })
	if err != nil {
		t.Fatal(err)
	}
	path, err := config.Path("pz_01", "servertest")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# keep\nPublic=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := config.ApplyNamed(ctx, "pz_01", "servertest", map[string]string{"RCONPassword": "x"}); err == nil {
		t.Fatal("the legacy path must keep rejecting unmanaged keys")
	}
	allowed := map[string]bool{"RCONPassword": true}
	if err := config.ApplyNamedAllowed(ctx, "pz_01", "servertest",
		map[string]string{"RCONPassword": "s3cret"}, func(key string) bool { return allowed[key] }); err != nil {
		t.Fatalf("catalogue-authorised write must succeed: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "RCONPassword=s3cret") {
		t.Fatalf("authorised key missing from the INI:\n%s", raw)
	}
	if !strings.Contains(string(raw), "# keep") || !strings.Contains(string(raw), "Public=true") {
		t.Fatal("comments and untouched keys must survive")
	}
	if err := config.ApplyNamedAllowed(ctx, "pz_01", "servertest",
		map[string]string{"Other": "1"}, func(string) bool { return false }); err == nil {
		t.Fatal("unauthorised keys must still be rejected")
	}
	if err := config.ApplyNamedAllowed(ctx, "pz_01", "servertest", map[string]string{"Public": "true"}, nil); err == nil {
		t.Fatal("a missing authoriser must be rejected")
	}
}
