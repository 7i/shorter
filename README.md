[![Go Report Card](https://goreportcard.com/badge/github.com/7i/shorter)](https://goreportcard.com/report/github.com/7i/shorter)
![Linux](https://img.shields.io/badge/Supports-Linux-green.svg)
![windows](https://img.shields.io/badge/Supports-windows-green.svg)
[![License](https://img.shields.io/badge/License-UNLICENSE-blue.svg)](https://raw.githubusercontent.com/7i/shorter/master/UNLICENSE)
[![License](https://img.shields.io/badge/License-0BSD-blue.svg)](https://raw.githubusercontent.com/7i/shorter/master/LICENSE)
# shorter
URL shortener with pastebin and file upload functions


## WIP

This project is a *work in progress*. The implementation is *incomplete* and subject to change.

If you want to try to run shorter, please set your correct values in the config before starting the server.

Shortened links on the pre alpha version test site 7i.se will be cleared from time to time during testing without notice.

## Installation

```bash
go get github.com/7i/shorter
```

## Usage

```bash
shorter /path/to/config
```

## TODO
- [x] Implement shortening of URLs
   - [x] 1 char long - 10min timeout
   - [x] 2 chars long - 1h timeout
   - [x] 3 chars long - 30days timeout
   - [x] make timeouts configurable
- [x] Add config file that specifies relevant options
- [ ] Pastebin functionality with same timeouts as above
- [ ] Temporary file upload
   - [ ] File encryption with AES-GCM
   - [x] Random human-readable password made in Diceware style via JavaScript (dictionary words not dice rolls)
- [ ] Move to ssl with Let's Encrypt
- [ ] Save all active links in a database file so we can resume the server state if the server needs to restart
- [ ] Add support for subdomains with diffrent configs e.g. d1.7i.se
   - [ ] Add password/client cert protected subdomain management e.g. d1.7i.se/admin
   - [ ] Let the user managing a subdomain specify generic links and set timeouts, including "no timeout" for the shortened links, text-blobs and files.
- [ ] Enable CSP
   - [ ] Move all js and css to seperate files and modify html/template files to use these
   - [ ] Setup a CSP report collector
   - [ ] setup nonce for all scripts and css files


## License

The `shorter` project is dual-licensed to the [public domain](UNLICENSE) and under a [zero-clause BSD license](LICENSE). You may choose either license to govern your use of `shorter`.

