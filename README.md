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

## Examples
A deployed version of shorter is accessable at [7i.se](http://7i.se)

create a temporary link to "https://www.example.com" via a GET request that is as short as possible:
```bash
7i.se?https://www.example.com
or
7i.se/?https://www.example.com
```
create a temporary link to "https://www.example.com" via a GET request using the key "KeyToExample":
```bash
7i.se/KeyToExample?https://www.example.com
or
7i.se/KeyToExample/?https://www.example.com
```

## TODO
- [x] Implement shortening of URLs
   - [x] 1 char long - configurabe timeout
   - [x] 2 chars long - configurabe timeout
   - [x] 3 chars long - configurabe timeout
   - [x] make timeouts configurable
   - [x] temporary word bindings (7i.se/coolthing)
   - [x] quick add link via get request with syntax 7i.se?https://example.com
   - [x] quick add word bindings link via get request with syntax 7i.se/coolthing?https://example.com where coolthing is the key
   - [ ] optional removal of link after N accesses
- [x] Add functionality to print where a link is pointing by adding ~ at the end of the link e.g. 7i.se/a~ will display where 7i.se/a is pointing to
- [x] Add config file that specifies relevant options
- [x] Pastebin functionality with same timeouts as above
- [ ] Temporary file upload
   - [ ] File encryption with AES-GCM
   - [x] Random human-readable password made in Diceware style via JavaScript (dictionary words not dice rolls)
- [x] Move to ssl with Let's Encrypt
- [ ] Save all active links in a database file so we can resume the server state if the server needs to restart
- [ ] Add support for subdomains with diffrent configs e.g. d1.7i.se
   - [ ] Add password/client cert protected subdomain management e.g. d1.7i.se/admin
   - [ ] Let the user managing a subdomain specify generic links and set timeouts, including "no timeout" for the shortened links, text-blobs and files.
- [ ] Enable CSP
   - [ ] Move all js and css to seperate files and modify html/template files to use these
   - [ ] Setup a CSP report collector
   - [ ] setup nonce for all scripts and css files
- [ ] Use blocklists for known malware sites, integrate with:
   - [ ] https://www.stopbadware.org/firefox
   - [ ] https://www.malwaredomainlist.com
   - [ ] https://isc.sans.edu/suspicious_domains.html
   - [ ] https://zeltser.com/malicious-ip-blocklists/
   - [ ] if linking to a page that redirects, follow redirects only for 5 levels and display error if redirected more times
- [ ] Check entropy, beginning and end of uploaded file as a sanity check to verifiy that the file is encrypted.
- [ ] Include report form to take down links that breaks terms of usage
   - [ ] implement capcha for submitting reports to take down links
- [ ] Create Terms of usage


## License

The `shorter` project is dual-licensed to the [public domain](UNLICENSE) and under a [zero-clause BSD license](LICENSE). You may choose either license to govern your use of `shorter`.

