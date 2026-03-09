# goupdate

`goupdate` is a command-line tool for managing Go installations. It allows you to quickly install, update, or switch between different Go versions. The tool downloads the specified version from the official Go releases, removes any existing Go installation from `/usr/local/go`, extracts the new version, and verifies the installation. Additional commands are available for cleaning up downloaded archives, checking the tool's version, and uninstalling Go. `goupdate` is designed for users who prefer to manage Go versions without manually downloading and installing.

### Installation

```sh
go install github.com/goozt/goupdate@latest
```

### Usage

```sh
goupdate 1.26      # Installs 1.26.1 as that is the latest version
goupdate 1.25      # Installs 1.25.8 as 8 is the latest stable version for 1.25
goupdate 1.25.1    # Installs the exact version 1.25.1

goupdate clean     # Remove all the downloaded go archives
goupdate version   # Display the version of the goupdate
goupdate uninstall # Uninstall current version of Go
```

### Troubleshooting

1. Running `sudo goupdate ...` results in `goupdate` not found?

   **Solution:** The `sudo` command looks in paths `/usr/local/sbin`, `/usr/local/bin`, `/usr/sbin`, `/usr/bin`. But go install binary to `$GOPATH/bin`. To fix this, use `sudo ln -s $GOPATH/bin/goupdate /usr/local/bin/goupdate`. However, it is recommended to not use sudo command directly. Running goupdate without sudo will prompt for sudo access by default.

### License

MIT
