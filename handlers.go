package main

import (
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
	"time"
)

func handleRoot(mux *http.ServeMux) {
	templateMap := make(map[string]*template.Template)
	//  Create index page
	defaultTmpl := template.Must(template.ParseFiles(filepath.Join(config.BaseDir, "index.tmpl")))

	for _, domain := range config.DomainNames {
		template, err := template.ParseFiles(filepath.Join(config.BaseDir, domain, "index.tmpl"))
		if err != nil {
			if debug && logger != nil {
				logger.Println("Missing /"+domain+"/index.tmpl in Template dir, fallback to default index.tmpl", logSep)
			}
			templateMap[domain] = defaultTmpl
		} else {
			templateMap[domain] = template
		}
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		// Defined in handlers.go
		addHeaders(w)
		if validRequest(r) {
			handleRequests(w, r, templateMap[r.Host])
		} else {
			http.Error(w, errServerError, http.StatusInternalServerError)
		}
	}
	mux.HandleFunc("/", handler)
}

// handleRequests will handle all web requests and direct the right action to the right linkLen
func handleRequests(w http.ResponseWriter, r *http.Request, indexTmpl *template.Template) {
	if r == nil || indexTmpl == nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if debug && logger != nil {
		logger.Println("request:\n", r.Host+r.RequestURI, "\n", r, logSep)
	}

	// remove / from the beginning of url and remove any character after the key
	key := r.URL.Path[1:]
	extradataindex := strings.IndexAny(key, "/?")
	if extradataindex > 0 {
		key = key[:extradataindex]
	}

	// Return Index page if GET request without a key
	if len(key) == 0 && r.Method == http.MethodGet {
		err := indexTmpl.Execute(w, nil)
		if err != nil {
			if logger != nil {
				logger.Println("Unable to Execute index template.\nRequest:\n", r.Host+r.RequestURI, "\n", r, logSep)
			}
			http.Error(w, errServerError, http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodGet {
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

		// Handle different request types
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
				// if logging is enabled then logs have already been written from the Add method. Note that the Add method should only return errors that are useful for the user while not leak server information.
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// TODO use template to make a better looking output, default template and optional templates for each domain
			// Note that r.Host has been validated earlier
			fmt.Fprint(w, "https://"+r.Host+"/"+key+"/ now pointing to "+html.EscapeString(formURL)+" \nThis link will be removed "+newLink.timeout.UTC().Format(dateFormat)+" ("+currentLinkLenTimeout.String()+" from now)")
			return
		case "text":
			http.Error(w, errNotImplemented, http.StatusNotImplemented)
			return
		case "file":
			http.Error(w, errNotImplemented, http.StatusNotImplemented)
			return
		default:
			http.Error(w, errServerError, http.StatusInternalServerError)
			return
		}
	}

	// If the request is not handled previously redirect to index, note that Host has been validated earlier
	http.Redirect(w, r, "https://"+r.Host, http.StatusSeeOther)
}

// handleGET will handle GET requests and redirect to the saved link for a key, return a saved textblob or return a file
func handleGET(w http.ResponseWriter, r *http.Request, key string) {
	if !validRequest(r) {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if !validate(key) {
		http.Error(w, errInvalidKey, http.StatusInternalServerError)
		return
	}
	var showLink bool
	if key[len(key)-1] == '~' {
		key = key[:len(key)-1]
		showLink = true
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
		if showLink {
			fmt.Fprint(w, "https://"+r.Host+"/"+key+"/ is pointing to "+html.EscapeString(lnk.data))
			return
		}
		http.Redirect(w, r, lnk.data, http.StatusTemporaryRedirect)
		return
	case "text":
		http.Error(w, errNotImplemented, http.StatusNotImplemented)
		return
	case "file":
		http.Error(w, errNotImplemented, http.StatusNotImplemented)
		return
	default:
		http.Error(w, errServerError, http.StatusInternalServerError)
	}
}

func handleJS(mux *http.ServeMux) {
	f, err := ioutil.ReadFile(filepath.Join(config.BaseDir, "sjcl.js"))
	if err != nil {
		log.Fatalln("Missing sjcl.js in Template dir")
	}
	handlejsfile := func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w)
		if validRequest(r) {
			w.Header().Add("Content-Type", "text/javascript")
			w.Header().Add("Cache-Control", "max-age=2592000, public")
			fmt.Fprintf(w, "%s", f)
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
	}
	mux.HandleFunc("/sjcl.js", handlejsfile)
}

// handleImages adds /logo.png, /favicon.ico and /favicon.png to all domains specified in config, if a domain is missing a image it will fall back to the default image
func handleImages(mux *http.ServeMux) {
	ImageMap := make(map[string][]byte)

	defaultLogo, err := ioutil.ReadFile(filepath.Join(config.BaseDir, "logo.png"))
	if err != nil {
		log.Fatalln("Missing logo.png in Template dir")
	}
	defaultFavicon, err := ioutil.ReadFile(filepath.Join(config.BaseDir, "favicon.png"))
	if err != nil {
		log.Fatalln("Missing favicon.png in Template dir")
	}

	for _, domain := range config.DomainNames {
		logo, err := ioutil.ReadFile(filepath.Join(config.BaseDir, domain, "logo.png"))
		if err != nil {
			if debug && logger != nil {
				logger.Println("Missing /"+domain+"/logo.png in Template dir, fallback to default logo.png", logSep)
			}
			ImageMap[domain+"-logo"] = defaultLogo
		} else {
			ImageMap[domain+"-logo"] = logo
		}

		favicon, err := ioutil.ReadFile(filepath.Join(config.BaseDir, domain, "favicon.png"))
		if err != nil {
			if debug && logger != nil {
				logger.Println("Missing /"+domain+"/favicon.png in Template dir, fallback to default favicon.png", logSep)
			}
			ImageMap[domain+"-favicon"] = defaultFavicon
		} else {
			ImageMap[domain+"-favicon"] = favicon
		}
	}

	handleLogoFile := func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w)
		if validRequest(r) {
			w.Header().Add("Content-Type", "image/png")
			w.Header().Add("Cache-Control", "max-age=2592000, public")
			fmt.Fprintf(w, "%s", ImageMap[r.Host+"-logo"])
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
	}

	handleFaviconFile := func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w)
		if validRequest(r) {
			w.Header().Add("Content-Type", "image/png")
			w.Header().Add("Cache-Control", "max-age=2592000, public")
			fmt.Fprintf(w, "%s", ImageMap[r.Host+"-favicon"])
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
	}

	mux.HandleFunc("/logo.png", handleLogoFile)
	mux.HandleFunc("/favicon.png", handleFaviconFile)
	mux.HandleFunc("/favicon.ico", handleFaviconFile)
}

// handleRobots will return the robots.txt located in the Template dir specified in the config file, if no robots.txt file is found we return a 404 error
func handleRobots(mux *http.ServeMux) {
	f, err := ioutil.ReadFile(filepath.Join(config.BaseDir, "robots.txt"))
	if err != nil {
		if debug && logger != nil {
			logger.Println("Missing robots.txt in Template dir, fallback to returning 404 on requests for robots.txt", logSep)
		}
		handler404 := func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Not Found", http.StatusNotFound)
		}
		mux.HandleFunc("/robots.txt", handler404)
		return
	}
	handleRobots := func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w)
		if validRequest(r) {
			w.Header().Add("Cache-Control", "max-age=2592000, public")
			fmt.Fprintf(w, "%s", f)
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
	}
	mux.HandleFunc("/robots.txt", handleRobots)
}
