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
	//  Create index page
	indexTmpl := template.Must(template.ParseFiles(filepath.Join(config.TemplateDir, "index.tmpl")))

	handler := func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL)
		// Defined in handlers.go
		addHeaders(w)
		handleRequests(w, r, indexTmpl)
	}
	mux.HandleFunc("/", handler)
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

func handleSJCL(mux *http.ServeMux) {
	f, err := ioutil.ReadFile(filepath.Join(config.TemplateDir, "sjcl.js"))
	if err != nil {
		log.Fatalln("Missing sjcl.js in Template dir")
	}
	handlejsfile := func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w)
		fmt.Fprintf(w, "%s", f)
	}
	mux.HandleFunc("/sjcl.js", handlejsfile)
}
