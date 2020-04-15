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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	yaml "gopkg.in/yaml.v2"
)

const (
	// charset consists of alphanumeric characters with some characters removed due to them being to similar in some fonts.
	charset       = "abcdefghijkmnopqrstuvwxyz23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	mutationsLen1 = len(charset)
	mutationsLen2 = len(charset) * len(charset)
	mutationsLen3 = len(charset) * len(charset) * len(charset)
)

const debug = true

// dateFormat specifies the format in which date and time is represented.
const dateFormat = "Mon 2006-01-02 15:04 MST"

// Config contains all valid fields from a shorter config file
type Config struct {
	// TemplateDir should point to the directory containing the template files for shorter
	TemplateDir string `yaml:"TemplateDir"`
	// UploadDir should point to the directory to save temporary files and textblobs
	UploadDir string `yaml:"UploadDir"`
	// DomainName should be the domain name of the instance of shorter, e.g. 7i.se
	DomainName string `yaml:"DomainName"`
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

// Add adds the value lnk with a new key to linkMap and removes the same key from freeMap and returns the key used or an error
func (l *linkLen) Add(lnk *link) (key string, err error) {
	if lnk == nil {
		return "", errors.New("invalid parameter lnk, lnk can not be nil")
	}

	if debug {
		log.Println("Starting to Add link:\n", lnk)
		log.Println("len(l.freeMap):", len(l.freeMap))
		if l.endClear != nil {
			log.Println("lnk.timeout:", lnk.timeout.Format(dateFormat), "l.endClear.timeout:", l.endClear.timeout.Format(dateFormat))
		} else {
			log.Println("lnk.timeout:", lnk.timeout.Format(dateFormat), "l.endClear is nil, will set it to lnk if no other errors occur")
		}
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	if len(l.freeMap) == 0 {
		return "", errors.New("no keys available at this time")
	}
	if time.Since(lnk.timeout) > 0 {
		return "", errors.New("invalid link, timeout has to be in the future")
	}
	for key = range l.freeMap {
		if debug {
			log.Println("Picking key:", key)
		}
		lnk.key = key
		if l.nextClear == nil {
			l.nextClear = lnk
		} else {
			if l.endClear == nil {
				return "", errors.New("unexpected server error, endClear is nil but nextClear is set to a value")
			}
			if l.endClear.timeout.Sub(lnk.timeout) > 0 {
				return "", errors.New("invalid link, timeout has to be after the previous links timeout")
			}
			l.endClear.nextClear = lnk
		}
		l.endClear = lnk
		l.linkMap[key] = lnk
		if debug {
			log.Println("lnk:", key)
			log.Println("l.endClear:", l.endClear)
			log.Println("l.nextClear:", l.nextClear)
		}
		delete(l.freeMap, key)
		if debug {
			log.Println("Finished adding key:", key, "with value of link:\n", lnk)
		}
		return key, nil
	}
	return key, nil
}

// TimeoutHandler removes links from its linkMap when the links have timed out. Start TimeoutHandler in a separate gorutine and only start one TimeoutHandler() per linkLen.
func (l *linkLen) TimeoutHandler() {
	if debug {
		log.Println("TimeoutHandler started for", len(l.freeMap), "keys")
	}
	ticker := time.NewTicker(time.Second)
	for {
		<-ticker.C // check if it is time to clear the next link
		if l.nextClear != nil && time.Since(l.nextClear.timeout) > 0 {
			// Time to clear next link
			l.mutex.Lock()
			keyToClear := l.nextClear.key
			if l.nextClear.nextClear != nil && l.nextClear != l.endClear {
				l.nextClear = l.nextClear.nextClear
			} else if l.nextClear.nextClear == nil && l.nextClear == l.endClear {
				l.nextClear = nil
				l.endClear = nil
			} else {
				log.Println("ERROR: invalid state, if l.nextClear.nextClear == nil then l.nextClear has to be equal to l.endClear\nlinkMap:", l.linkMap, "\nfreeMap:", l.freeMap, "\nnextClear:", l.nextClear, "\nendClear:", l.endClear)
			}
			delete(l.linkMap, keyToClear)
			l.freeMap[keyToClear] = true
			if debug {
				log.Println("Finished clearing nextClear of length")
				log.Println("currently using:", len(l.linkMap), "keys")
				log.Println("current free keys:", len(l.freeMap))
				totalkeys := len(l.linkMap) + len(l.freeMap)
				if totalkeys != mutationsLen1 && totalkeys != mutationsLen2 && totalkeys != mutationsLen3 {
					log.Println("ERROR: Unexpected total number of keys:", len(l.linkMap)+len(l.freeMap))
				}
			}
			l.mutex.Unlock()
		}
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

// Server config variable
var config Config

var (
	linkLen1 linkLen
	linkLen2 linkLen
	linkLen3 linkLen
)

func main() {
	// Parse command line arguments.
	var (
		// address to listen on.
		addr string
		// path to config directory.
		confDir string
	)
	flag.StringVar(&addr, "addr", "127.0.0.1:8080", "address to listen on")
	flag.StringVar(&confDir, "config", ".", "path to the config directory")
	flag.Parse()
	conf, err := ioutil.ReadFile(filepath.Join(confDir, "config"))
	if err != nil {
		log.Fatalln("Invalid config file:\n", err)
	}
	var readOps uint64
	atomic.AddUint64(&readOps, 1)
	if err := yaml.UnmarshalStrict(conf, &config); err != nil {
		log.Fatalln("Unable to parse config file:\n", err)
	}
	if debug {
		log.Println("config:\n", config)
	}

	//  Create index page
	indexTmpl := template.Must(template.ParseFiles(filepath.Join(config.TemplateDir, "index.tmpl")))

	// init linkLen1, linkLen2, linkLen3 and fill each freeMap with all valid keys for each len
	initLinkLens()

	// Start TimeoutHandlers for all key lengths
	go linkLen1.TimeoutHandler()
	go linkLen2.TimeoutHandler()
	go linkLen3.TimeoutHandler()

	// Setup handler for web requests
	handler := func(w http.ResponseWriter, r *http.Request) {
		handleRequests(w, r, indexTmpl)
	}
	http.HandleFunc("/", handler)

	// Start server
	if debug {
		log.Println("Starting server")
	}
	log.Fatal(http.ListenAndServe(addr, nil))
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
	if debug {
		log.Println("All maps initialized")
	}
}

// handleRequests will handle all web requests and direct the right action to the right linkLen
func handleRequests(w http.ResponseWriter, r *http.Request, indexTmpl *template.Template) {
	if debug {
		log.Println("request:\n", r)
	}
	if r == nil || indexTmpl == nil {
		http.Error(w, "Unexpected server error", http.StatusInternalServerError)
		return
	}

	// remove / from the beginning of url
	key := r.URL.Path[1:]

	// Return Index page if GET request without a key
	if len(key) == 0 && r.Method == http.MethodGet {
		err := indexTmpl.Execute(w, nil)
		if err != nil {
			log.Fatalln(err)
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
			// TODO: consider logging error to disk, and only output static error message to use. This is to prevent information disclosure.
			http.Error(w, "Unexpected server error: "+err.Error(), http.StatusInternalServerError)
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
			http.Error(w, "Invalid request type", http.StatusInternalServerError)
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
			newLink := &link{linkType: "url", data: formURL, times: xTimes, timeout: time.Now().Add(currentLinkLen.timeout)}
			key, err := currentLinkLen.Add(newLink)
			if err != nil {
				// NOTE: as all errors of Add are controlled by us, this is not an information disclosure. However, it could become if e.g. file handling was added to the Add method, and we return an error from the os. To limit this risk, we could define specific errors that are intended for the user, e.g. ErrNoKeysLeft, ErrTimeout, etc. Each of these would have static error messages. Then for any other error we just log to disk, and return internal server error to the user.
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			fmt.Fprint(w, config.DomainName+"/"+key+"/ now pointing to "+html.EscapeString(formURL)+" \nThis link will be removed "+newLink.timeout.UTC().Format(dateFormat)+" ("+currentLinkLen.timeout.String()+" from now)")
		case "text":
			fmt.Fprint(w, "Not implemented")
		case "file":
			fmt.Fprint(w, "Not implemented")
		default:
			http.Error(w, "Invalid request type", http.StatusInternalServerError)
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
		http.Error(w, "Invalid key", http.StatusInternalServerError)
		return
	}

	var lnk *link
	var ok bool
	switch len(key) {
	case 1:
		if lnk, ok = linkLen1.linkMap[key]; !ok {
			http.Error(w, "Invalid key", http.StatusInternalServerError)
			return
		}
	case 2:
		if lnk, ok = linkLen2.linkMap[key]; !ok {
			http.Error(w, "Invalid key", http.StatusInternalServerError)
			return
		}
	case 3:
		if lnk, ok = linkLen3.linkMap[key]; !ok {
			http.Error(w, "Invalid key", http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Invalid key", http.StatusInternalServerError)
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
		http.Error(w, "Invalid linkType", http.StatusInternalServerError)
	}
}
