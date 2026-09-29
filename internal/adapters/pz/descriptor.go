package pz

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Launch vectors. direct-executable runs the platform binary (Linux/darwin, and
// Windows when the package ships ProjectZomboid64.exe); launcher-descriptor
// follows the vendor ProjectZomboid64.json and runs the bundled JRE directly.
const (
	VectorDirectExecutable   = "direct-executable"
	VectorLauncherDescriptor = "launcher-descriptor"
)

// classpathRewrite fixes vendor descriptor entries that are not loadable as
// written. The Windows server package ships "java/." in ProjectZomboid64.json
// while its own StartServer64.bat uses "java/"; the batch file is the observed
// working authority, so the rewrite is deterministic and logged rather than
// passed through.
var classpathRewrite = map[string]string{
	"java/.": "java/",
}

// descriptorStatisticArg mirrors StartServer64.bat, which passes "-statistic 0"
// although ProjectZomboid64.json does not mention it.
var descriptorStatisticArg = []string{"-statistic", "0"}

// forbiddenVMArg matches JVM options that would let the descriptor execute or
// load arbitrary code, overriding the code-owned argv authority.
var forbiddenVMArg = regexp.MustCompile(`(?i)(javaagent|agentlib|agentpath|^-cp$|^-classpath$|^-jar$|^@|-Xbootclasspath)`)

// allowedVMArg is the shape whitelist for descriptor vmArgs.
// Shape whitelist: system properties, -XX options, and ordinary -X flags
// (including the -Xmx/-Xms memory forms the vendor descriptor uses).
var allowedVMArg = regexp.MustCompile(`^-(D[A-Za-z0-9._]+(=.*)?|XX:[-+A-Za-z0-9._:=]+|X([a-zA-Z]+[0-9]+[mMgGkK]?|[a-zA-Z][A-Za-z0-9._:=-]*))$`)

// LauncherDescriptor is the vendor launch descriptor (ProjectZomboid64.json).
type LauncherDescriptor struct {
	MainClass string                      `json:"mainClass"`
	Classpath []string                    `json:"classpath"`
	VMArgs    []string                    `json:"vmArgs"`
	Windows   map[string]DescriptorVMRule `json:"windows"`
}

// DescriptorVMRule carries the per-Windows-version additions.
type DescriptorVMRule struct {
	VMArgs []string `json:"vmArgs"`
}

// DescriptorConfig carries the instance-owned parts of the launch.
type DescriptorConfig struct {
	InstallDir  string
	CacheDir    string
	ServerName  string
	AdminPass   string
	MemoryMB    int
	WindowsVer  string
	AllowedArgs []string
}

// LoadLauncherDescriptor reads and validates a vendor descriptor, rejecting
// traversal, symlinks and malformed fields.
func LoadLauncherDescriptor(installDir string) (LauncherDescriptor, error) {
	root, err := filepath.Abs(installDir)
	if err != nil {
		return LauncherDescriptor{}, err
	}
	path := filepath.Join(root, "ProjectZomboid64.json")
	info, err := os.Lstat(path)
	if err != nil {
		return LauncherDescriptor{}, fmt.Errorf("launcher descriptor not found: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return LauncherDescriptor{}, errors.New("launcher descriptor must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return LauncherDescriptor{}, errors.New("launcher descriptor must be a regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return LauncherDescriptor{}, err
	}
	var descriptor LauncherDescriptor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return LauncherDescriptor{}, fmt.Errorf("parse launcher descriptor: %w", err)
	}
	if err := descriptor.validate(); err != nil {
		return LauncherDescriptor{}, err
	}
	return descriptor, nil
}

func (d LauncherDescriptor) validate() error {
	if strings.TrimSpace(d.MainClass) == "" {
		return errors.New("launcher descriptor mainClass is required")
	}
	if len(d.Classpath) == 0 {
		return errors.New("launcher descriptor classpath is required")
	}
	for _, entry := range d.Classpath {
		if strings.TrimSpace(entry) == "" || strings.ContainsAny(entry, "\x00\n\r") {
			return errors.New("launcher descriptor classpath entry is invalid")
		}
	}
	return nil
}

// javaExecutable resolves the bundled JRE inside the install directory.
func javaExecutable(installDir string) (string, error) {
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	path := filepath.Join(installDir, "jre64", "bin", name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("bundled java executable not found: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("bundled java executable must be a regular file")
	}
	return path, nil
}

// mergedVMArgs appends the per-Windows-version rule to the generic vmArgs, which
// is what StartServer64.bat does (Windows 10 -> -XX:+UseZGC).
func (d LauncherDescriptor) mergedVMArgs(windowsVersion string) []string {
	merged := append([]string(nil), d.VMArgs...)
	if windowsVersion == "" || d.Windows == nil {
		return merged
	}
	major := strings.SplitN(windowsVersion, ".", 2)[0]
	rule, ok := d.Windows[major]
	if !ok {
		return merged
	}
	return append(merged, rule.VMArgs...)
}

// validateVMArgs enforces the code-owned whitelist: the descriptor may only
// supply the options recorded in the Target Manifest, never code loading or
// classpath overrides.
func validateVMArgs(args []string, allowed []string) error {
	allow := make(map[string]bool, len(allowed))
	for _, token := range allowed {
		allow[token] = true
	}
	seen := make(map[string]bool, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			return errors.New("launcher descriptor contains an empty vmArgs token")
		}
		if strings.ContainsAny(arg, "\x00\n\r") {
			return errors.New("launcher descriptor vmArgs must not contain control characters")
		}
		if forbiddenVMArg.MatchString(arg) {
			return fmt.Errorf("launcher descriptor vmArgs token is forbidden: %q", arg)
		}
		if len(allow) > 0 && !allow[arg] {
			return fmt.Errorf("launcher descriptor vmArgs token is not manifest-approved: %q", arg)
		}
		if !allowedVMArg.MatchString(arg) {
			return fmt.Errorf("launcher descriptor vmArgs token has an unexpected shape: %q", arg)
		}
		if seen[arg] {
			return fmt.Errorf("launcher descriptor vmArgs token is duplicated: %q", arg)
		}
		seen[arg] = true
	}
	return nil
}

// classpathArg rewrites known vendor typos and joins with the platform separator.
func classpathArg(entries []string) string {
	rewritten := make([]string, 0, len(entries))
	for _, entry := range entries {
		if replacement, ok := classpathRewrite[entry]; ok {
			entry = replacement
		}
		rewritten = append(rewritten, entry)
	}
	return strings.Join(rewritten, string(filepath.ListSeparator))
}

// HeapMB reports the heap size declared by the vendor descriptor, so the
// service can keep the vendor default when no explicit value is configured.
func (d LauncherDescriptor) HeapMB() int {
	pattern := regexp.MustCompile(`^-Xmx([0-9]+)([mMgGkK]?)$`)
	for _, arg := range d.VMArgs {
		match := pattern.FindStringSubmatch(arg)
		if match == nil {
			continue
		}
		value, err := strconv.Atoi(match[1])
		if err != nil {
			return 0
		}
		switch strings.ToLower(match[2]) {
		case "g":
			return value * 1024
		case "k":
			return value / 1024
		default:
			return value
		}
	}
	return 0
}

// memoryArgPattern matches the vendor heap options that this adapter owns.
var memoryArgPattern = regexp.MustCompile(`^-Xm[sx]`)

// withoutMemoryArgs removes the vendor heap options so the code-owned values in
// MemoryMB are the only ones on the final argv.
func withoutMemoryArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if memoryArgPattern.MatchString(arg) {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

// DescriptorLaunchSpec builds the typed argv for the launcher-descriptor vector.
// Every token is passed as a single argv element: no shell, no interpolation.
func DescriptorLaunchSpec(descriptor LauncherDescriptor, config DescriptorConfig) (LaunchSpecResult, error) {
	installDir, err := filepath.Abs(config.InstallDir)
	if err != nil {
		return LaunchSpecResult{}, err
	}
	executable, err := javaExecutable(installDir)
	if err != nil {
		return LaunchSpecResult{}, err
	}
	// The vendor descriptor carries its own -Xms/-Xmx; those two are code-owned
	// here (see MemoryMB), so they are dropped before the vendor whitelist check
	// and re-issued from configuration.
	vendorArgs := withoutMemoryArgs(descriptor.mergedVMArgs(config.WindowsVer))
	if err := validateVMArgs(vendorArgs, config.AllowedArgs); err != nil {
		return LaunchSpecResult{}, err
	}
	if config.MemoryMB < 1024 || config.MemoryMB > 65536 {
		return LaunchSpecResult{}, fmt.Errorf("server memory %d MB is outside the supported range", config.MemoryMB)
	}

	args := append([]string(nil), vendorArgs...)
	memory := []string{"-Xms" + strconv.Itoa(config.MemoryMB) + "m", "-Xmx" + strconv.Itoa(config.MemoryMB) + "m"}
	args = append(args, memory...)
	args = append(args, "-cp", classpathArg(descriptor.Classpath), descriptor.MainClass)
	args = append(args, descriptorStatisticArg...)
	args = append(args,
		"-cachedir="+config.CacheDir,
		"-servername="+config.ServerName,
		// Two tokens: PZ 42.21 rejects -adminpassword=<value>.
		"-adminpassword", config.AdminPass,
	)
	return LaunchSpecResult{
		Executable: executable,
		Args:       args,
		WorkDir:    installDir,
		Rewrites:   []string{"java/. -> java/ (vendor descriptor typo; batch file is authoritative)"},
	}, nil
}

// LaunchSpecResult is the adapter-local launch description.
type LaunchSpecResult struct {
	Executable string
	Args       []string
	WorkDir    string
	Rewrites   []string
}

// sortedKeys is a small helper used by tests and diagnostics.
func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
