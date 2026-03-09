# goupdate

`goupdate` is a simple command-line tool for updating the Go programming language installation on your system. It takes a version number as an argument, verifies the corresponding Go tarball exists locally, removes the current Go installation from `/usr/local/go`, extracts the specified version, and checks that the update was successful. This tool is intended for users who manually manage their Go installations and want a quick way to switch versions.

### Installation

```
go install github.com/goozt/goupdate@latest
```

### Usage

```
goupdate 1.26    # Installs 1.26.1 as that is the latest version
goupdate 1.25    # Installs 1.25.8 as 8 is the latest stable version for 1.25
goupdate 1.25.1  # Installs the exact version 1.25.1

goupdate clean   # Remove all the downloaded go archives
goupdate version # Display the version of the goupdate
```

### License

MIT
