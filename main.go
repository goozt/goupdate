package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// 1. Check for command line argument
	if len(os.Args) < 2 {
		fmt.Println("Invalid version. eg: update 1.25.1")
		os.Exit(1)
	}

	version := os.Args[1]
	filePath := fmt.Sprintf("/home/nikz/go/src/go%s.linux-amd64.tar.gz", version)

	// 2. Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Println("File does not exist:", filePath)
		os.Exit(1)
	}

	// 3. Remove existing installation
	fmt.Println("Removing old Go installation...")
	rmCmd := exec.Command("sudo", "rm", "-rf", "/usr/local/go")
	if err := rmCmd.Run(); err != nil {
		fmt.Printf("Failed to remove old directory: %v\n", err)
		os.Exit(1)
	}

	// 4. Extract the new tarball
	fmt.Printf("Installing go%s...\n", version)
	tarCmd := exec.Command("sudo", "tar", "-C", "/usr/local", "-xzf", filePath)
	if err := tarCmd.Run(); err != nil {
		fmt.Printf("Failed to extract tarball: %v\n", err)
		os.Exit(1)
	}

	// 5. Verify the version
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		fmt.Printf("Error checking version: %v\n", err)
		os.Exit(1)
	}

	versionStrs := strings.Fields(string(out))
	if len(versionStrs) < 3 {
		fmt.Printf("Unexpected version output: %s\n", string(out))
		os.Exit(1)
	}

	if versionStrs[2] != fmt.Sprintf("go%s", version) {
		fmt.Printf("Version mismatch. Expected: go%s, Got: %s\n", version, versionStrs[2])
		os.Exit(1)
	}

	fmt.Printf("Successfully installed go%s", version)
}
