package main

import (
	"net/http"
	"strings"
	"sync"
)

// Not a crypto related function so no need for constant time
func validate(s string) bool {
	if s[len(s)-1] == '~' {
		s = s[:len(s)-1]
	}

	for _, char := range s {
		if !strings.Contains(charset, string(char)) {
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
	if debug && logger != nil {
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
