package main

import (
	"encoding/json"
	"fmt"
	"go/build"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	goupdate_version = "1.1.0"
	isRoot           = os.Geteuid() == 0
)

type progressWriter struct {
	Total      int64
	Downloaded int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Downloaded += int64(n)

	if pw.Total > 0 {
		percent := float64(pw.Downloaded) / float64(pw.Total) * 100
		barLength := 20
		completed := min(int(float64(barLength)*(float64(pw.Downloaded)/float64(pw.Total))), barLength)
		bar := strings.Repeat("█", completed) + strings.Repeat("░", barLength-completed)

		// \r returns the cursor to the start of the line to overwrite it
		fmt.Printf("\rDownloading [%s] %.2f%% (%d/%d MB)",
			bar, percent, pw.Downloaded/1024/1024, pw.Total/1024/1024)
	}
	return n, nil
}

func getGoSourcePath() string {
	if isRoot {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "go", "src")
	}
	return filepath.Join(build.Default.GOPATH, "src")
}

func generatePathForVersion(version string) string {
	return fmt.Sprintf(getGoSourcePath()+"/go%s.linux-amd64.tar.gz", version)
}

func getLatestVersions(partialVersion string) (string, error) {
	apiUrl := "https://go.dev/dl/?mode=json"
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
		if strings.HasPrefix(v.Version, "go"+partialVersion) {
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
		return nil
	}

	fmt.Printf("🔴 Version go%s is not found\n", version)
	url := fmt.Sprintf("https://golang.org/dl/go%s.linux-amd64.tar.gz", version)
	resp, err := http.Get(url)
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

	pw := &progressWriter{Total: resp.ContentLength}
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
	if os.IsNotExist(err) {
		return false
	}
	// Return true only if it exists AND is a directory
	return info.IsDir()
}

func uninstallGo() error {
	if dirExists("/usr/local/go") {
		fmt.Print("⌛ Uninstalling Go...")
		var rmCmd *exec.Cmd
		if isRoot {
			rmCmd = exec.Command("rm", "-rf", "/usr/local/go")
		} else {
			rmCmd = exec.Command("sudo", "rm", "-rf", "/usr/local/go")
			fmt.Print("\r\033[2K⌛ Uninstalling Go...")
		}
		if err := rmCmd.Run(); err != nil {
			fmt.Printf("❌ Failed to remove old directory: %v\n", err)
			return fmt.Errorf("failed to remove old directory: %v", err)
		}
		fmt.Printf("\r\033[2K✅ Uninstalled Go\n")
	} else {
		fmt.Println("No Go installation found")
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
	err := uninstallGo()
	if err != nil {
		fmt.Printf("❌ Failed to uninstall old Go version: %v\n", err)
		return fmt.Errorf("failed to uninstall old Go version: %v", err)
	}

	// 3. Extract the new tarball
	fmt.Printf("⌛ Installing go%s", version)
	var tarCmd *exec.Cmd
	if isRoot {
		tarCmd = exec.Command("tar", "-C", "/usr/local", "-xzf", filePath)
	} else {
		tarCmd = exec.Command("sudo", "tar", "-C", "/usr/local", "-xzf", filePath)
	}
	if err := tarCmd.Run(); err != nil {
		fmt.Printf("❌ Failed to extract tarball: %v\n", err)
		return fmt.Errorf("failed to extract tarball: %v", err)
	}

	// 4. Verify the version
	out, err := exec.Command("/usr/local/go/bin/go", "version").Output()
	if err != nil {
		fmt.Printf("❌ Error checking version: %v\n", err)
		return fmt.Errorf("failed to check version: %v", err)
	}

	// 5. Parse and compare versions
	versionStrs := strings.Fields(string(out))
	if len(versionStrs) < 3 {
		fmt.Printf("❌ Unexpected version output: %s\n", string(out))
		return fmt.Errorf("unexpected version output")
	}

	// 6. Check if the installed version matches the expected version
	if versionStrs[2] != fmt.Sprintf("go%s", version) {
		fmt.Printf("❌ Version mismatch. Expected: go%s, Got: %s\n", version, versionStrs[2])
		return fmt.Errorf("version mismatch")
	}

	return nil
}

func showVersion() {
	fmt.Printf("goupdate version v%s\n\nSource: https://github.com/goozt/goupdate\nAuthor: Nikhil John\nLicense: MIT\n", goupdate_version)
}

func showHelp() {
	fmt.Println("Usage: goupdate <command>|<version>")
	fmt.Println("\nCommands:")
	fmt.Println("  version     Show goupdate tool version")
	fmt.Println("  clean       Remove all downloaded Go tarballs from source path")
	fmt.Println("  uninstall   Remove the Go installation from /usr/local/go")
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

	cleanedCount := 0
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "go") && strings.HasSuffix(file.Name(), ".linux-amd64.tar.gz") {
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
	if !isRoot {
		cmd := exec.Command("sudo", "-n", "true")
		if err := cmd.Run(); err != nil {
			cmd = exec.Command("sudo", "-v")
			cmd.Run()
		}
	}
}

func main() {
	// 1. Check for command line argument
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
		uninstallGo()
	case "help", "-h", "--help":
		showHelp()
	default:
		validateSudo()
		version := os.Args[1]
		re := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
		if len(version) < 3 || !re.MatchString(version) {
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

		fmt.Printf("\r\033[2K✅ Successfully installed go%s\n", version)
	}

}
