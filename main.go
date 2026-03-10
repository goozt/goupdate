package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"go/build"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var goupdate_version = "1.2.0"

var goDevAPIURL = "https://go.dev/dl/?mode=json"
var goDevDLBaseURL = "https://golang.org/dl"

// isPrivileged returns true if running as root on Unix, or always true on Windows
// (Windows does not use sudo; privilege is handled by UAC/elevated prompt).
func isPrivileged() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return os.Geteuid() == 0
}

// On Linux, "arm" is mapped to "armv6l".
func getArch() string {
	arch := runtime.GOARCH
	if arch == "arm" && runtime.GOOS == "linux" {
		return "armv6l"
	}
	return arch
}

func getArchiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func getInstallDir() string {
	if runtime.GOOS == "windows" {
		return `C:\go`
	}
	return "/usr/local/go"
}

type progressWriter struct {
	Total   int64
	Written int64
	Label   string
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Written += int64(n)

	if pw.Total > 0 {
		percent := float64(pw.Written) / float64(pw.Total) * 100
		barLength := 20
		completed := min(int(float64(barLength)*(float64(pw.Written)/float64(pw.Total))), barLength)
		var bar string
		if runtime.GOOS == "windows" {
			bar = strings.Repeat("#", completed) + strings.Repeat("-", barLength-completed)
		} else {
			bar = strings.Repeat("█", completed) + strings.Repeat("░", barLength-completed)
		}

		fmt.Printf("\r⌛ %s [%s] %.2f%% (%d/%d MB)",
			pw.Label, bar, percent, pw.Written/1024/1024, pw.Total/1024/1024)
	} else {
		// Content-Length unknown (chunked transfer): show bytes only
		fmt.Printf("\r⌛ %s %d MB", pw.Label, pw.Written/1024/1024)
	}
	return n, nil
}

func getGoSourcePath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "go", "src")
}

func generatePathForVersion(version string) string {
	filename := fmt.Sprintf("go%s.%s-%s%s", version, runtime.GOOS, getArch(), getArchiveExt())
	return filepath.Join(getGoSourcePath(), filename)
}

func getLatestVersions(partialVersion string) (string, error) {
	apiUrl := goDevAPIURL
	resp, err := http.Get(apiUrl)
	if err != nil {
		return "", fmt.Errorf("failed to fetch versions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	var versions []struct {
		Version string `json:"version"`
	}

	if err := json.Unmarshal(body, &versions); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %v", err)
	}

	for _, v := range versions {
		if strings.HasPrefix(v.Version, "go"+partialVersion+".") {
			return strings.TrimPrefix(v.Version, "go"), nil
		}
	}

	return "", fmt.Errorf("no matching version found for %s", partialVersion)
}

func checkVersionAvailableLocally(version string) bool {
	filePath := generatePathForVersion(version)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}
	return true
}

func checkVersionAvailable(version string) error {
	if checkVersionAvailableLocally(version) {
		fmt.Printf("✅ Found go%s in cache\n", version)
		return nil
	}

	fmt.Printf("🟢 Searching go%s online\n", version)
	url := fmt.Sprintf("%s/go%s.%s-%s%s", goDevDLBaseURL, version, runtime.GOOS, getArch(), getArchiveExt())

	var resp *http.Response
	var err error

	client := &http.Client{Transport: &http.Transport{
		Proxy:              http.ProxyFromEnvironment,
		DisableCompression: true,
	}}
	resp, err = client.Get(url)

	if err != nil {
		return fmt.Errorf("failed to fetch version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	outputPath := generatePathForVersion(version)

	os.MkdirAll(filepath.Dir(outputPath), 0755)
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer out.Close()

	fmt.Printf("\033[2K")
	pw := &progressWriter{Total: resp.ContentLength, Label: "Downloading"}
	_, err = io.Copy(io.MultiWriter(out, pw), resp.Body)
	if err != nil {
		out.Close()
		os.Remove(outputPath)
		return fmt.Errorf("failed to save file: %v", err)
	}
	fmt.Println("\033[2K\r✅ Download completed")

	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func uninstallGo(installedDir ...string) error {
	var installDir string
	if len(installedDir) > 0 && strings.TrimSpace(installedDir[0]) != "" {
		installDir = installedDir[0]
	} else {
		installDir = getInstallDir()
	}

	if dirExists(installDir) {
		fmt.Print("⌛ Uninstalling Go...")
		if err := os.RemoveAll(installDir); err != nil {
			// On Unix non-root, fall back to sudo rm
			if runtime.GOOS != "windows" && !isPrivileged() {
				sudoCmd := exec.Command("sudo", "rm", "-rf", installDir)
				if err2 := sudoCmd.Run(); err2 != nil {
					fmt.Printf("❌ Failed to remove old directory: %v\n", err2)
					return fmt.Errorf("failed to remove old directory: %v", err2)
				}
			} else {
				fmt.Printf("❌ Failed to remove old directory: %v\n", err)
				return fmt.Errorf("failed to remove old directory: %v", err)
			}
		}
		fmt.Printf("\r\033[2K✅ Uninstalled Go\n")
	} else {
		fmt.Println("No Go installation found")
	}
	return nil
}

func extractZip(src, destParent string, pw *progressWriter) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("failed to open zip: %v", err)
	}
	defer r.Close()

	var total int64
	for _, f := range r.File {
		total += int64(f.UncompressedSize64)
	}
	pw.Total = total

	parent := filepath.Clean(destParent)
	for _, f := range r.File {
		destPath := filepath.Clean(filepath.Join(parent, f.Name))

		// Guard against zip slip
		rel, err := filepath.Rel(parent, destPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, f.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(io.MultiWriter(outFile, pw), rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(src, destParent string, pw *progressWriter) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open archive: %v", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat archive: %v", err)
	}
	pw.Total = info.Size()

	gr, err := gzip.NewReader(io.TeeReader(f, pw))
	if err != nil {
		return fmt.Errorf("failed to open gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	parent := filepath.Clean(destParent)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %v", err)
		}

		destPath := filepath.Clean(filepath.Join(parent, hdr.Name))

		// Guard against tar slip
		rel, err := filepath.Rel(parent, destPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("illegal file path in tar: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(destPath, os.FileMode(hdr.Mode))
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(outFile, tr)
			outFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func runInstallation(version string) error {
	filePath := generatePathForVersion(version)

	// 1. Check if version exists
	if err := checkVersionAvailable(version); err != nil {
		fmt.Printf("❌ Version go%s not available\n", version)
		return fmt.Errorf("failed to check version availability: %v", err)
	}

	// 2. Remove existing installation
	if err := uninstallGo(); err != nil {
		fmt.Printf("❌ Failed to uninstall old Go version: %v\n", err)
		return fmt.Errorf("failed to uninstall old Go version: %v", err)
	}

	// 3. Extract the new archive
	installParent := filepath.Dir(getInstallDir())
	var extractErr error
	fmt.Printf("\033[2K")
	if runtime.GOOS == "windows" {
		pw := &progressWriter{Label: "Installing"}
		extractErr = extractZip(filePath, installParent, pw)
	} else if isPrivileged() {
		pw := &progressWriter{Label: "Installing"}
		extractErr = extractTar(filePath, installParent, pw)
	} else {
		fmt.Printf("⌛ Installing...")
		tarCmd := exec.Command("sudo", "tar", "-C", installParent, "-xzf", filePath)
		extractErr = tarCmd.Run()
	}
	if extractErr != nil {
		fmt.Printf("\r\033[2K❌ Failed to extract archive: %v\n", extractErr)
		return fmt.Errorf("failed to extract archive: %v", extractErr)
	}
	fmt.Println("\033[2K\r✅ Installation completed")

	// 4. Verify the version
	goBin := filepath.Join(getInstallDir(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBin += ".exe"
	}
	out, err := exec.Command(goBin, "version").Output()
	if err != nil {
		fmt.Printf("\r\033[2K❌ Error checking version: %v\n", err)
		return fmt.Errorf("failed to check version: %v", err)
	}

	// 5. Parse and compare versions
	versionStrs := strings.Fields(string(out))
	if len(versionStrs) < 3 {
		fmt.Printf("\r\033[2K❌ Unexpected version output: %s\n", string(out))
		return fmt.Errorf("unexpected version output")
	}

	// 6. Check if the installed version matches the expected version
	if versionStrs[2] != fmt.Sprintf("go%s", version) {
		fmt.Printf("\r\033[2K❌ Installation failed\n")
		fmt.Printf("❌ Version mismatch. Expected: go%s, Got: %s\n", version, versionStrs[2])
		return fmt.Errorf("version mismatch")
	}

	return nil
}

func addToUnixShellConfigs(paths []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	exportLines := []string{"", "# Added by goupdate"}
	for _, p := range paths {
		exportLines = append(exportLines, fmt.Sprintf("export PATH=$PATH:%s", p))
	}
	block := strings.Join(exportLines, "\n") + "\n"

	candidates := []string{
		filepath.Join(homeDir, ".bashrc"),
		filepath.Join(homeDir, ".zshrc"),
		filepath.Join(homeDir, ".profile"),
	}

	for _, cfg := range candidates {
		if _, err := os.Stat(cfg); os.IsNotExist(err) {
			continue
		}
		// Check if already present to avoid duplicates
		f, err := os.Open(cfg)
		if err != nil {
			continue
		}
		alreadyPresent := false
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			for _, p := range paths {
				if strings.Contains(line, p) {
					alreadyPresent = true
					break
				}
			}
			if alreadyPresent {
				break
			}
		}
		f.Close()

		if alreadyPresent {
			continue
		}

		file, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		file.WriteString(block)
		file.Close()
		fmt.Printf("✅ Updated PATH in %s\n", cfg)
	}
}

func addToWindowsUserPATH(paths []string) {
	for _, p := range paths {
		script := fmt.Sprintf(
			`$old = [Environment]::GetEnvironmentVariable('Path','User'); `+
				`if ($old -notlike '*%s*') { `+
				`[Environment]::SetEnvironmentVariable('Path', $old+';%s', 'User') }`,
			p, p,
		)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
		if err := cmd.Run(); err != nil {
			fmt.Printf("⚠️  Could not update PATH for %s: %v\n", p, err)
		} else {
			fmt.Printf("✅ Added %s to user PATH\n", p)
		}
	}
}

func ensurePATH() {
	installBin := filepath.Join(getInstallDir(), "bin")
	gopathBin := filepath.Join(build.Default.GOPATH, "bin")
	currentPath := os.Getenv("PATH")

	var missing []string
	for _, p := range []string{installBin, gopathBin} {
		if !strings.Contains(currentPath, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return
	}

	fmt.Printf("⚠️  Some Go paths are not in your PATH. Adding them now...")
	time.Sleep(time.Millisecond * 200)
	fmt.Printf("\r\033[2K")
	if runtime.GOOS == "windows" {
		addToWindowsUserPATH(missing)
	} else {
		addToUnixShellConfigs(missing)
	}
	fmt.Println("   Restart your shell or source your config file to apply.")
}

func showVersion() {
	fmt.Printf("goupdate version v%s\n\nSource: https://github.com/goozt/goupdate\nAuthor: Nikhil John\nLicense: MIT\n", goupdate_version)
}

func showHelp() {
	fmt.Println("Usage: goupdate <command>|<version>")
	fmt.Println("\nCommands:")
	fmt.Println("  version     Show goupdate tool version")
	fmt.Println("  clean       Remove all downloaded Go archives from source path")
	fmt.Println("  uninstall   Remove the Go installation from", getInstallDir())
	fmt.Println("  help        Show this help message")
	fmt.Println("\nInstallation:")
	fmt.Println("  <version>   Install a specific version (e.g., 1.25.1 or 1.26)")
	fmt.Println("              If a partial version (1.26) is provided, the latest")
	fmt.Println("              patch version will be fetched automatically.")
	fmt.Println("\nExamples:")
	fmt.Println("  goupdate 1.25.1")
	fmt.Println("  goupdate 1.26")
	fmt.Println("  goupdate clean")
}

func cleanup() {
	files, err := os.ReadDir(getGoSourcePath())
	if err != nil {
		fmt.Printf("❌ Failed to read directory: %v\n", err)
		os.Exit(1)
	}

	suffix := fmt.Sprintf(".%s-%s%s", runtime.GOOS, getArch(), getArchiveExt())
	cleanedCount := 0
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "go") && strings.HasSuffix(file.Name(), suffix) {
			fmt.Printf("⌛ Cleaning: %s", file.Name())
			filePath := filepath.Join(getGoSourcePath(), file.Name())
			if err := os.Remove(filePath); err != nil {
				fmt.Printf("\r\033[2K❌ Failed to remove file %s: %v", file.Name(), err)
			} else {
				time.Sleep(200 * time.Millisecond)
				fmt.Printf("\r\033[2K✅ Cleaned: %s", file.Name())
				cleanedCount++
			}
			fmt.Printf("\n")
		}
	}
	if cleanedCount == 0 {
		fmt.Println("🧹 No files to clean")
	} else {
		fmt.Printf("Removed %d files\n", cleanedCount)
	}
}

func validateSudo() {
	if runtime.GOOS == "windows" || isPrivileged() {
		return
	}
	cmd := exec.Command("sudo", "-n", "true")
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("sudo", "-v")
		cmd.Run()
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("❌ Version string missing. eg: update 1.26.1")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		showVersion()
	case "clean":
		cleanup()
	case "uninstall":
		validateSudo()
		if len(os.Args) > 2 && strings.TrimSpace(os.Args[2]) != "" {
			uninstallGo(os.Args[2])
		} else {
			uninstallGo()
		}
	case "help", "-h", "--help":
		showHelp()
	default:
		validateSudo()
		version := os.Args[1]
		re := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
		if !re.MatchString(version) {
			fmt.Printf("❌ Invalid version format\n\n")
			showHelp()
			os.Exit(1)
		}

		if len(strings.Split(version, ".")) == 2 {
			latest, err := getLatestVersions(version)
			if err == nil {
				version = latest
			} else {
				version = version + ".0"
			}
		}

		if err := runInstallation(version); err != nil {
			fmt.Printf("❌ Failed to install go%s\n", version)
			os.Exit(1)
		}

		ensurePATH()
		fmt.Printf("\r\033[2K✅ Successfully installed go%s\n", version)
	}
}
