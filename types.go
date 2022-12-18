package main

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config contains all valid fields from a shorter config file
type Config struct {
	// BaseDir specifies the path to the base directory to search for resources for the shorter service
	BaseDir string `yaml:"BaseDir"`
	// CertDir specifies the path to the directory that shorter will use to cache the LetsEnctypt certs
	CertDir string `yaml:"CertDir"`
	// Logging specifies if shorter should write debug data and requests to a log file, if false no logging will be done
	Logging bool `yaml:"Logging"`
	// Logfile specifies the file to write logs to, If Logfile is not specified BaseDir/shorter.log is used
	Logfile string `yaml:"Logfile"`
	//LogSep is a secret log separator value to make it harder to forge log entry's
	LogSep string `yaml:"LogSep"`
	// DomainName should be the domain name of the instance of shorter, e.g. 7i.se
	DomainNames []string `yaml:"DomainNames"`
	// NoTLS specifies if we should inactivate TLS and only use unencrypted HTTP
	NoTLS bool `yaml:"NoTLS"`
	// AddressPort specifies the address and port the shorter service should listen on
	AddressPort string `yaml:"AddressPort"`
	// TLSAddressPort specifies the address and port the shorter service should listen to HTTPS connections on
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
	// Max file size when parsing POST form data
	MaxFileSize int64 `yaml:"MaxFileSize"`
	// MaxDiskUsage specifies how much space in total shorter is allowed to save on disk
	MaxDiskUsage int64 `yaml:"MaxDiskUsage"`
	// LinkAccessMaxNr specifies how many times a link is allowed to be accessed if xTimes is specified in the request
	LinkAccessMaxNr int `yaml:"LinkAccessMaxNr"`
	// MaxRam sets the maximum RAM usage that shorter is allowed to use before returning 500 errLowRAM errors to new requests
	MaxRAM uint64 `yaml:"MaxRAM"`
	// Email optionally specifies a contact email address.
	// This is used by CAs, such as Let's Encrypt, to notify about problems with issued certificates.
	// If the Client's account key is already registered, Email is not used.
	Email string `yaml:"Email"`
	// StaticLinks contains a list of static keys that will no time out
	StaticLinks map[string]string `yaml:"StaticLinks"`
	// Salt is used as the Salt for the password for special requests
	Salt string `yaml:"Salt"`
	// HashSHA256 is the sha256 hash of the password and Salt used for special requests
	HashSHA256 string `yaml:"HashSHA256"`
	// CSP controls if a Content-Security-Policy should be included in all requests to shorter, if not set no Content-Security-Policy header is used
	CSP string `yaml:"CSP"`
	// HSTS controls if a Strict-Transport-Security header should be included in all requests to shorter. Can only be used if NoTLS is set to false. If not set then no Strict-Transport-Security header will be included
	HSTS string `yaml:"HSTS"`
	// ReportTo controls if a Report-To header should be included in all requests to shorter, if not set no Report-To header is used
	ReportTo string `yaml:"ReportTo"`
}

// link tracks the contents and lifetime of a link.
type Link struct {
	Key          string    `json:"Key"`
	LinkType     string    `json:"LinkType"`
	Data         string    `json:"Data"`
	IsCompressed bool      `json:"IsCompressed"`
	Times        int       `json:"Times"`
	Timeout      time.Time `json:"Timeout"`
	NextClear    *Link     `json:"NextClear"`
}

type LinkLen struct {
	Mutex     sync.RWMutex     `json:"Mutex"`
	LinkMap   map[string]*Link `json:"LinkMap"`
	FreeMap   map[string]bool  `json:"FreeMap"`
	Links     int              `json:"Links"`
	NextClear *Link            `json:"NextClear"` // first element in linked list
	EndClear  *Link            `json:"EndClear"`  // last element in linked list
	Timeout   time.Duration    `json:"Timeout"`
	Domain    string           `json:"Domain"`
}

type LinkLens struct {
	LinkLen1   LinkLen `json:"LinkLen1"`
	LinkLen2   LinkLen `json:"LinkLen2"`
	LinkLen3   LinkLen `json:"LinkLen3"`
	LinkCustom LinkLen `json:"LinkCustom"`
}

type showLinkVars struct {
	Domain  string `json:"Domain"`
	Data    string `json:"Data"`
	Timeout string `json:"Timeout"`
}

// Add adds the value lnk with a new key if no key is provided to linkMap and removes the same key from freeMap if freeMap is used and returns the key used or an error, note that the error should be useful for the user while not leak server information
func (l *LinkLen) Add(lnk *Link) (key string, err error) {
	if lnk == nil {
		if logger != nil {
			logger.Println("Add: invalid parameter lnk, lnk can not be nil")
		}
		return "", errors.New(errServerError)
	}

	l.Mutex.Lock()
	defer l.Mutex.Unlock()

	// check if lnk is a custom link, FreeMap is nill for custom links
	isCustomLink := false
	if l.FreeMap == nil {
		if len(lnk.Key) < 4 || len(lnk.Key) >= MaxKeyLen || !validate(lnk.Key) {
			logger.Println("AddKey: invalid parameter key, key can only be > 4 or < " + strconv.Itoa(MaxKeyLen))
			return "", errors.New("Error: key can only be of length > 4 and < " + strconv.Itoa(MaxKeyLen) + " and only use the following characters:\n" + customKeyCharset)
		}
		isCustomLink = true
		key = lnk.Key
	}

	// Formatted output for the log
	var logstr []string

	if logger != nil {
		if lnk.IsCompressed {
			decompressed, err := decompress(lnk.Data)
			if err != nil {
				logger.Println("Error while decompressing lnk.Data")
				return "", errors.New(errServerError)
			}
			logstr = append(logstr, "Starting to add lnk:\n   linkType: "+lnk.LinkType+"\n   data: "+url.QueryEscape(decompressed)+"\n   timeout: "+lnk.Timeout.UTC().Format(dateFormat)+"\n   xTimes: "+strconv.Itoa(lnk.Times))
		} else {
			logstr = append(logstr, "Starting to add lnk:\n   linkType: "+lnk.LinkType+"\n   data: "+url.QueryEscape(lnk.Data)+"\n   timeout: "+lnk.Timeout.UTC().Format(dateFormat)+"\n   xTimes: "+strconv.Itoa(lnk.Times))
		}
		if isCustomLink {
			logstr = append(logstr, "\n   l.Links:"+strconv.Itoa(l.Links))
		} else {
			logstr = append(logstr, "\n   len(l.FreeMap):"+strconv.Itoa(len(l.FreeMap)))
		}
		if l.EndClear != nil {
			logstr = append(logstr, "\n   lnk.Timeout:"+lnk.Timeout.UTC().Format(dateFormat)+"\n   l.EndClear.Timeout:"+l.EndClear.Timeout.UTC().Format(dateFormat))
		} else {
			logstr = append(logstr, "\n   lnk.Timeout:"+lnk.Timeout.UTC().Format(dateFormat)+"\n   l.EndClear is nil, will set it to lnk if no other errors occur")
		}
	}

	if (!isCustomLink && len(l.FreeMap) == 0) || (isCustomLink && l.Links >= config.MaxCustomLinks) {
		if logger != nil {
			logger.Println("Error: No keys left")
		}
		if isCustomLink {
			return "", errors.New("No custom links left")
		} else {
			return "", errors.New("No keys left for key length " + strconv.Itoa(len(l.EndClear.Key)))
		}
	}

	if time.Since(lnk.Timeout) > 0 {
		if logger != nil {
			logger.Println("Error, ", logstr, "timeout has to be in the future")
		}
		return "", errors.New(errServerError)
	}

	// if we are adding a specific length key, get the next free key from l.FreeMap
	if !isCustomLink {
		for key = range l.FreeMap {
			break
		}
		if logger != nil {
			logstr = append(logstr, "\n   Picking key:"+key)
		}
		lnk.Key = key
	}

	if l.NextClear == nil {
		l.NextClear = lnk
	} else {
		if l.EndClear == nil {
			if logger != nil {
				logger.Println("Error", logstr, "endClear is nil but nextClear is set to a value")
			}
			return "", errors.New(errServerError)
		}
		if l.EndClear.Timeout.Sub(lnk.Timeout) > 0 {
			if logger != nil {
				logger.Println("Error", logstr, "timeout has to be after the previous links timeout")
			}
			return "", errors.New(errServerError)
		}
		l.EndClear.NextClear = lnk
	}
	l.EndClear = lnk
	l.LinkMap[key] = lnk
	if isCustomLink {
		l.Links++
	} else {
		delete(l.FreeMap, key)
	}
	if logger != nil {
		logstr = append(logstr, "\n   Added key:"+url.QueryEscape(key)+"\n   l.NextClear.Key: "+url.QueryEscape(l.NextClear.Key)+"\n   l.EndClear.Key: "+url.QueryEscape(l.EndClear.Key))
		logger.Println(strings.Join(logstr, ""))
	}
	return key, nil
}

// TimeoutHandler removes links from its linkMap when the links have timed out. Start TimeoutHandler in a separate gorutine and only start one TimeoutHandler() per linkLen.
func (l *LinkLen) TimeoutManager() {
	if logger != nil {
		logger.Println("TimeoutHandler started for", len(l.FreeMap)+len(l.LinkMap), "keys on domain", l.Domain)
	}
	// Check if any new keys should be cleared every 10 seconds
	ticker := time.NewTicker(time.Second * 10)
	// Check if any new keys should be cleared set by l.NextClear.Timeout
	timer := time.NewTimer(time.Second)
	for {
		// block until it is time to clear the next link or to check if l.NextClear has timed out every 10 seconds
		select {
		case <-ticker.C:
		case <-timer.C:
		}
		l.Mutex.RLock()
		if l.NextClear != nil && time.Since(l.NextClear.Timeout) > 0 {
			l.Mutex.RUnlock()
			// Time to clear next link
			l.Mutex.Lock()
			keyToClear := l.NextClear.Key
			if l.NextClear.NextClear != nil && l.NextClear != l.EndClear {
				l.NextClear = l.NextClear.NextClear
				if time.Since(l.NextClear.Timeout) > 0 {
					// if the timeout already passed on nextClear then send a new value on the channel timer.C
					timer.Reset(time.Nanosecond)
				} else {
					timer.Reset(time.Until(l.NextClear.Timeout))
				}
			} else if l.NextClear.NextClear == nil && l.NextClear == l.EndClear {
				l.NextClear = nil
				l.EndClear = nil
			} else {
				if logger != nil {
					logger.Println("ERROR: invalid state, if l.NextClear.NextClear == nil then l.NextClear has to be equal to l.EndClear\nlinkMap:", url.QueryEscape(fmt.Sprint(l.LinkMap)), "\nfreeMap:", url.QueryEscape(fmt.Sprint(l.FreeMap)), "\nnextClear:", url.QueryEscape(fmt.Sprint(l.NextClear)), "\nendClear:", url.QueryEscape(fmt.Sprint(l.EndClear)))
				}
			}
			delete(l.LinkMap, keyToClear)
			if l.FreeMap != nil {
				// Links of specific length
				l.FreeMap[keyToClear] = true
				if logger != nil {
					logger.Println("Finished clearing nextClear of length:", len(keyToClear), "\ncurrently using:", len(l.LinkMap), "keys\ncurrent free keys:", len(l.FreeMap))
				}
			} else {
				// Custom links
				l.Links--
				if logger != nil {
					logger.Println("Finished clearing nextClear for custom link\ncurrently using:", l.Links, "keys\ncurrent free keys:", config.MaxCustomLinks-l.Links)
				}
			}

			l.Mutex.Unlock()
			l.Mutex.RLock()
		}
		l.Mutex.RUnlock()
	}
}
