# goupdate

`goupdate` is a simple command-line tool for updating the Go programming language installation on your system. It takes a version number as an argument, verifies the corresponding Go tarball exists locally, removes the current Go installation from `/usr/local/go`, extracts the specified version, and checks that the update was successful. This tool is intended for users who manually manage their Go installations and want a quick way to switch versions.

### Installation

```
go install github.com/goozt/goupdate@latest
```
