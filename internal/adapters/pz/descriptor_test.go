package pz

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func writeDescriptor(t *testing.T, installDir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(installDir, "jre64", "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	if err := os.WriteFile(filepath.Join(installDir, "jre64", "bin", name), []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(installDir, "ProjectZomboid64.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const vendorDescriptor = `{"mainClass":"zombie/network/GameServer",
 "classpath":["java/.","java/projectzomboid.jar"],
 "vmArgs":["-Djava.awt.headless=true","-Xmx3072m","-Dzomboid.steam=1","-Dzomboid.znetlog=1","-Djava.library.path=natives/","-XX:-CreateCoredumpOnCrash","-XX:-OmitStackTraceInFastThrow"],
 "windows":{"7":{"vmArgs":["-XX:+UseG1GC"]},"10":{"vmArgs":["-XX:+UseZGC"]}}}`

var vendorVMArgs = []string{"-Djava.awt.headless=true", "-Xmx3072m", "-Dzomboid.steam=1", "-Dzomboid.znetlog=1",
	"-Djava.library.path=natives/", "-XX:-CreateCoredumpOnCrash", "-XX:-OmitStackTraceInFastThrow", "-XX:+UseZGC"}

func TestDescriptorLaunchSpecMatchesVendorShape(t *testing.T) {
	dir := t.TempDir()
	writeDescriptor(t, dir, vendorDescriptor)
	descriptor, err := LoadLauncherDescriptor(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	result, err := DescriptorLaunchSpec(descriptor, DescriptorConfig{
		InstallDir: dir, CacheDir: filepath.Join(dir, "Zomboid"), ServerName: "servertest",
		AdminPass: "secret-pass", MemoryMB: 4096, WindowsVer: "10.0.19045", AllowedArgs: vendorVMArgs,
	})
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if !strings.HasSuffix(result.Executable, filepath.Join("jre64", "bin", "java")) &&
		!strings.HasSuffix(result.Executable, filepath.Join("jre64", "bin", "java.exe")) {
		t.Fatalf("executable = %q", result.Executable)
	}
	args := result.Args
	if len(args) < 12 {
		t.Fatalf("argv too short: %v", args)
	}
	if !reflect.DeepEqual(args[:8], vendorVMArgs) {
		t.Fatalf("vmArgs = %v\nwant %v", args[:8], vendorVMArgs)
	}
	if args[8] != "-Xms4096m" || args[9] != "-Xmx4096m" {
		t.Fatalf("memory override = %v", args[8:10])
	}
	if args[10] != "-cp" {
		t.Fatalf("expected -cp after memory: %v", args[10:])
	}
	wantClasspath := "java/" + string(filepath.ListSeparator) + "java/projectzomboid.jar"
	joined := strings.Join(result.Args, " ")
	for _, want := range []string{"-cp " + wantClasspath, "zombie/network/GameServer", "-statistic 0",
		"-cachedir=", "-servername=servertest", "-adminpassword=secret-pass"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv missing %q: %v", want, result.Args)
		}
	}
	if strings.Contains(joined, "java/. ") || strings.HasSuffix(joined, "java/.") {
		t.Fatalf("vendor classpath typo must be rewritten: %v", result.Args)
	}
	if result.WorkDir != dir {
		t.Fatalf("workdir = %q, want %q", result.WorkDir, dir)
	}
	if len(result.Rewrites) == 0 {
		t.Fatal("rewrites must be reported")
	}
}

func TestDescriptorRejectsForbiddenVMArgs(t *testing.T) {
	cases := map[string]string{
		"javaagent":          `{"mainClass":"zombie/network/GameServer","classpath":["java/"],"vmArgs":["-javaagent:/tmp/evil.jar"]}`,
		"classpath override": `{"mainClass":"zombie/network/GameServer","classpath":["java/"],"vmArgs":["-cp","/tmp/evil.jar"]}`,
		"argfile":            `{"mainClass":"zombie/network/GameServer","classpath":["java/"],"vmArgs":["@/tmp/args"]}`,
		"not approved":       `{"mainClass":"zombie/network/GameServer","classpath":["java/"],"vmArgs":["-Dunknown=1"]}`,
		"duplicate":          `{"mainClass":"zombie/network/GameServer","classpath":["java/"],"vmArgs":["-Dzomboid.steam=1","-Dzomboid.steam=1"]}`,
		"empty token":        `{"mainClass":"zombie/network/GameServer","classpath":["java/"],"vmArgs":[""]}`,
		"control char":       `{"mainClass":"zombie/network/GameServer","classpath":["java/"],"vmArgs":["-Dx=1\n-Dy=2"]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeDescriptor(t, dir, body)
			descriptor, err := LoadLauncherDescriptor(dir)
			if err != nil {
				return // rejected at load time is acceptable
			}
			if _, err := DescriptorLaunchSpec(descriptor, DescriptorConfig{
				InstallDir: dir, CacheDir: dir, ServerName: "s", MemoryMB: 3072, WindowsVer: "10", AllowedArgs: vendorVMArgs,
			}); err == nil {
				t.Fatalf("expected rejection for %s", name)
			}
		})
	}
}

func TestDescriptorRejectsMalformedArtifacts(t *testing.T) {
	t.Run("missing descriptor", func(t *testing.T) {
		dir := t.TempDir()
		writeDescriptor(t, dir, "")
		if _, err := LoadLauncherDescriptor(dir); err == nil {
			t.Fatal("missing descriptor must be rejected")
		}
	})
	t.Run("missing java", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "ProjectZomboid64.json"), []byte(vendorDescriptor), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := DescriptorLaunchSpec(LauncherDescriptor{MainClass: "a", Classpath: []string{"java/"}},
			DescriptorConfig{InstallDir: dir, CacheDir: dir, ServerName: "s", MemoryMB: 3072}); err == nil {
			t.Fatal("missing java must be rejected")
		}
	})
	t.Run("missing mainClass", func(t *testing.T) {
		dir := t.TempDir()
		writeDescriptor(t, dir, `{"classpath":["java/"]}`)
		if _, err := LoadLauncherDescriptor(dir); err == nil {
			t.Fatal("missing mainClass must be rejected")
		}
	})
	t.Run("memory out of range", func(t *testing.T) {
		dir := t.TempDir()
		writeDescriptor(t, dir, `{"mainClass":"zombie/network/GameServer","classpath":["java/"]}`)
		descriptor, err := LoadLauncherDescriptor(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DescriptorLaunchSpec(descriptor, DescriptorConfig{InstallDir: dir, CacheDir: dir, ServerName: "s", MemoryMB: 64}); err == nil {
			t.Fatal("memory below the supported floor must be rejected")
		}
	})
}
