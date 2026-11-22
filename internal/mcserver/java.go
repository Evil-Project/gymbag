package mcserver

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	maxNestedJarBytes  = 64 << 20
	maxClassFilesProbe = 256
	javaProbeTimeout   = 3 * time.Second
)

// JarRequirement describes the minimum Java version detected from a Minecraft server JAR.
type JarRequirement struct {
	MinMajor        int    `json:"min_major"`
	LegacyMinecraft bool   `json:"legacy_minecraft,omitempty"`
	MCVersion       string `json:"mc_version,omitempty"`
	Source          string `json:"source"`
}

// JavaInstall describes a discovered Java runtime binary and its parsed version.
type JavaInstall struct {
	Path       string `json:"path"`
	Major      int    `json:"major"`
	Minor      int    `json:"minor,omitempty"`
	Patch      int    `json:"patch,omitempty"`
	RawVersion string `json:"raw_version,omitempty"`
	Source     string `json:"source"`
}

// SelectionResult captures the resolved Java runtime for a server JAR and any non-fatal warnings.
type SelectionResult struct {
	Java        JavaInstall    `json:"java"`
	Requirement JarRequirement `json:"requirement"`
	Warning     string         `json:"warning,omitempty"`
}

// DiscoverOptions customizes Java runtime discovery and probing (injectable for tests).
type DiscoverOptions struct {
	Prober     func(ctx context.Context, binPath string) (string, error)
	Candidates []string
	Installs   []JavaInstall
	GOOS       string
	Getenv     func(string) string
}

type versionJSONDoc struct {
	ID          string `json:"id"`
	JavaVersion int    `json:"java_version"`
}

// InspectServerJar examines a Minecraft server .jar (Vanilla, Paper/Purpur bundler, Paperclip legacy,
// Fabric, Forge, or plain bytecode) and determines the minimum required Java major version.
func InspectServerJar(jarPath string) (JarRequirement, error) {
	p := strings.TrimSpace(jarPath)
	if p == "" {
		return JarRequirement{}, fmt.Errorf("server_jar path must not be empty")
	}
	rc, err := zip.OpenReader(p)
	if err != nil {
		return JarRequirement{}, fmt.Errorf("failed to inspect server jar %s: %w", p, err)
	}
	defer func() { _ = rc.Close() }()

	if req, ok := inspectZipReader(&rc.Reader, 0); ok {
		return req, nil
	}
	return JarRequirement{MinMajor: 8, Source: "default"}, nil
}

func inspectZipReader(zr *zip.Reader, depth int) (JarRequirement, bool) {
	if zr == nil {
		return JarRequirement{}, false
	}

	// 1. Mojang / Paper / Purpur version.json at archive root.
	for _, f := range zr.File {
		if f.Name == "version.json" {
			if req, ok := parseVersionJSONFile(f); ok {
				return req, true
			}
		}
	}

	// 2. Mojang 1.18+ / Paperclip bundler nested server JARs under META-INF/versions/.
	if depth == 0 {
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			lower := strings.ToLower(f.Name)
			if !strings.HasPrefix(lower, "meta-inf/versions/") || !strings.HasSuffix(lower, ".jar") {
				continue
			}
			if f.UncompressedSize64 > maxNestedJarBytes {
				continue
			}
			data, ok := readZipEntryLimit(f, maxNestedJarBytes)
			if !ok {
				continue
			}
			nested, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				continue
			}
			if req, found := inspectZipReader(nested, depth+1); found {
				if req.MCVersion == "" {
					if verDir := extractBundlerVersionDir(f.Name); verDir != "" {
						req.MCVersion = verDir
					}
				}
				return req, true
			}
		}
	}

	// 3. Legacy Paperclip patch.properties (version=1.16.5).
	for _, f := range zr.File {
		if f.Name == "patch.properties" {
			if req, ok := parsePatchPropertiesFile(f); ok {
				return req, true
			}
		}
	}

	// 4. META-INF/MANIFEST.MF + non-multi-release .class bytecode inspection.
	manifestAttrs := readManifestAttributes(zr)
	mainClassEntry := ""
	if mc := strings.TrimSpace(manifestAttrs["main-class"]); mc != "" {
		mainClassEntry = strings.ReplaceAll(mc, ".", "/") + ".class"
	}

	maxClassJava := 0
	inspected := 0
	inspectClass := func(f *zip.File) {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".class") {
			return
		}
		if strings.HasPrefix(strings.ToLower(f.Name), "meta-inf/versions/") {
			return
		}
		r, err := f.Open()
		if err != nil {
			return
		}
		var hdr [8]byte
		_, err = io.ReadFull(r, hdr[:])
		_ = r.Close()
		if err != nil {
			return
		}
		if binary.BigEndian.Uint32(hdr[0:4]) != 0xCAFEBABE {
			return
		}
		classMajor := int(binary.BigEndian.Uint16(hdr[6:8]))
		if classMajor >= 49 {
			javaMajor := classMajor - 44
			if javaMajor > maxClassJava {
				maxClassJava = javaMajor
			}
		}
		inspected++
	}

	if mainClassEntry != "" {
		for _, f := range zr.File {
			if f.Name == mainClassEntry {
				inspectClass(f)
				break
			}
		}
	}
	for _, f := range zr.File {
		if inspected >= maxClassFilesProbe {
			break
		}
		if f.Name == mainClassEntry {
			continue
		}
		inspectClass(f)
	}
	if maxClassJava > 0 {
		return JarRequirement{
			MinMajor:        maxClassJava,
			LegacyMinecraft: maxClassJava <= 8,
			Source:          "classfile",
		}, true
	}

	// 5. Manifest Build-Jdk-Spec / Build-Jdk fallback.
	for _, key := range []string{"build-jdk-spec", "build-jdk"} {
		if raw := strings.TrimSpace(manifestAttrs[key]); raw != "" {
			if maj, _, _, _, ok := parseNumericJavaVersion(raw); ok && maj >= 8 {
				return JarRequirement{
					MinMajor:        maj,
					LegacyMinecraft: maj <= 8,
					Source:          "manifest",
				}, true
			}
		}
	}

	return JarRequirement{}, false
}

func extractBundlerVersionDir(entryPath string) string {
	const prefix = "META-INF/versions/"
	idx := strings.Index(strings.ToLower(entryPath), strings.ToLower(prefix))
	if idx == -1 {
		return ""
	}
	rest := entryPath[idx+len(prefix):]
	seg, _, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	if _, _, valid := MCVersionMinJava(seg); valid {
		return seg
	}
	return ""
}

func readZipEntryLimit(f *zip.File, limit int64) ([]byte, bool) {
	rc, err := f.Open()
	if err != nil {
		return nil, false
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, false
	}
	return data, true
}

func parseVersionJSONFile(f *zip.File) (JarRequirement, bool) {
	data, ok := readZipEntryLimit(f, 1<<20)
	if !ok {
		return JarRequirement{}, false
	}
	var doc versionJSONDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return JarRequirement{}, false
	}
	id := strings.TrimSpace(doc.ID)
	mappedMin, legacy, mappedOK := MCVersionMinJava(id)
	if doc.JavaVersion > 0 {
		return JarRequirement{
			MinMajor:        doc.JavaVersion,
			LegacyMinecraft: legacy,
			MCVersion:       id,
			Source:          "version.json",
		}, true
	}
	if mappedOK {
		return JarRequirement{
			MinMajor:        mappedMin,
			LegacyMinecraft: legacy,
			MCVersion:       id,
			Source:          "version.json",
		}, true
	}
	return JarRequirement{}, false
}

func parsePatchPropertiesFile(f *zip.File) (JarRequirement, bool) {
	data, ok := readZipEntryLimit(f, 256<<10)
	if !ok {
		return JarRequirement{}, false
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		k, v, hasEq := strings.Cut(line, "=")
		if !hasEq || strings.TrimSpace(k) != "version" {
			continue
		}
		ver := strings.TrimSpace(v)
		if minMajor, legacy, ok := MCVersionMinJava(ver); ok {
			return JarRequirement{
				MinMajor:        minMajor,
				LegacyMinecraft: legacy,
				MCVersion:       ver,
				Source:          "patch.properties",
			}, true
		}
	}
	return JarRequirement{}, false
}

func readManifestAttributes(zr *zip.Reader) map[string]string {
	attrs := make(map[string]string)
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, "META-INF/MANIFEST.MF") {
			data, ok := readZipEntryLimit(f, 256<<10)
			if !ok {
				return attrs
			}
			raw := strings.ReplaceAll(string(data), "\r\n", "\n")
			raw = strings.ReplaceAll(raw, "\n ", "")
			for _, line := range strings.Split(raw, "\n") {
				k, v, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				attrs[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
			return attrs
		}
	}
	return attrs
}

// MCVersionMinJava maps a Minecraft Java Edition version string (e.g. "1.21.4", "1.20.4-R0.1-SNAPSHOT", "1.16.5")
// to (minJavaMajor, legacyMinecraft, ok).
func MCVersionMinJava(ver string) (minMajor int, legacy bool, ok bool) {
	clean := strings.TrimSpace(ver)
	if clean == "" {
		return 0, false, false
	}
	for _, sep := range []string{"-", "+", " "} {
		if before, _, found := strings.Cut(clean, sep); found {
			clean = before
		}
	}
	if len(clean) >= 4 && clean[2] == 'w' {
		yy, err := strconv.Atoi(clean[:2])
		if err == nil {
			switch {
			case yy >= 24:
				return 21, false, true
			case yy >= 22:
				return 17, false, true
			case yy == 21:
				return 16, false, true
			default:
				return 8, true, true
			}
		}
	}

	parts := strings.Split(clean, ".")
	if len(parts) < 2 {
		return 0, false, false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, false, false
	}
	var pat int
	if len(parts) >= 3 {
		pat, _ = strconv.Atoi(parts[2])
	}

	if maj >= 2 {
		return 21, false, true
	}
	if maj != 1 {
		return 0, false, false
	}
	switch {
	case min >= 21:
		return 21, false, true
	case min == 20:
		if pat >= 5 {
			return 21, false, true
		}
		return 17, false, true
	case min >= 18:
		return 17, false, true
	case min == 17:
		return 16, false, true
	case min > 0:
		return 8, true, true
	default:
		return 0, false, false
	}
}

// ParseJavaVersionOutput parses `java -version` output into (major, minor, patch, versionStr, ok).
func ParseJavaVersionOutput(raw string) (major, minor, patch int, versionStr string, ok bool) {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "unable to locate a java runtime") {
		return 0, 0, 0, "", false
	}
	for _, line := range strings.Split(raw, "\n") {
		if !strings.Contains(strings.ToLower(line), "version") {
			continue
		}
		firstQ := strings.IndexByte(line, '"')
		if firstQ == -1 {
			continue
		}
		rest := line[firstQ+1:]
		secondQ := strings.IndexByte(rest, '"')
		if secondQ <= 0 {
			continue
		}
		ver := strings.TrimSpace(rest[:secondQ])
		if maj, min, pat, _, parsed := parseNumericJavaVersion(ver); parsed {
			return maj, min, pat, ver, true
		}
	}
	return 0, 0, 0, "", false
}

func parseNumericJavaVersion(ver string) (major, minor, patch int, clean string, ok bool) {
	clean = strings.TrimSpace(ver)
	core := clean
	for _, sep := range []string{"-", "+"} {
		if before, _, found := strings.Cut(core, sep); found {
			core = before
		}
	}
	tokens := strings.FieldsFunc(core, func(r rune) bool {
		return r == '.' || r == '_'
	})
	if len(tokens) == 0 {
		return 0, 0, 0, "", false
	}
	var nums []int
	for _, tok := range tokens {
		n, err := strconv.Atoi(tok)
		if err != nil {
			break
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return 0, 0, 0, "", false
	}
	if nums[0] == 1 && len(nums) >= 2 && nums[1] > 0 {
		major = nums[1]
		if len(nums) >= 3 {
			minor = nums[2]
		}
		if len(nums) >= 4 {
			patch = nums[3]
		}
		return major, minor, patch, clean, true
	}
	major = nums[0]
	if len(nums) >= 2 {
		minor = nums[1]
	}
	if len(nums) >= 3 {
		patch = nums[2]
	}
	return major, minor, patch, clean, major > 0
}

// DiscoverJavaInstalls scans JAVA_HOME, PATH, and standard OS JVM directories for working Java binaries.
func DiscoverJavaInstalls(ctx context.Context, opts DiscoverOptions) []JavaInstall {
	if len(opts.Installs) > 0 {
		return slices.Clone(opts.Installs)
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	prober := opts.Prober
	if prober == nil {
		prober = defaultJavaProber
	}

	binName := "java"
	if goos == "windows" {
		binName = "java.exe"
	}

	type candidate struct {
		path   string
		source string
	}
	var candidates []candidate
	for _, p := range opts.Candidates {
		if t := strings.TrimSpace(p); t != "" {
			candidates = append(candidates, candidate{path: t, source: "custom"})
		}
	}
	if jh := strings.TrimSpace(getenv("JAVA_HOME")); jh != "" {
		candidates = append(candidates, candidate{
			path:   filepath.Join(jh, "bin", binName),
			source: "java_home",
		})
	}
	if pathEnv := getenv("PATH"); pathEnv != "" {
		for _, dir := range filepath.SplitList(pathEnv) {
			if dir = strings.TrimSpace(dir); dir != "" {
				candidates = append(candidates, candidate{
					path:   filepath.Join(dir, binName),
					source: "path",
				})
			}
		}
	}
	for _, pattern := range osJavaCandidatePatterns(goos, getenv) {
		if strings.ContainsAny(pattern, "*?[") {
			if matches, err := filepath.Glob(pattern); err == nil {
				for _, m := range matches {
					candidates = append(candidates, candidate{path: m, source: "system"})
				}
			}
		} else {
			candidates = append(candidates, candidate{path: pattern, source: "system"})
		}
	}

	seen := make(map[string]struct{})
	var installs []JavaInstall
	for _, cand := range candidates {
		if ctx.Err() != nil {
			break
		}
		info, err := os.Stat(cand.path)
		if err != nil || info.IsDir() {
			continue
		}
		if goos != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		abs, err := filepath.Abs(cand.path)
		if err != nil {
			abs = filepath.Clean(cand.path)
		}
		canon := abs
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			canon = resolved
		}
		if _, dup := seen[canon]; dup {
			continue
		}
		seen[canon] = struct{}{}

		rawOut, pErr := prober(ctx, abs)
		if pErr != nil && strings.TrimSpace(rawOut) == "" {
			continue
		}
		maj, min, pat, rawVer, ok := ParseJavaVersionOutput(rawOut)
		if !ok {
			continue
		}
		installs = append(installs, JavaInstall{
			Path:       abs,
			Major:      maj,
			Minor:      min,
			Patch:      pat,
			RawVersion: rawVer,
			Source:     cand.source,
		})
	}
	return installs
}

func defaultJavaProber(ctx context.Context, binPath string) (string, error) {
	pCtx, cancel := context.WithTimeout(ctx, javaProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(pCtx, binPath, "-version").CombinedOutput()
	return string(out), err
}

func osJavaCandidatePatterns(goos string, getenv func(string) string) []string {
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" {
		home = strings.TrimSpace(getenv("USERPROFILE"))
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	var raw []string
	switch goos {
	case "linux":
		raw = []string{
			"/usr/lib/jvm/*/bin/java",
			"/usr/lib/jvm/*/jre/bin/java",
			"/usr/lib64/jvm/*/bin/java",
			"/usr/java/*/bin/java",
			"/opt/java/*/bin/java",
			"/opt/jdk*/bin/java",
			"/opt/jvm/*/bin/java",
			"/run/current-system/sw/bin/java",
		}
		if home != "" {
			raw = append(raw,
				filepath.Join(home, ".sdkman/candidates/java/*/bin/java"),
				filepath.Join(home, ".jdks/*/bin/java"),
				filepath.Join(home, ".asdf/installs/java/*/bin/java"),
				filepath.Join(home, ".local/share/mise/installs/java/*/bin/java"),
				filepath.Join(home, ".nix-profile/bin/java"),
			)
		}
	case "darwin":
		raw = []string{
			"/Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java",
			"/opt/homebrew/opt/openjdk*/bin/java",
			"/opt/homebrew/opt/openjdk*/libexec/openjdk.jdk/Contents/Home/bin/java",
			"/usr/local/opt/openjdk*/bin/java",
			"/usr/local/opt/openjdk*/libexec/openjdk.jdk/Contents/Home/bin/java",
		}
		if home != "" {
			raw = append(raw,
				filepath.Join(home, "Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java"),
				filepath.Join(home, ".jdks/*/Contents/Home/bin/java"),
				filepath.Join(home, ".jdks/*/bin/java"),
				filepath.Join(home, ".sdkman/candidates/java/*/bin/java"),
			)
		}
	case "windows":
		for _, pf := range []string{getenv("ProgramFiles"), getenv("ProgramFiles(x86)"), `C:\Program Files`} {
			pf = strings.TrimSpace(pf)
			if pf == "" {
				continue
			}
			for _, vendor := range []string{"Java", "Eclipse Adoptium", "AdoptOpenJDK", "Microsoft", "Zulu", "BellSoft", "Amazon Corretto"} {
				raw = append(raw, filepath.Join(pf, vendor, "*", "bin", "java.exe"))
			}
		}
		if home != "" {
			raw = append(raw, filepath.Join(home, ".jdks", "*", "bin", "java.exe"))
		}
		if localApp := strings.TrimSpace(getenv("LOCALAPPDATA")); localApp != "" {
			raw = append(raw, filepath.Join(localApp, "Programs", "Eclipse Adoptium", "*", "bin", "java.exe"))
		}
	}
	return raw
}

// SelectBestJava chooses the best compatible Java installation from installs for req.
func SelectBestJava(req JarRequirement, installs []JavaInstall) (JavaInstall, error) {
	minMajor := req.MinMajor
	if minMajor <= 0 {
		minMajor = 8
	}

	var compatible []JavaInstall
	for _, inst := range installs {
		if inst.Major >= minMajor {
			compatible = append(compatible, inst)
		}
	}
	if len(compatible) == 0 {
		var found []string
		for _, inst := range installs {
			found = append(found, fmt.Sprintf("Java %d (%s at %s)", inst.Major, inst.RawVersion, inst.Path))
		}
		mcNote := ""
		if req.MCVersion != "" {
			mcNote = fmt.Sprintf(" for Minecraft %s", req.MCVersion)
		}
		if len(found) == 0 {
			return JavaInstall{}, fmt.Errorf("no Java runtime found; server JAR requires Java %d+%s (detected via %s). Install Java %d+ or set java_path",
				minMajor, mcNote, req.Source, minMajor)
		}
		return JavaInstall{}, fmt.Errorf("no compatible Java runtime found; server JAR requires Java %d+%s (detected via %s), found only: %s. Install Java %d+ or set java_path",
			minMajor, mcNote, req.Source, strings.Join(found, ", "), minMajor)
	}

	slices.SortStableFunc(compatible, func(a, b JavaInstall) int {
		aTier, aMajScore := selectionRank(req, minMajor, a.Major)
		bTier, bMajScore := selectionRank(req, minMajor, b.Major)
		if aTier != bTier {
			return aTier - bTier
		}
		if aMajScore != bMajScore {
			return aMajScore - bMajScore
		}
		if a.Minor != b.Minor {
			return b.Minor - a.Minor
		}
		if a.Patch != b.Patch {
			return b.Patch - a.Patch
		}
		return javaSourceRank(a.Source) - javaSourceRank(b.Source)
	})
	return compatible[0], nil
}

func selectionRank(req JarRequirement, minMajor, major int) (tier, majorScore int) {
	if req.LegacyMinecraft {
		switch {
		case major == 8:
			return 0, major
		case major == 11:
			return 1, major
		case major == 16 || major == 17:
			return 2, major
		case major < 18:
			return 3, major
		default:
			return 4, major
		}
	}
	if major == minMajor {
		return 0, major
	}
	if isJavaLTS(major) {
		return 1, major
	}
	return 2, major
}

func isJavaLTS(major int) bool {
	switch major {
	case 8, 11, 17, 21, 25:
		return true
	default:
		return false
	}
}

func javaSourceRank(source string) int {
	switch strings.ToLower(source) {
	case "custom":
		return 0
	case "java_home":
		return 1
	case "path":
		return 2
	case "system":
		return 3
	default:
		return 4
	}
}

// ResolveJavaForJar inspects jarPath and either validates explicitJavaPath or automatically discovers
// and selects the best compatible Java runtime.
func ResolveJavaForJar(ctx context.Context, jarPath, explicitJavaPath string, opts DiscoverOptions) (SelectionResult, error) {
	req, err := InspectServerJar(jarPath)
	if err != nil {
		return SelectionResult{}, err
	}

	if explicit := strings.TrimSpace(explicitJavaPath); explicit != "" {
		resolved := explicit
		if !strings.ContainsRune(explicit, '/') && !strings.ContainsRune(explicit, '\\') {
			if lp, lpErr := exec.LookPath(explicit); lpErr == nil {
				resolved = lp
			} else {
				return SelectionResult{}, fmt.Errorf("configured java_path %q not found in PATH: %w", explicit, lpErr)
			}
		} else if _, stErr := os.Stat(resolved); stErr != nil {
			return SelectionResult{}, fmt.Errorf("configured java_path %q does not exist: %w", resolved, stErr)
		}
		abs, err := filepath.Abs(resolved)
		if err == nil {
			resolved = abs
		}
		prober := opts.Prober
		if prober == nil {
			prober = defaultJavaProber
		}
		inst := JavaInstall{Path: resolved, Source: "explicit"}
		if rawOut, _ := prober(ctx, resolved); rawOut != "" {
			if maj, min, pat, rawVer, ok := ParseJavaVersionOutput(rawOut); ok {
				inst.Major = maj
				inst.Minor = min
				inst.Patch = pat
				inst.RawVersion = rawVer
			}
		}
		var warning string
		if inst.Major > 0 && inst.Major < req.MinMajor {
			warning = fmt.Sprintf("configured java_path %s reports Java %d, but %s requires Java %d+ (detected via %s)",
				resolved, inst.Major, filepath.Base(jarPath), req.MinMajor, req.Source)
		}
		return SelectionResult{Java: inst, Requirement: req, Warning: warning}, nil
	}

	installs := DiscoverJavaInstalls(ctx, opts)
	best, err := SelectBestJava(req, installs)
	if err != nil {
		return SelectionResult{}, err
	}
	return SelectionResult{Java: best, Requirement: req}, nil
}
