package main

import (
	"errors"
	"flag"
	"fmt"
	"html"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v2"
)

const (
	debug = true
	// charset consists of alphanumeric characters with some characters removed due to them being to similar in some fonts.
	charset = "abcdefghijkmnopqrstuvwxyz23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	// dateFormat specifies the format in which date and time is represented.
	dateFormat = "Mon 2006-01-02 15:04 MST"
	// logSep sets the seperator between log entrys in the log file, only used for aesthetics purposes, do not rely on this if doing log parsing
	logSep = "\n---\n"
	// errServerError contains the generic error message users will se when somthing goes wrong
	errServerError = "Unexpected server error"
	errInvalidKey  = "Invalid key"
)

// Config contains all valid fields from a shorter config file
type Config struct {
	// TemplateDir should point to the directory containing the template files for shorter
	TemplateDir string `yaml:"TemplateDir"`
	// UploadDir should point to the directory to save temporary files and textblobs
	UploadDir string `yaml:"UploadDir"`
	// Logfile specifies the file to write logs to, if empty or missing, no logging will be done
	Logfile string `yaml:"Logfile"`
	// DomainName should be the domain name of the instance of shorter, e.g. 7i.se
	DomainName string `yaml:"DomainName"`
	// AddressPort specifies the adress and port the shorter service should listen on
	AddressPort string `yaml:"AddressPort"`
	// Clear1Duration should specify the time between clearing old 1 character long URLs.
	// The syntax is 1h20m30s for 1hour 20minutes and 30 seconds
	Clear1Duration time.Duration `yaml:"Clear1Duration"`
	// Clear2Duration, same as Clear1Duration bur for 2 character long URLs
	Clear2Duration time.Duration `yaml:"Clear2Duration"`
	// Clear3Duration, same as Clear1Duration bur for 3 character long URLs
	Clear3Duration time.Duration `yaml:"Clear3Duration"`
	// MaxFileSize specifies the maximum filesize when uploading temporary files
	MaxFileSize int64 `yaml:"MaxFileSize"`
	// MaxDiskUsage specifies how much space in total shorter is allowed to save ondisk
	MaxDiskUsage int64 `yaml:"MaxDiskUsage"`
	// LinkAccessMaxNr specifies how many times a link is allowed to be accessed if xTimes is specified in the request
	LinkAccessMaxNr int `yaml:"LinkAccessMaxNr"`
}

type linkLen struct {
	mutex     sync.RWMutex
	linkMap   map[string]*link
	freeMap   map[string]bool
	nextClear *link // first element in linked list
	endClear  *link // last element in linked list
	timeout   time.Duration
}

// Add adds the value lnk with a new key to linkMap and removes the same key from freeMap and returns the key used or an error, note that the error should be useful for the user while not leak server information
func (l *linkLen) Add(lnk *link) (key string, err error) {
	if lnk == nil {
		if debug && logger != nil {
			logger.Println("Add: invalid parameter lnk, lnk can not be nil", logSep)
		}
		return "", errors.New(errServerError)
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Formated output for the log
	logstr := ""

	if debug && logger != nil {
		logstr = "lnk:\n   linkType: " + lnk.linkType + "\n   data: " + url.QueryEscape(lnk.data) + "\n   timeout: " + lnk.timeout.UTC().Format(dateFormat) + "\n   xTimes: " + strconv.Itoa(lnk.times)
		logger.Println("Starting to Add", logstr)
		logger.Println("len(l.freeMap):", len(l.freeMap))
		if l.endClear != nil {
			logger.Println("lnk.timeout:", lnk.timeout.UTC().Format(dateFormat), "l.endClear.timeout:", l.endClear.timeout.UTC().Format(dateFormat))
		} else {
			logger.Println("lnk.timeout:", lnk.timeout.UTC().Format(dateFormat), "l.endClear is nil, will set it to lnk if no other errors occur")
		}
	}

	if len(l.freeMap) == 0 {
		if debug && logger != nil {
			logger.Println("Error: No keys left", logSep)
		}
		return "", errors.New("No keys left for key length " + strconv.Itoa(len(l.endClear.key)))
	}
	if time.Since(lnk.timeout) > 0 {
		if debug && logger != nil {
			logger.Println("Error, ", logstr, "timeout has to be in the future", logSep)
		}
		return "", errors.New(errServerError)
	}
	for key = range l.freeMap {
		if debug && logger != nil {
			logger.Println("Picking key:", key)
		}
		lnk.key = key
		if l.nextClear == nil {
			l.nextClear = lnk
		} else {
			if l.endClear == nil {
				if debug && logger != nil {
					logger.Println("Error", logstr, "endClear is nil but nextClear is set to a value", logSep)
				}
				return "", errors.New(errServerError)
			}
			if l.endClear.timeout.Sub(lnk.timeout) > 0 {
				if debug && logger != nil {
					logger.Println("Error", logstr, "timeout has to be after the previous links timeout", logSep)
				}
				return "", errors.New(errServerError)
			}
			l.endClear.nextClear = lnk
		}
		l.endClear = lnk
		l.linkMap[key] = lnk
		delete(l.freeMap, key)
		if debug && logger != nil {
			logger.Println("Finished adding key:", key, "with", logstr, "\nl.nextClear.key", l.nextClear.key, "\nl.endClear.key", l.endClear.key, logSep)
		}
		return key, nil
	}
	return
}

// TimeoutHandler removes links from its linkMap when the links have timed out. Start TimeoutHandler in a separate gorutine and only start one TimeoutHandler() per linkLen.
func (l *linkLen) TimeoutHandler() {
	if debug && logger != nil {
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
				if debug && logger != nil {
					logger.Println("ERROR: invalid state, if l.nextClear.nextClear == nil then l.nextClear has to be equal to l.endClear\nlinkMap:", l.linkMap, "\nfreeMap:", l.freeMap, "\nnextClear:", l.nextClear, "\nendClear:", l.endClear, logSep)
				}
			}
			delete(l.linkMap, keyToClear)
			l.freeMap[keyToClear] = true
			if debug && logger != nil {
				logger.Println("Finished clearing nextClear of length:", len(keyToClear), "\ncurrently using:", len(l.linkMap), "keys\ncurrent free keys:", len(l.freeMap), logSep)
				totalkeys := len(l.linkMap) + len(l.freeMap)
				// verify that the number of keys are valid
				if totalkeys != len(charset) && totalkeys != len(charset)*len(charset) && totalkeys != len(charset)*len(charset)*len(charset) {
					logger.Println("ERROR: Unexpected total number of keys:", len(l.linkMap)+len(l.freeMap), logSep)
				}
			}
			l.mutex.Unlock()
			l.mutex.RLock()
		}
		l.mutex.RUnlock()
	}
}

// link tracks the contents and lifetime of a link.
type link struct {
	key       string
	linkType  string
	data      string
	times     int
	timeout   time.Time
	nextClear *link
}

var (
	// Server config variable
	config Config
	// linkLen1, linkLen2 and linkLen3 will contain all data related to their respective key length.
	linkLen1 linkLen
	linkLen2 linkLen
	linkLen3 linkLen
	// If we want to log errors logger will write these to a file specified in the config
	logger *log.Logger
)

func main() {
	// Parse command line arguments.
	var confFile string // confDir specifies the path to config file.
	flag.StringVar(&confFile, "config", filepath.Join(".", "config"), "path to the config file")
	flag.Parse()
	conf, err := ioutil.ReadFile(confFile)
	if err != nil {
		// accept if we specify the path to the config directly without a flag, e.g. shorter /path/to/config
		if len(os.Args) == 2 {
			conf, err = ioutil.ReadFile(os.Args[1])
			if err != nil {
				log.Fatalln("Invalid config file:\n", err)
			}
		} else {
			log.Fatalln("Invalid config file:\n", err)
		}
	}
	if err := yaml.UnmarshalStrict(conf, &config); err != nil {
		log.Fatalln("Unable to parse config file:\n", err)
	}

	if config.Logfile != "" {
		f, err := os.OpenFile(config.Logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
			logger = nil
		}
		defer f.Close()
		logger = log.New(f, "shorter ", log.LstdFlags)
	} else {
		logger = nil
	}

	//  Create index page
	indexTmpl := template.Must(template.ParseFiles(filepath.Join(config.TemplateDir, "index.tmpl")))

	if debug && logger != nil {
		logger.Println("config:\n", config, logSep)
	}

	// init linkLen1, linkLen2, linkLen3 and fill each freeMap with all valid keys for each len
	initLinkLens()

	// Start TimeoutHandlers for all key lengths
	go linkLen1.TimeoutHandler()
	go linkLen2.TimeoutHandler()
	go linkLen3.TimeoutHandler()
	// TODO: find better solution, maybe waitgroup so all TimeoutHandlers have started before starting the server
	time.Sleep(time.Millisecond * 200)

	// Setup handler for web requests
	handler := func(w http.ResponseWriter, r *http.Request) {
		handleRequests(w, r, indexTmpl)
	}
	http.HandleFunc("/", handler)

	// Start server
	if debug && logger != nil {
		logger.Println("Starting server", logSep)
	}
	log.Fatalln(http.ListenAndServe(config.AddressPort, nil))
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

// handleRequests will handle all web requests and direct the right action to the right linkLen
func handleRequests(w http.ResponseWriter, r *http.Request, indexTmpl *template.Template) {
	if debug && logger != nil {
		logger.Println("request:\n", r, logSep)
	}
	if r == nil || indexTmpl == nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}

	// remove / from the beginning of url
	key := r.URL.Path[1:]

	// Return Index page if GET request without a key
	if len(key) == 0 && r.Method == http.MethodGet {
		err := indexTmpl.Execute(w, nil)
		if err != nil {
			if logger != nil {
				logger.Println("Unable to Execute index template", logSep)
			}
			http.Error(w, errServerError, http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodGet && len(key) <= 3 {
		handleGET(w, r, key)
		return
	}

	// If the user tries to submit data via POST
	if r.Method == http.MethodPost {
		err := r.ParseMultipartForm(config.MaxFileSize)
		if err != nil {
			if logger != nil {
				logger.Println("Error: ", err.Error(), url.QueryEscape(fmt.Sprintln(r)), logSep)
			}
			http.Error(w, errServerError, http.StatusInternalServerError)
			return
		}

		// Get length of key to be used
		length := r.Form.Get("len")
		var currentLinkLen *linkLen
		switch length {
		case "1":
			currentLinkLen = &linkLen1
		case "2":
			currentLinkLen = &linkLen2
		case "3":
			currentLinkLen = &linkLen3
		default:
			http.Error(w, errServerError, http.StatusInternalServerError)
			return
		}

		// Get how many times the link can be used before becoming invalid, -1 represents no limit
		xTimes, err := strconv.Atoi(r.Form.Get("xTimes"))
		if err != nil {
			xTimes = -1
		} else {
			if xTimes < 1 {
				xTimes = -1
			} else if xTimes > config.LinkAccessMaxNr {
				xTimes = config.LinkAccessMaxNr
			}
		}

		// Handle diffrent request types
		requestType := r.Form.Get("requestType")
		switch requestType {
		case "url":
			formURL := r.Form.Get("url")
			// simple sanity check to fail early, If len(formURL) is less than 11 it is definitely an invalid url.
			if len(formURL) < 11 || !strings.HasPrefix(formURL, "http://") && !strings.HasPrefix(formURL, "https://") {
				http.Error(w, "Invalid url, only \"http://\" and \"https://\" url schemes are allowed.", http.StatusInternalServerError)
				return
			}
			_, err = url.Parse(formURL)
			if err != nil {
				http.Error(w, "Invalid url", http.StatusInternalServerError)
				return
			}
			currentLinkLen.mutex.RLock()
			currentLinkLenTimeout := currentLinkLen.timeout
			currentLinkLen.mutex.RUnlock()
			newLink := &link{linkType: "url", data: formURL, times: xTimes, timeout: time.Now().Add(currentLinkLenTimeout)}
			key, err := currentLinkLen.Add(newLink)
			if err != nil {
				// if logging is enabled then logs have allready been written from the Add method. Note that the Add method should only return errors that are useful for the user while not leak server information.
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// TODO use template to make a better looking output
			fmt.Fprint(w, config.DomainName+"/"+key+"/ now pointing to "+html.EscapeString(formURL)+" \nThis link will be removed "+newLink.timeout.UTC().Format(dateFormat)+" ("+currentLinkLenTimeout.String()+" from now)")
			return
		case "text":
			fmt.Fprint(w, "Not implemented")
			return
		case "file":
			fmt.Fprint(w, "Not implemented")
			return
		default:
			http.Error(w, errServerError, http.StatusInternalServerError)
			return
		}
	}
	// If the request is not handled previously redirect to index
	http.Redirect(w, r, config.DomainName, http.StatusSeeOther)
}

// Not a crypto related function so no need for constant time
func validate(s string) bool {
	for _, char := range s {
		if !strings.Contains(charset, string(char)) {
			return false
		}
	}
	return true
}

// handleGET will handle GET requests and redirect to the saved link for a key, return a saved textblob or return a file
func handleGET(w http.ResponseWriter, r *http.Request, key string) {
	if !validate(key) {
		http.Error(w, errInvalidKey, http.StatusInternalServerError)
		return
	}

	var lnk *link
	var ok bool
	switch len(key) {
	case 1:
		if lnk, ok = linkLen1.linkMap[key]; !ok {
			http.Error(w, errInvalidKey, http.StatusInternalServerError)
			return
		}
	case 2:
		if lnk, ok = linkLen2.linkMap[key]; !ok {
			http.Error(w, errInvalidKey, http.StatusInternalServerError)
			return
		}
	case 3:
		if lnk, ok = linkLen3.linkMap[key]; !ok {
			http.Error(w, errInvalidKey, http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, errInvalidKey, http.StatusInternalServerError)
		return
	}

	switch lnk.linkType {
	case "url":
		http.Redirect(w, r, lnk.data, http.StatusTemporaryRedirect)
		return
	case "text":
		fmt.Fprint(w, "Not implemented")
		return
	case "file":
		fmt.Fprint(w, "Not implemented")
		return
	default:
		http.Error(w, errServerError, http.StatusInternalServerError)
	}
}
