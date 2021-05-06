package main

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Config contains all valid fields from a shorter config file
type Config struct {
	// BaseDir specifies the path to the base directory to search for resources for the shorter service
	BaseDir string `yaml:"BaseDir"`
	// UploadDir specifies the path to the directory that shorter will save temporary files and textblobs to
	UploadDir string `yaml:"UploadDir"`
	// BackupDir specifies the path to the directory that shorter will use to save its database file "shorter.db"
	BackupDBDir string `yaml:"BackupDBDir"`
	// CertDir specifies the path to the directory that shorter will use to cache the LetsEnctypt certs
	CertDir string `yaml:"CertDir"`
	// Logging specifies if shorter should write debug data and requests to a log file, if false no logging will be done
	Logging bool `yaml:"Logging"`
	// Logfile specifies the file to write logs to, If Logfile is not specified BaseDir/shorter.log is used
	Logfile string `yaml:"Logfile"`
	// DomainName should be the domain name of the instance of shorter, e.g. 7i.se
	DomainNames []string `yaml:"DomainNames"`
	// NoTLS specifies if we should inactivate TLS and only use unencrypted HTTP
	NoTLS bool `yaml:"NoTLS"`
	// AddressPort specifies the address and port the shorter service should listen on
	AddressPort string `yaml:"AddressPort"`
	// TLSAddressPort specifies the adress and port the shorter service should listen to HTTPS connections on
	TLSAddressPort string `yaml:"TLSAddressPort"`
	// Clear1Duration should specify the time between clearing old 1 character long URLs.
	// The syntax is 1h20m30s for 1hour 20minutes and 30 seconds
	Clear1Duration time.Duration `yaml:"Clear1Duration"`
	// Clear2Duration, same as Clear1Duration bur for 2 character long URLs
	Clear2Duration time.Duration `yaml:"Clear2Duration"`
	// Clear3Duration, same as Clear1Duration bur for 3 character long URLs
	Clear3Duration time.Duration `yaml:"Clear3Duration"`
	// ClearCustomLinksDuration, same as Clear1Duration bur for custom URLs
	ClearCustomLinksDuration time.Duration `yaml:"ClearCustomLinksDuration"`
	// MaxCustomLinks, sets the maximum number of active CustomLinks before reporting that all are used up
	MaxCustomLinks int `yaml:"MaxCustomLinks"`
	// MaxFileSize specifies the maximum filesize when uploading temporary files
	MaxFileSize int64 `yaml:"MaxFileSize"`
	// MaxDiskUsage specifies how much space in total shorter is allowed to save ondisk
	MaxDiskUsage int64 `yaml:"MaxDiskUsage"`
	// LinkAccessMaxNr specifies how many times a link is allowed to be accessed if xTimes is specified in the request
	LinkAccessMaxNr int `yaml:"LinkAccessMaxNr"`
	// MaxRam sets the maximum RAM usage that shorter is allowd to use before returning 500 errLowRAM errors to new requests
	MaxRAM uint64 `yaml:"MaxRAM"`
	// Email optionally specifies a contact email address.
	// This is used by CAs, such as Let's Encrypt, to notify about problems with issued certificates.
	// If the Client's account key is already registered, Email is not used.
	Email string `yaml:"Email"`
	// StaticLinks contains a list of static keys that will no time out
	StaticLinks map[string]string `yaml:"StaticLinks"`
}

// link tracks the contents and lifetime of a link.
type link struct {
	key          string
	linkType     string
	data         string
	isCompressed bool
	times        int
	timeout      time.Time
	nextClear    *link
}

type linkLen struct {
	mutex     sync.RWMutex
	linkMap   map[string]*link
	freeMap   map[string]bool
	links     int
	nextClear *link // first element in linked list
	endClear  *link // last element in linked list
	timeout   time.Duration
}

// Add adds the value lnk with a new key if no key is provided to linkMap and removes the same key from freeMap if freeMap is used and returns the key used or an error, note that the error should be useful for the user while not leak server information
func (l *linkLen) Add(lnk *link) (key string, err error) {
	if lnk == nil {
		if logger != nil {
			logger.Println("Add: invalid parameter lnk, lnk can not be nil", logSep)
		}
		return "", errors.New(errServerError)
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	// check if lnk is a custum link
	isCustomLink := false
	if l.freeMap == nil {
		if len(lnk.key) < 4 || len(lnk.key) >= MaxKeyLen || !validate(lnk.key) {
			logger.Println("AddKey: invalid parameter key, key can only be > 4 or < "+strconv.Itoa(MaxKeyLen), logSep)
			return "", errors.New("Error: key can only be of length > 4 and < " + strconv.Itoa(MaxKeyLen) + " and only use the folowing characters:\n" + customKeyCharset)
		}
		isCustomLink = true
		key = lnk.key
	}

	// Formated output for the log
	logstr := ""

	if logger != nil {
		if lnk.isCompressed {
			decompressed, err := decompress(lnk.data)
			if err != nil {
				logger.Println("Error while decompressing lnk.data")
				return "", errors.New(errServerError)
			}
			logstr = "lnk:\n   linkType: " + lnk.linkType + "\n   data: " + url.QueryEscape(decompressed) + "\n   timeout: " + lnk.timeout.UTC().Format(dateFormat) + "\n   xTimes: " + strconv.Itoa(lnk.times)
		} else {
			logstr = "lnk:\n   linkType: " + lnk.linkType + "\n   data: " + url.QueryEscape(lnk.data) + "\n   timeout: " + lnk.timeout.UTC().Format(dateFormat) + "\n   xTimes: " + strconv.Itoa(lnk.times)
		}
		logger.Println("Starting to Add", logstr)
		if isCustomLink {
			logger.Println("l.links:", l.links)
		} else {
			logger.Println("len(l.freeMap):", len(l.freeMap))
		}
		if l.endClear != nil {
			logger.Println("lnk.timeout:", lnk.timeout.UTC().Format(dateFormat), "l.endClear.timeout:", l.endClear.timeout.UTC().Format(dateFormat))
		} else {
			logger.Println("lnk.timeout:", lnk.timeout.UTC().Format(dateFormat), "l.endClear is nil, will set it to lnk if no other errors occur")
		}
	}

	if (!isCustomLink && len(l.freeMap) == 0) || (isCustomLink && l.links >= config.MaxCustomLinks) {
		if logger != nil {
			logger.Println("Error: No keys left", logSep)
		}
		if isCustomLink {
			return "", errors.New("No custom links left")
		} else {
			return "", errors.New("No keys left for key length " + strconv.Itoa(len(l.endClear.key)))
		}
	}
	if time.Since(lnk.timeout) > 0 {
		if logger != nil {
			logger.Println("Error, ", logstr, "timeout has to be in the future", logSep)
		}
		return "", errors.New(errServerError)
	}
	// if we are adding a specific lenghth key, get the next free key from l.freeMap
	if !isCustomLink {
		for key = range l.freeMap {
			break
		}
		if logger != nil {
			logger.Println("Picking key:", key)
		}
		lnk.key = key
	}

	if l.nextClear == nil {
		l.nextClear = lnk
	} else {
		if l.endClear == nil {
			if logger != nil {
				logger.Println("Error", logstr, "endClear is nil but nextClear is set to a value", logSep)
			}
			return "", errors.New(errServerError)
		}
		if l.endClear.timeout.Sub(lnk.timeout) > 0 {
			if logger != nil {
				logger.Println("Error", logstr, "timeout has to be after the previous links timeout", logSep)
			}
			return "", errors.New(errServerError)
		}
		l.endClear.nextClear = lnk
	}
	l.endClear = lnk
	l.linkMap[key] = lnk
	if isCustomLink {
		l.links++
	} else {
		delete(l.freeMap, key)
	}
	if logger != nil {
		logger.Println("Finished adding key:", url.QueryEscape(key), "with", logstr, "\nl.nextClear.key", url.QueryEscape(l.nextClear.key), "\nl.endClear.key", url.QueryEscape(l.endClear.key), logSep)
	}
	return key, nil
}

// TimeoutHandler removes links from its linkMap when the links have timed out. Start TimeoutHandler in a separate gorutine and only start one TimeoutHandler() per linkLen.
func (l *linkLen) TimeoutManager() {
	if logger != nil {
		logger.Println("TimeoutHandler started for", len(l.freeMap), "keys", logSep)
	}
	// Check if any new keys should be cleared every 10 seconds
	ticker := time.NewTicker(time.Second * 10)
	// Check if any new keys should be cleared set by l.nextClear.timeout
	timer := time.NewTimer(time.Second)
	for {
		// block until it is time to clear the next link or to check if l.nextClear has timed out every 10 seconds
		select {
		case <-ticker.C:
		case <-timer.C:
		}
		l.mutex.RLock()
		if l.nextClear != nil && time.Since(l.nextClear.timeout) > 0 {
			l.mutex.RUnlock()
			// Time to clear next link
			l.mutex.Lock()
			keyToClear := l.nextClear.key
			if l.nextClear.nextClear != nil && l.nextClear != l.endClear {
				l.nextClear = l.nextClear.nextClear
				if time.Since(l.nextClear.timeout) > 0 {
					// if the timeout already passed on nextClear then send a new value on the channel timer.C
					timer.Reset(time.Nanosecond)
				} else {
					timer.Reset(l.nextClear.timeout.Sub(time.Now()))
				}
			} else if l.nextClear.nextClear == nil && l.nextClear == l.endClear {
				l.nextClear = nil
				l.endClear = nil
			} else {
				if logger != nil {
					logger.Println("ERROR: invalid state, if l.nextClear.nextClear == nil then l.nextClear has to be equal to l.endClear\nlinkMap:", url.QueryEscape(fmt.Sprint(l.linkMap)), "\nfreeMap:", url.QueryEscape(fmt.Sprint(l.freeMap)), "\nnextClear:", url.QueryEscape(fmt.Sprint(l.nextClear)), "\nendClear:", url.QueryEscape(fmt.Sprint(l.endClear)), logSep)
				}
			}
			delete(l.linkMap, keyToClear)
			if l.freeMap != nil {
				// Links of specific length
				l.freeMap[keyToClear] = true
				if logger != nil {
					logger.Println("Finished clearing nextClear of length:", len(keyToClear), "\ncurrently using:", len(l.linkMap), "keys\ncurrent free keys:", len(l.freeMap), logSep)
				}
			} else {
				// Custom links
				l.links--
				if logger != nil {
					logger.Println("Finished clearing nextClear for custom link\ncurrently using:", l.links, "keys\ncurrent free keys:", config.MaxCustomLinks-l.links, logSep)
				}
			}

			l.mutex.Unlock()
			l.mutex.RLock()
		}
		l.mutex.RUnlock()

	}
}
