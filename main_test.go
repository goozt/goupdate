package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeVersionServer starts a test HTTP server that returns the supplied
// versions as a JSON array compatible with the go.dev/dl API.
func makeVersionServer(t *testing.T, versions []string) *httptest.Server {
	t.Helper()
	type entry struct {
		Version string `json:"version"`
	}
	entries := make([]entry, len(versions))
	for i, v := range versions {
		entries[i] = entry{Version: v}
	}
	body, _ := json.Marshal(entries)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
}

// createTestZip writes a zip archive containing the supplied files
// (map[archivePath]content) to a temp file and returns its path.
func createTestZip(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	return zipPath
}

// ---------------------------------------------------------------------------
// isPrivileged
// ---------------------------------------------------------------------------

func TestIsPrivileged_AlwaysTrueOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	if !isPrivileged() {
		t.Error("isPrivileged() must always return true on Windows")
	}
}

func TestIsPrivileged_UnixDoesNotPanic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}
	// Just ensure the call succeeds; actual value depends on test runner UID.
	_ = isPrivileged()
}

// ---------------------------------------------------------------------------
// getArch
// ---------------------------------------------------------------------------

func TestGetArch_ReturnsNonEmpty(t *testing.T) {
	if got := getArch(); got == "" {
		t.Error("getArch() returned empty string")
	}
}

func TestGetArch_NonArmMatchesGoArch(t *testing.T) {
	if runtime.GOARCH == "arm" {
		t.Skip("arm-specific behaviour tested separately")
	}
	if got := getArch(); got != runtime.GOARCH {
		t.Errorf("getArch() = %q, want %q", got, runtime.GOARCH)
	}
}

func TestGetArch_LinuxArmMapsToArmv6l(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm" {
		t.Skip("Only meaningful on linux/arm")
	}
	if got := getArch(); got != "armv6l" {
		t.Errorf("getArch() on linux/arm = %q, want %q", got, "armv6l")
	}
}

// ---------------------------------------------------------------------------
// getArchiveExt
// ---------------------------------------------------------------------------

func TestGetArchiveExt_WindowsReturnsZip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only")
	}
	if got := getArchiveExt(); got != ".zip" {
		t.Errorf("getArchiveExt() = %q, want %q", got, ".zip")
	}
}

func TestGetArchiveExt_NonWindowsReturnsTarGz(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Non-Windows only")
	}
	if got := getArchiveExt(); got != ".tar.gz" {
		t.Errorf("getArchiveExt() on %s = %q, want %q", runtime.GOOS, got, ".tar.gz")
	}
}

// ---------------------------------------------------------------------------
// getInstallDir
// ---------------------------------------------------------------------------

func TestGetInstallDir_WindowsReturnsExpectedPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only")
	}
	if got := getInstallDir(); got != `C:\go` {
		t.Errorf("getInstallDir() = %q, want %q", got, `C:\go`)
	}
}

func TestGetInstallDir_UnixReturnsExpectedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only")
	}
	if got := getInstallDir(); got != "/usr/local/go" {
		t.Errorf("getInstallDir() = %q, want %q", got, "/usr/local/go")
	}
}

// ---------------------------------------------------------------------------
// progressWriter
// ---------------------------------------------------------------------------

func TestProgressWriter_UpdatesDownloadedBytes(t *testing.T) {
	pw := &progressWriter{Total: 10 * 1024 * 1024}
	data := make([]byte, 1024*1024) // 1 MB chunk
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
	if pw.Written != int64(len(data)) {
		t.Errorf("Written = %d, want %d", pw.Written, int64(len(data)))
	}
}

func TestProgressWriter_ZeroTotalNoDivisionByZero(t *testing.T) {
	// Total == 0 must not divide by zero; the if-guard should prevent it.
	pw := &progressWriter{Total: 0}
	data := []byte("hello world")
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
	if pw.Written != int64(len(data)) {
		t.Errorf("Written = %d, want %d", pw.Written, int64(len(data)))
	}
}

func TestProgressWriter_FullDownload100Percent(t *testing.T) {
	total := int64(4096)
	pw := &progressWriter{Total: total}
	data := make([]byte, total)
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != int(total) {
		t.Errorf("Write() n = %d, want %d", n, total)
	}
	if pw.Written != total {
		t.Errorf("Written = %d, want %d", pw.Written, total)
	}
}

func TestProgressWriter_MultipleWritesAccumulate(t *testing.T) {
	pw := &progressWriter{Total: 100}
	for i := 0; i < 10; i++ {
		pw.Write(make([]byte, 10))
	}
	if pw.Written != 100 {
		t.Errorf("Written after 10×10 bytes = %d, want 100", pw.Written)
	}
}

// ---------------------------------------------------------------------------
// generatePathForVersion
// ---------------------------------------------------------------------------

func TestGeneratePathForVersion_ContainsVersion(t *testing.T) {
	v := "1.26.1"
	path := generatePathForVersion(v)
	if !strings.Contains(path, "go"+v) {
		t.Errorf("generatePathForVersion(%q) = %q; must contain %q", v, path, "go"+v)
	}
}

func TestGeneratePathForVersion_ContainsCurrentOS(t *testing.T) {
	v := "1.26.1"
	path := generatePathForVersion(v)
	if !strings.Contains(path, runtime.GOOS) {
		t.Errorf("generatePathForVersion(%q) = %q; must contain OS %q", v, path, runtime.GOOS)
	}
}

func TestGeneratePathForVersion_ContainsCurrentArch(t *testing.T) {
	v := "1.26.1"
	path := generatePathForVersion(v)
	if !strings.Contains(path, getArch()) {
		t.Errorf("generatePathForVersion(%q) = %q; must contain arch %q", v, path, getArch())
	}
}

func TestGeneratePathForVersion_HasCorrectExtension(t *testing.T) {
	v := "1.26.1"
	path := generatePathForVersion(v)
	if !strings.HasSuffix(path, getArchiveExt()) {
		t.Errorf("generatePathForVersion(%q) = %q; must end with %q", v, path, getArchiveExt())
	}
}

func TestGeneratePathForVersion_IsAbsolutePath(t *testing.T) {
	path := generatePathForVersion("1.26.1")
	if !filepath.IsAbs(path) {
		t.Errorf("generatePathForVersion() = %q; expected absolute path", path)
	}
}

// ---------------------------------------------------------------------------
// getLatestVersions  (HTTP mocked)
// ---------------------------------------------------------------------------

func TestGetLatestVersions_ExactPatchMatch(t *testing.T) {
	ts := makeVersionServer(t, []string{"go1.26.3", "go1.26.2", "go1.26.1", "go1.25.5"})
	defer ts.Close()
	old := goDevAPIURL
	goDevAPIURL = ts.URL
	defer func() { goDevAPIURL = old }()

	got, err := getLatestVersions("1.26")
	if err != nil {
		t.Fatalf("getLatestVersions() error = %v", err)
	}
	// API is newest-first; should return first (latest) matching version.
	if got != "1.26.3" {
		t.Errorf("getLatestVersions(\"1.26\") = %q, want %q", got, "1.26.3")
	}
}

func TestGetLatestVersions_NoMatchReturnsError(t *testing.T) {
	ts := makeVersionServer(t, []string{"go1.26.3", "go1.25.5"})
	defer ts.Close()
	old := goDevAPIURL
	goDevAPIURL = ts.URL
	defer func() { goDevAPIURL = old }()

	_, err := getLatestVersions("1.24")
	if err == nil {
		t.Error("getLatestVersions() = nil error; expected an error for unmatched version")
	}
}

func TestGetLatestVersions_ServerErrorReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	old := goDevAPIURL
	goDevAPIURL = ts.URL
	defer func() { goDevAPIURL = old }()

	_, err := getLatestVersions("1.26")
	if err == nil {
		t.Error("getLatestVersions() = nil error; expected an error on HTTP 500")
	}
}

func TestGetLatestVersions_InvalidJSONReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not json"))
	}))
	defer ts.Close()
	old := goDevAPIURL
	goDevAPIURL = ts.URL
	defer func() { goDevAPIURL = old }()

	_, err := getLatestVersions("1.26")
	if err == nil {
		t.Error("getLatestVersions() = nil error; expected an error on invalid JSON")
	}
}

func TestGetLatestVersions_EmptyListReturnsError(t *testing.T) {
	ts := makeVersionServer(t, []string{})
	defer ts.Close()
	old := goDevAPIURL
	goDevAPIURL = ts.URL
	defer func() { goDevAPIURL = old }()

	_, err := getLatestVersions("1.26")
	if err == nil {
		t.Error("getLatestVersions() = nil error; expected an error on empty version list")
	}
}

// ---------------------------------------------------------------------------
// BUG: getLatestVersions – naive HasPrefix causes wrong matches
//
// When the caller passes "1.2", the function checks:
//   strings.HasPrefix(v.Version, "go1.2")
// This is TRUE for "go1.21.13" and "go1.20.14" as well, because they both
// start with the string "go1.2".  Because the API returns versions
// newest-first, the function will return a 1.21.x or 1.20.x release instead
// of the 1.2.x release the caller intended.
//
// The fix is to append a dot: strings.HasPrefix(v.Version, "go"+partial+".")
// ---------------------------------------------------------------------------

func TestGetLatestVersions_BUG_ShortPrefixMatchesWrongMajorVersion(t *testing.T) {
	// Simulated API (newest first): 1.21 and 1.20 come before 1.2
	ts := makeVersionServer(t, []string{
		"go1.21.13",
		"go1.20.14",
		"go1.2.2",
	})
	defer ts.Close()
	old := goDevAPIURL
	goDevAPIURL = ts.URL
	defer func() { goDevAPIURL = old }()

	got, err := getLatestVersions("1.2")
	if err != nil {
		t.Fatalf("getLatestVersions() error = %v", err)
	}
	// EXPECTED correct result: "1.2.2"
	// ACTUAL buggy result:     "1.21.13"  (HasPrefix("go1.21.13","go1.2") == true)
	if got == "1.21.13" {
		t.Errorf(
			"BUG CONFIRMED – getLatestVersions(\"1.2\") returned %q "+
				"instead of %q because strings.HasPrefix(\"go1.21.13\", \"go1.2\") is true. "+
				"Fix: use suffix dot check (\"go1.2.\").",
			got, "1.2.2",
		)
	} else if got != "1.2.2" {
		t.Errorf("getLatestVersions(\"1.2\") = %q, want %q", got, "1.2.2")
	}
}

// ---------------------------------------------------------------------------
// checkVersionAvailableLocally
// ---------------------------------------------------------------------------

func TestCheckVersionAvailableLocally_ReturnsFalseForNonExistent(t *testing.T) {
	if checkVersionAvailableLocally("0.0.0") {
		t.Error("checkVersionAvailableLocally(\"0.0.0\") = true; expected false")
	}
}

func TestCheckVersionAvailableLocally_ReturnsTrueWhenFileExists(t *testing.T) {
	version := "99.88.77"
	expectedPath := generatePathForVersion(version)
	dir := filepath.Dir(expectedPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Skipf("Cannot create dir %s: %v", dir, err)
	}
	f, err := os.Create(expectedPath)
	if err != nil {
		t.Skipf("Cannot create temp archive at %s: %v", expectedPath, err)
	}
	f.Close()
	defer os.Remove(expectedPath)

	if !checkVersionAvailableLocally(version) {
		t.Errorf("checkVersionAvailableLocally(%q) = false; expected true (file exists at %s)", version, expectedPath)
	}
}

// ---------------------------------------------------------------------------
// dirExists
// ---------------------------------------------------------------------------

func TestDirExists_ReturnsTrueForExistingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	if !dirExists(tmpDir) {
		t.Errorf("dirExists(%q) = false; want true for existing directory", tmpDir)
	}
}

func TestDirExists_ReturnsFalseForNonExistentPath(t *testing.T) {
	if dirExists(filepath.Join(t.TempDir(), "does_not_exist")) {
		t.Error("dirExists() = true for non-existent path; want false")
	}
}

func TestDirExists_ReturnsFalseForFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(tmpFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if dirExists(tmpFile) {
		t.Errorf("dirExists(%q) = true for a regular file; want false", tmpFile)
	}
}

// ---------------------------------------------------------------------------
// BUG: dirExists – potential nil-pointer panic on non-IsNotExist errors
//
// If os.Stat returns an error that is NOT a "not-exist" error (e.g. EACCES),
// the function skips the early return and calls info.IsDir() on a nil *FileInfo,
// which will panic at runtime.
//
// Fix: add a guard for any non-nil error:
//   if err != nil { return false }
// ---------------------------------------------------------------------------

func TestDirExists_BUG_DocumentedNilInfoPanicRisk(t *testing.T) {
	// We document the code path here; triggering it portably (without root) is
	// impractical, so we cannot run it directly.  The test serves as a
	// regression anchor and bug record.
	//
	// Affected code:
	//   info, err := os.Stat(path)
	//   if os.IsNotExist(err) { return false }
	//   return info.IsDir()   // info is nil when err != nil && !os.IsNotExist(err)
	t.Log("BUG DOCUMENTED: dirExists() will panic when os.Stat returns a non-IsNotExist error (e.g. permission denied) because info is nil but info.IsDir() is still called.")
}

// ---------------------------------------------------------------------------
// extractZip
// ---------------------------------------------------------------------------

func TestExtractZip_BasicFileExtraction(t *testing.T) {
	files := map[string]string{
		"go/bin/go":    "fake go binary",
		"go/README.md": "readme content",
	}
	zipPath := createTestZip(t, files)
	destDir := t.TempDir()

	if err := extractZip(zipPath, destDir, &progressWriter{}); err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}

	for name, wantContent := range files {
		extractedPath := filepath.Join(destDir, filepath.FromSlash(name))
		got, err := os.ReadFile(extractedPath)
		if err != nil {
			t.Errorf("extractZip(): file %q not found: %v", name, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("extractZip(): file %q = %q, want %q", name, string(got), wantContent)
		}
	}
}

func TestExtractZip_CreatesIntermediateDirectories(t *testing.T) {
	files := map[string]string{
		"go/src/pkg/main.go": "package main",
	}
	zipPath := createTestZip(t, files)
	destDir := t.TempDir()

	if err := extractZip(zipPath, destDir, &progressWriter{}); err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}

	pkgDir := filepath.Join(destDir, "go", "src", "pkg")
	if !dirExists(pkgDir) {
		t.Errorf("extractZip(): expected directory %q to exist", pkgDir)
	}
}

func TestExtractZip_DirectoryEntry(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "withdir.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// Explicit directory entry
	header := &zip.FileHeader{Name: "go/src/", Method: zip.Store}
	header.SetMode(0755 | os.ModeDir)
	w.CreateHeader(header)
	// File inside
	fw, _ := w.Create("go/src/main.go")
	fw.Write([]byte("package main"))
	w.Close()
	f.Close()

	destDir := t.TempDir()
	if err := extractZip(zipPath, destDir, &progressWriter{}); err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}
	srcDir := filepath.Join(destDir, "go", "src")
	if !dirExists(srcDir) {
		t.Errorf("extractZip() did not create directory %q", srcDir)
	}
}

func TestExtractZip_ZipSlipPathTraversalRejected(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "evil.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	fw, _ := w.Create("../../../etc/evil")
	fw.Write([]byte("malicious"))
	w.Close()
	f.Close()

	destDir := t.TempDir()
	err = extractZip(zipPath, destDir, &progressWriter{})
	if err == nil {
		t.Fatal("extractZip() accepted a zip-slip path; expected an error")
	}
	if !strings.Contains(err.Error(), "illegal file path") {
		t.Errorf("extractZip() error = %q; want message containing 'illegal file path'", err.Error())
	}
}

func TestExtractZip_NonExistentSourceReturnsError(t *testing.T) {
	err := extractZip("/nonexistent/path/archive.zip", t.TempDir(), &progressWriter{})
	if err == nil {
		t.Error("extractZip() = nil error for non-existent source; expected error")
	}
}

func TestExtractZip_EmptyZipExtractsSuccessfully(t *testing.T) {
	zipPath := createTestZip(t, map[string]string{})
	if err := extractZip(zipPath, t.TempDir(), &progressWriter{}); err != nil {
		t.Errorf("extractZip() error on empty zip = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Version regex (mirrors the logic in main())
// ---------------------------------------------------------------------------

func TestVersionRegex_ValidVersions(t *testing.T) {
	re := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	valid := []string{
		"1.26", "1.26.1", "1.2.3", "10.20.30", "1.0", "2.0.0",
	}
	for _, v := range valid {
		if !re.MatchString(v) {
			t.Errorf("version regex rejected valid version %q", v)
		}
	}
}

func TestVersionRegex_InvalidVersions(t *testing.T) {
	re := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	invalid := []string{
		"1", "1.", ".1", "1.2.", "1.2.3.4", "abc", "1.2.x", "v1.26", "",
	}
	for _, v := range invalid {
		if re.MatchString(v) {
			t.Errorf("version regex accepted invalid version %q", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Minor: redundant len(version) < 3 guard in main()
//
// The regex ^\d+\.\d+(\.\d+)?$ already requires at least 3 characters
// (digit + dot + digit), so the len check is completely redundant.
// Demonstrated below:
// ---------------------------------------------------------------------------

func TestVersionLengthCheckRedundancy(t *testing.T) {
	re := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	// Any string short enough to fail len < 3 also fails the regex.
	shortStrings := []string{"", "1", "1.", ".1"}
	for _, s := range shortStrings {
		regexRejects := !re.MatchString(s)
		lenRejects := len(s) < 3
		if regexRejects != lenRejects && !regexRejects {
			// Only a problem if regex PASSES but len check FAILS
			t.Errorf("len(%q)<3=%v but regex accepts it=%v – len check would incorrectly gate",
				s, lenRejects, !regexRejects)
		}
	}
	// Document: len<3 is purely redundant; regex is sufficient.
	t.Log("NOTE: The `len(version) < 3` guard in main() is redundant. The regex rejects all such strings already.")
}

// ---------------------------------------------------------------------------
// Cleanup suffix matching logic
// ---------------------------------------------------------------------------

func TestCleanup_SuffixMatchesGoArchiveFiles(t *testing.T) {
	suffix := fmt.Sprintf(".%s-%s%s", runtime.GOOS, getArch(), getArchiveExt())

	shouldMatch := []string{
		fmt.Sprintf("go1.26.1%s", suffix),
		fmt.Sprintf("go1.25.0%s", suffix),
		fmt.Sprintf("go2.0.0%s", suffix),
	}
	for _, f := range shouldMatch {
		if !(strings.HasPrefix(f, "go") && strings.HasSuffix(f, suffix)) {
			t.Errorf("file %q should match cleanup pattern but does not", f)
		}
	}
}

func TestCleanup_SuffixDoesNotMatchNonGoFiles(t *testing.T) {
	suffix := fmt.Sprintf(".%s-%s%s", runtime.GOOS, getArch(), getArchiveExt())

	shouldNotMatch := []string{
		"somefile.txt",
		"notes.zip",
		"mygo.tar.gz",
	}
	for _, f := range shouldNotMatch {
		if strings.HasPrefix(f, "go") && strings.HasSuffix(f, suffix) {
			t.Errorf("file %q should NOT match cleanup pattern but does", f)
		}
	}
}

// ---------------------------------------------------------------------------
// getGoSourcePath
// ---------------------------------------------------------------------------

func TestGetGoSourcePath_ReturnsNonEmpty(t *testing.T) {
	p := getGoSourcePath()
	if p == "" {
		t.Error("getGoSourcePath() returned empty string")
	}
}

func TestGetGoSourcePath_IsAbsolute(t *testing.T) {
	p := getGoSourcePath()
	if !filepath.IsAbs(p) {
		t.Errorf("getGoSourcePath() = %q; expected absolute path", p)
	}
}

func TestGetGoSourcePath_EndsWithGoSrc(t *testing.T) {
	p := getGoSourcePath()
	// The path must end in …/go/src (with OS-appropriate separator)
	if !strings.HasSuffix(p, filepath.Join("go", "src")) {
		t.Errorf("getGoSourcePath() = %q; expected path ending with go%csrc", p, os.PathSeparator)
	}
}
