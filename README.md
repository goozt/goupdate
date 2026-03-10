# goupdate

`goupdate` is a cross-platform command-line tool for managing Go installations. It allows you to quickly install, update, or switch between different Go versions on Linux, macOS, and Windows. The tool downloads the specified version from the official Go releases, removes any existing installation, extracts the new version, and verifies it. It also automatically updates your PATH configuration if needed.

| Platform      | Install directory | Archive format |
|---------------|-------------------|----------------|
| Linux / macOS | `/usr/local/go`   | `.tar.gz`      |
| Windows       | `C:\go`           | `.zip`         |

### Installation

```sh
go install github.com/goozt/goupdate@latest
```

#### Install manually

If Go is not installed, download a binary from [Releases](https://github.com/goozt/goupdate/releases). It can install Go even if no prior Go installation exists.

### Usage

```sh
goupdate 1.26      # Installs the latest patch release of 1.26 (e.g. 1.26.1)
goupdate 1.25      # Installs the latest patch release of 1.25 (e.g. 1.25.8)
goupdate 1.25.1    # Installs the exact version 1.25.1

goupdate clean     # Remove all downloaded Go archives from cache
goupdate version   # Display the version of goupdate
goupdate uninstall # Uninstall the current Go installation
```

### PATH management

After installation, `goupdate` checks whether the Go binary directory and `$GOPATH/bin` are in your PATH and adds them automatically if not:

- **Linux / macOS** — appends `export PATH=...` entries to `~/.bashrc`, `~/.zshrc`, and/or `~/.profile` (whichever exist). Restart your shell or `source` the config file to apply.
- **Windows** — updates the user-level `Path` environment variable via PowerShell. Open a new terminal to apply.

### Privileges

Installing Go requires write access to the system install directory.

- **Linux / macOS** — run `goupdate` as a normal user; it will prompt for `sudo` access automatically. You can also run it as root directly.
- **Windows** — no elevated privileges are required; the tool writes to `C:\go` directly.

### Troubleshooting

#### Linux / macOS: `sudo goupdate` reports command not found

`sudo` searches a restricted set of paths (`/usr/local/sbin`, `/usr/local/bin`, etc.) but `go install` places binaries in `$GOPATH/bin`. You do **not** need to call `sudo goupdate` directly — running `goupdate` as a normal user will prompt for `sudo` access automatically. If you still want to call it with `sudo`, create a symlink:

```sh
sudo ln -s $GOPATH/bin/goupdate /usr/local/bin/goupdate
```

### License

MIT
