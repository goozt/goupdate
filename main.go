package main

import (
	"encoding/json"
	"fmt"
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
	goSourcePathTemplate = "%s/go/src/go%s.linux-amd64.tar.gz"
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
		completed := int(float64(barLength) * (float64(pw.Downloaded) / float64(pw.Total)))
		if completed > barLength {
			completed = barLength
		}
		bar := strings.Repeat("█", completed) + strings.Repeat("░", barLength-completed)

		// \r returns the cursor to the start of the line to overwrite it
		fmt.Printf("\rDownloading [%s] %.2f%% (%d/%d MB)",
			bar, percent, pw.Downloaded/1024/1024, pw.Total/1024/1024)
	}
	return n, nil
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

func getHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
	}
	return homeDir
}

func checkVersionAvailableLocally(version string) bool {
	filePath := fmt.Sprintf(goSourcePathTemplate, getHomeDir(), version)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}
	return true
}

func checkVersionAvailable(version string) error {
	if checkVersionAvailableLocally(version) {
		return nil
	}

	fmt.Printf("Version go%s is not found\n", version)
	url := fmt.Sprintf("https://golang.org/dl/go%s.linux-amd64.tar.gz", version)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	outputPath := fmt.Sprintf(goSourcePathTemplate, getHomeDir(), version)

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
	fmt.Println("\033[2K\rDownload completed")

	return nil
}

func runInstallation(version string) error {

	filePath := fmt.Sprintf(goSourcePathTemplate, getHomeDir(), version)

	// 1. Check if version exists
	if err := checkVersionAvailable(version); err != nil {
		fmt.Printf("Version go%s not available\n", version)
		return fmt.Errorf("failed to check version availability: %v", err)
	}

	// 2. Remove existing installation
	fmt.Println("Removing old Go installation")
	rmCmd := exec.Command("sudo", "rm", "-rf", "/usr/local/go")
	if err := rmCmd.Run(); err != nil {
		fmt.Printf("Failed to remove old directory: %v\n", err)
		return fmt.Errorf("failed to remove old directory: %v", err)
	}

	// 3. Extract the new tarball
	fmt.Printf("Installing go%s\n", version)
	tarCmd := exec.Command("sudo", "tar", "-C", "/usr/local", "-xzf", filePath)
	if err := tarCmd.Run(); err != nil {
		fmt.Printf("Failed to extract tarball: %v\n", err)
		return fmt.Errorf("failed to extract tarball: %v", err)
	}

	// 4. Verify the version
	out, err := exec.Command("/usr/local/go/bin/go", "version").Output()
	if err != nil {
		fmt.Printf("Error checking version: %v\n", err)
		return fmt.Errorf("failed to check version: %v", err)
	}

	// 5. Parse and compare versions
	versionStrs := strings.Fields(string(out))
	if len(versionStrs) < 3 {
		fmt.Printf("Unexpected version output: %s\n", string(out))
		return fmt.Errorf("unexpected version output")
	}

	// 6. Check if the installed version matches the expected version
	if versionStrs[2] != fmt.Sprintf("go%s", version) {
		fmt.Printf("Version mismatch. Expected: go%s, Got: %s\n", version, versionStrs[2])
		return fmt.Errorf("version mismatch")
	}

	return nil
}

func main() {
	// 1. Check for command line argument
	if len(os.Args) < 2 {
		fmt.Println("Version string missing. eg: update 1.26.1")
		os.Exit(1)
	}

	if os.Args[1] == "clean" {
		fmt.Println("Cleaning up downloaded Go versions")
		files, err := os.ReadDir(filepath.Join(getHomeDir(), "go", "src"))
		if err != nil {
			fmt.Printf("Failed to read directory: %v\n", err)
			os.Exit(1)
		}
		cleaned_count := 0
		for _, file := range files {
			if strings.HasPrefix(file.Name(), "go") && strings.HasSuffix(file.Name(), ".linux-amd64.tar.gz") {
				fmt.Printf("🟥 %s", file.Name())
				filePath := filepath.Join(getHomeDir(), "go", "src", file.Name())
				if err := os.Remove(filePath); err != nil {
					fmt.Printf("Failed to remove file %s: %v", file.Name(), err)
				} else {
					time.Sleep(time.Second * 1) // Sleep for a moment to show the progress
					fmt.Printf("\033[2K\r✅ %s", file.Name())
					cleaned_count++
				}
				fmt.Printf("\n")
			}
		}
		fmt.Printf("Cleaned up %d files\n", cleaned_count)
		os.Exit(0)
	}

	version := os.Args[1]
	re := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	if len(version) < 3 || !re.MatchString(version) {
		fmt.Println("Invalid version format. Version should be in the format '1.x' or '1.x.y'")
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
		fmt.Printf("Failed to install go%s\n", version)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully installed go%s\n", version)
}
