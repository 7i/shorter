package main

import (
	"bytes"
	"compress/gzip"
	"go/build"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// validate validates if string s contains only characters in charset. validate is not a crypto related function so no need for constant time
func validate(s string) bool {
	if len(s) == 0 {
		return true
	}

	if s[len(s)-1] == '~' {
		s = s[:len(s)-1]
	}

	for _, char := range s {
		if !strings.Contains(customKeyCharset, string(char)) {
			return false
		}
	}
	return true
}

// initMaps will init and fill linkLen1, linkLen2 and linkLen3 with all valid free keys for each of them
func initLinkLens() {
	linkLen1 = linkLen{
		mutex:   sync.RWMutex{},
		linkMap: make(map[string]*link),
		freeMap: make(map[string]bool),
		timeout: config.Clear1Duration,
	}

	linkLen2 = linkLen{
		mutex:   sync.RWMutex{},
		linkMap: make(map[string]*link),
		freeMap: make(map[string]bool),
		timeout: config.Clear2Duration,
	}

	linkLen3 = linkLen{
		mutex:   sync.RWMutex{},
		linkMap: make(map[string]*link),
		freeMap: make(map[string]bool),
		timeout: config.Clear3Duration,
	}

	linkCustom = linkLen{
		mutex:   sync.RWMutex{},
		linkMap: make(map[string]*link),
		timeout: config.ClearCustomLinksDuration,
	}

	linkLen1.mutex.Lock()
	defer linkLen1.mutex.Unlock()
	linkLen2.mutex.Lock()
	defer linkLen2.mutex.Unlock()
	linkLen3.mutex.Lock()
	defer linkLen3.mutex.Unlock()

	for _, char1 := range charset {
		linkLen1.freeMap[string(char1)] = true
		for _, char2 := range charset {
			linkLen2.freeMap[string(char1)+string(char2)] = true
			for _, char3 := range charset {
				linkLen3.freeMap[string(char1)+string(char2)+string(char3)] = true
			}
		}
	}
	if logger != nil {
		logger.Println("All maps initialized", logSep)
	}
}

func addHeaders(w http.ResponseWriter) {
	w.Header().Add("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
}

// hasValidHost returns true if the host string matches any of the valid hosts specified in the config
func validRequest(r *http.Request) bool {
	var validHost, validType bool
	for _, d := range config.DomainNames {
		if r.Host == d {
			validHost = true
		}
	}

	if r.Method == "GET" || r.Method == "POST" {
		validType = true
	}

	return validHost && validType
}

func validURL(link string) bool {
	// simple sanity check to fail early, If len(link) is less than 11 it is definitely an invalid url link.
	if len(link) < 11 || !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return false
	}
	_, err := url.Parse(link)
	if err != nil {
		return false
	}
	return true
}

func lowRAM() bool {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys > config.MaxRAM
}

func findFolderDefaultLocations(folder string) (path string) {
	if _, err := os.Stat(filepath.Join("/7i", folder)); !os.IsNotExist(err) {
		return filepath.Join("/7i", folder)
	}
	if _, err := os.Stat(filepath.Join(".", folder)); !os.IsNotExist(err) {
		return filepath.Join(".", folder)
	}
	possibleDirs := os.Getenv("GOPATH")
	if possibleDirs == "" {
		possibleDirs = build.Default.GOPATH
	} else {
		var dirs []string
		if runtime.GOOS == "windows" {
			dirs = strings.Split(possibleDirs, ";")
		} else {
			dirs = strings.Split(possibleDirs, ":")
		}
		for _, dir := range dirs {
			if _, err := os.Stat(filepath.Join(dir, "src", "github.com", "7i", "shorter", folder)); !os.IsNotExist(err) {
				// Found
				return filepath.Join(dir, "src", "github.com", "7i", "shorter", folder)
			}
		}
	}
	return ""
}

func compress(data string) (compressedData string, err error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := io.Copy(zw, strings.NewReader(data)); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func decompress(data string) (decompressedData string, err error) {
	var buf bytes.Buffer
	zw, err := gzip.NewReader(strings.NewReader(data))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(&buf, zw); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func redirectToDecompressed(lnk *link, w http.ResponseWriter, r *http.Request) {
	if lnk == nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: invalid link in request to redirectToDecompressed().")
		return
	}
	dataReader, err := gzip.NewReader(strings.NewReader(lnk.data))
	if err == nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: invalid link.data in request to redirectToDecompressed().")
		return
	}
	buf := new(strings.Builder)
	_, err = io.Copy(buf, dataReader)
	if err != nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: when reading data in request to redirectToDecompressed().")
		return
	}
	logOK(r, http.StatusTemporaryRedirect)
	http.Redirect(w, r, buf.String(), http.StatusTemporaryRedirect)
	return
}

func returnDecompressed(lnk *link, w http.ResponseWriter, r *http.Request) {
	if lnk == nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: invalid lnk in request to returnDecompressed().")
		return
	}
	dataReader, err := gzip.NewReader(strings.NewReader(lnk.data))
	if err == nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: invalid lnk.data in request to returnDecompressed().")
		return
	}
	if _, err = io.Copy(w, dataReader); err != nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: while decompresing in request to returnDecompressed().")
		return
	}
	if err = dataReader.Close(); err != nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: closing dataReader in request to returnDecompressed().")
		return
	}
	logOK(r, http.StatusOK)
	return
}

// logErrors will write the error to the log file, note that the arguments errStr and logStr should be escaped correctly with url.QueryEscape() if any user data is included.
func logErrors(w http.ResponseWriter, r *http.Request, errStr string, statusCode int, logStr string) {
	if logger != nil {
		logger.Println("Request:\nStatuscode:", statusCode, logStr, errStr, "\n", url.QueryEscape(r.Host+r.RequestURI), "\n", r, logSep)
	}
	http.Error(w, errServerError, statusCode)
}

func logOK(r *http.Request, statusCode int) {
	if logger != nil {
		logger.Println("Request:\nStatuscode:", statusCode, "\n", url.QueryEscape(r.Host+r.RequestURI), "\n", r, logSep)
	}
}
