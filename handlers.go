package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"html"
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
	handler := func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w, r)
		if validRequest(r) {
			handleRequests(w, r)
		} else {
			http.Error(w, errServerError, http.StatusInternalServerError)
		}
	}
	mux.HandleFunc("/", handler)
}

// handleRequests will handle all web requests and direct the right action to the right linkLen
func handleRequests(w http.ResponseWriter, r *http.Request) {
	if r == nil {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "invalid request")
		return
	}

	// browsers should send a path that begins with a /
	if r.URL.Path[0] != '/' {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "")
		return
	}

	if r.Method == http.MethodGet {
		handleGET(w, r)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	// If the user tries to submit data via POST
	if r.Method == http.MethodPost {
		err := r.ParseMultipartForm(config.MaxFileSize)
		if err != nil {
			logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: "+url.QueryEscape(err.Error()))
			return
		}

		// Get length of key to be used
		length := r.Form.Get("len")
		var currentLinkLen *LinkLen
		switch length {
		case "1":
			currentLinkLen = &domainLinkLens[r.Host].LinkLen1
		case "2":
			currentLinkLen = &domainLinkLens[r.Host].LinkLen2
		case "3":
			currentLinkLen = &domainLinkLens[r.Host].LinkLen3
		case "custom":
			currentLinkLen = &domainLinkLens[r.Host].LinkCustom
		default:
			logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: Invalid len argument.")
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

		// Check if request is a custom key request and report error if it is invalid
		customKey := ""
		if length == "custom" {
			customKey = r.Form.Get("custom")
			if !validate(customKey) || len(customKey) < 4 || len(customKey) > MaxKeyLen {
				logErrors(w, r, errInvalidCustomKey, http.StatusInternalServerError, "")
				return
			}

			if _, used := domainLinkLens[r.Host].LinkCustom.LinkMap[customKey]; used {
				http.Error(w, errInvalidKeyUsed, http.StatusInternalServerError)
				return
			}
		}

		// Handle different request types
		requestType := r.Form.Get("requestType")
		switch requestType {
		case "url":
			formURL := r.Form.Get("url")
			valid := validURL(formURL)
			if !valid {
				logErrors(w, r, "Invalid url, only \"http://\" and \"https://\" url schemes are allowed.", http.StatusInternalServerError, "")
				return
			}
			currentLinkLen.Mutex.RLock()
			currentLinkLenTimeout := currentLinkLen.Timeout
			currentLinkLen.Mutex.RUnlock()

			isCompressed := false

			showLnk := &Link{Key: customKey, LinkType: "url", Data: formURL, IsCompressed: isCompressed, Times: xTimes, Timeout: time.Now().Add(currentLinkLenTimeout)}
			key, err := currentLinkLen.Add(showLnk)
			if err == nil {
				w.Header().Add("Content-Type", "text/html; charset=utf-8")
				logger.Println("requesting template :", r.Host+"showLink")
				t, ok := templateMap[r.Host+"#showLink"]
				if !ok {
					logger.Println("ERROR getting template template :", r.Host+"showLink")
					http.Error(w, errServerError, http.StatusInternalServerError)
					return
				}

				tmplArgs := showLinkVars{Domain: scheme + "://" + r.Host, Data: scheme + "://" + r.Host + "/" + key, Timeout: showLnk.Timeout.Format("Mon 2006-01-02 15:04 MST")}

				err = t.ExecuteTemplate(w, "showLink.tmpl", tmplArgs)
				if err != nil {
					logger.Println("ERROR executing template template showLink.tmpl for host :", r.Host, "with args: ", tmplArgs, "with the error: ", err)
					http.Error(w, errServerError, http.StatusInternalServerError)
				}
				logOK(r, http.StatusOK)
				return
			}
			return
		case "text":
			if lowRAM() {
				logErrors(w, r, errServerError, http.StatusInternalServerError, errLowRAM)
				return
			}
			textBlob := r.Form.Get("text")

			isCompressed := false
			if len(textBlob) > minSizeToGzip {
				compressed, err := compress(textBlob)
				if err == nil && len(textBlob) > len(compressed) {
					textBlob = compressed
					isCompressed = true
				}
			}

			currentLinkLen.Mutex.RLock()
			currentLinkLenTimeout := currentLinkLen.Timeout
			currentLinkLen.Mutex.RUnlock()

			showLnk := &Link{Key: customKey, LinkType: "text", Data: textBlob, IsCompressed: isCompressed, Times: xTimes, Timeout: time.Now().Add(currentLinkLenTimeout)}
			key, err := currentLinkLen.Add(showLnk)
			if err == nil {
				w.Header().Add("Content-Type", "text/html; charset=utf-8")
				t, ok := templateMap[r.Host+"#showLink"]
				if !ok {
					http.Error(w, errServerError, http.StatusInternalServerError)
					return
				}
				tmplArgs := showLinkVars{Domain: scheme + "://" + r.Host, Data: scheme + "://" + r.Host + "/" + key, Timeout: showLnk.Timeout.Format("Mon 2006-01-02 15:04 MST")}

				err = t.ExecuteTemplate(w, "showLink.tmpl", tmplArgs)
				if err != nil {
					logger.Println("ERROR executing template template showLink.tmpl for host :", r.Host, "with args: ", tmplArgs)
					http.Error(w, errServerError, http.StatusInternalServerError)
				}
				logOK(r, http.StatusOK)
				return
			}
			return
		default:
			logErrors(w, r, errNotImplemented, http.StatusNotImplemented, "Error: Invalid requestType argument.")
			return
		}
	}

	// If the request is not handled previously redirect to index, note that Host has been validated earlier
	logOK(r, http.StatusSeeOther)
	http.Redirect(w, r, scheme+"://"+r.Host, http.StatusSeeOther)
}

// handleGET will handle GET requests and redirect to the saved link for a key, return a saved textblob or return a file
func handleGET(w http.ResponseWriter, r *http.Request) {

	if !validRequest(r) {
		logErrors(w, r, errServerError, http.StatusInternalServerError, "Error: invalid request.")
		return
	}

	// remove / from the beginning of url and remove any character after the key
	key := r.URL.Path[1:]
	extradataindex := strings.IndexAny(key, "/")
	if extradataindex >= 0 {
		key = key[:extradataindex]
	}

	// Return Index page if GET request without a key
	if len(key) == 0 {
		indexTmpl, ok := templateMap[r.Host+"#index"]
		if !ok || indexTmpl == nil {
			logErrors(w, r, errServerError, http.StatusInternalServerError, "Unable to load index template: "+r.Host+"#index")
			return
		}
		err := indexTmpl.Execute(w, nil)
		if err != nil {
			logErrors(w, r, errServerError, http.StatusInternalServerError, "Unable to Execute index template: "+r.Host+"#index")
			return
		}
		logOK(r, http.StatusOK)
		return
	}

	// verify that key only consists of valid characters
	if !validate(key) {
		logErrors(w, r, errInvalidKey, http.StatusInternalServerError, "")
		return
	}

	// quick check if request is quickAddURL request
	if len(r.URL.RawQuery) > 0 {
		if key == "listactive~" {
			listActiveLinks(w, r)
			return
		}
		if validURL(r.URL.RawQuery) {
			quickAddURL(w, r, r.URL.RawQuery, key)
			return
		} else {
			logErrors(w, r, "Invalid Quick Add URL request", http.StatusInternalServerError, "Invalid Quick Add URL request, please use the following syntax: \""+r.Host+"?http://example.com/\". where http://example.com/ is your link.\nAlso note that only \"http://\" and \"https://\" url schemes are allowed.")
			return
		}
	}

	var showLink bool
	if key[len(key)-1] == '~' {
		key = key[:len(key)-1]
		showLink = true
	}

	// start by checking static key map
	if lnk, ok := config.StaticLinks[key]; ok {
		logOK(r, http.StatusPermanentRedirect)
		http.Redirect(w, r, lnk, http.StatusPermanentRedirect)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	var lnk *Link
	var ok bool
	switch keylen := len(key); {
	case keylen == 1:
		domainLinkLens[r.Host].LinkLen1.Mutex.RLock()
		if lnk, ok = domainLinkLens[r.Host].LinkLen1.LinkMap[key]; !ok {
			domainLinkLens[r.Host].LinkLen1.Mutex.RUnlock()
			http.Error(w, errInvalidKey, http.StatusInternalServerError)
			return
		}
		domainLinkLens[r.Host].LinkLen1.Mutex.RUnlock()
	case keylen == 2:
		domainLinkLens[r.Host].LinkLen2.Mutex.RLock()
		if lnk, ok = domainLinkLens[r.Host].LinkLen2.LinkMap[key]; !ok {
			domainLinkLens[r.Host].LinkLen2.Mutex.RUnlock()
			http.Error(w, errInvalidKey, http.StatusInternalServerError)
			return
		}
		domainLinkLens[r.Host].LinkLen2.Mutex.RUnlock()
	case keylen == 3:
		domainLinkLens[r.Host].LinkLen3.Mutex.RLock()
		if lnk, ok = domainLinkLens[r.Host].LinkLen3.LinkMap[key]; !ok {
			domainLinkLens[r.Host].LinkLen3.Mutex.RUnlock()
			http.Error(w, errInvalidKey, http.StatusInternalServerError)
			return
		}
		domainLinkLens[r.Host].LinkLen3.Mutex.RUnlock()
	case keylen > 3 && keylen < MaxKeyLen:
		// key is validated previously
		domainLinkLens[r.Host].LinkCustom.Mutex.RLock()
		if lnk, ok = domainLinkLens[r.Host].LinkCustom.LinkMap[key]; !ok {
			domainLinkLens[r.Host].LinkCustom.Mutex.RUnlock()
			http.Error(w, errInvalidKey, http.StatusInternalServerError)
			return
		}
		domainLinkLens[r.Host].LinkCustom.Mutex.RUnlock()
	default:
		http.Error(w, errInvalidKey, http.StatusInternalServerError)
		return
	}

	if lnk == nil {
		http.Error(w, errInvalidKey, http.StatusInternalServerError)
		return
	}

	switch lnk.LinkType {
	case "url":
		if showLink {
			logOK(r, http.StatusOK)
			w.Header().Add("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, r.Host+"/"+key+"\n\nis pointing to \n\n"+html.EscapeString(lnk.Data))
			return
		}
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
		t, ok := templateMap[r.Host+"#showLink"]
		if !ok {
			http.Error(w, errServerError, http.StatusInternalServerError)
		}
		tmplArgs := showLinkVars{Domain: scheme + "://" + r.Host, Data: lnk.Data, Timeout: lnk.Timeout.Format("Mon 2006-01-02 15:04 MST")}
		err := t.ExecuteTemplate(w, "showLink.tmpl", tmplArgs)
		if err != nil {
			http.Error(w, errServerError, http.StatusInternalServerError)
		}
		logOK(r, http.StatusTemporaryRedirect)
		return
	case "text":
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		if showLink {
			logOK(r, http.StatusOK)
			fmt.Fprint(w, r.Host+"/"+key+"\n\nis pointing to a "+r.Host+" Text dump")
			return
		}
		if lnk.IsCompressed {
			if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				w.Header().Add("content-encoding", "gzip")
				logOK(r, http.StatusOK)
				fmt.Fprint(w, lnk.Data)
				return
			} else {
				returnDecompressed(lnk, w, r) // defined in misc.go
				return
			}
		}
		logOK(r, http.StatusOK)
		fmt.Fprint(w, lnk.Data)
		return
	default:
		logErrors(w, r, errServerError, http.StatusInternalServerError, "invalid LinkType "+url.QueryEscape(lnk.LinkType))
	}
}

func handleCSS(mux *http.ServeMux) {
	f, err := ioutil.ReadFile(filepath.Join(config.BaseDir, "css", "shorter.css"))
	if err != nil {
		log.Fatalln("Missing shorter.css in Template dir/css/")
	}

	mux.HandleFunc("/shorter.css", getSingleFileHandler(f, "text/css"))
}

func getSingleFileHandler(f []byte, mimeType string) (handleFile func(w http.ResponseWriter, r *http.Request)) {
	var buf bytes.Buffer
	tryGzip := true
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(f)
	if err != nil {
		tryGzip = false
	}
	zw.Close()
	cf := buf.Bytes()

	handleFile = func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w, r)
		if validRequest(r) {
			w.Header().Add("Content-Type", mimeType)
			w.Header().Add("Cache-Control", "max-age=2592000, public")
			if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && tryGzip && false {
				w.Header().Add("content-encoding", "gzip")
				fmt.Fprintf(w, "%s", cf)
				return
			}
			fmt.Fprintf(w, "%s", f)
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
	}
	return
}

func getImgHandler(img string, mimeType string) (handleImgFile func(w http.ResponseWriter, r *http.Request)) {
	handleImgFile = func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w, r)
		if validRequest(r) {
			w.Header().Add("Content-Type", mimeType)
			w.Header().Add("Cache-Control", "max-age=2592000, public")
			fmt.Fprintf(w, "%s", ImageMap[r.Host+img])
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
	}
	return
}

// handleImages adds /logo.png, /favicon.ico and /favicon.png to all domains specified in config, if a domain is missing a image it will fall back to the default image
func handleImages(mux *http.ServeMux) {
	ImageMap = make(map[string][]byte)

	defaultLogo := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x80, 0x04, 0x03, 0x00, 0x00, 0x00, 0x31, 0x10, 0x7c, 0xf8, 0x00, 0x00, 0x00, 0x0f, 0x50, 0x4c, 0x54, 0x45, 0x00, 0x00, 0x00, 0x17, 0x9c, 0xf2, 0x8a, 0xc2, 0xf3, 0xb7, 0xda, 0xf8, 0xfd, 0xff, 0xfc, 0x73, 0x3f, 0xef, 0xad, 0x00, 0x00, 0x00, 0x01, 0x74, 0x52, 0x4e, 0x53, 0x00, 0x40, 0xe6, 0xd8, 0x66, 0x00, 0x00, 0x02, 0x10, 0x49, 0x44, 0x41, 0x54, 0x68, 0xde, 0xed, 0xda, 0x5d, 0x76, 0x82, 0x30, 0x10, 0x86, 0x61, 0xbb, 0x83, 0x26, 0xb0, 0x01, 0x94, 0x0d, 0x50, 0xdd, 0x00, 0x98, 0xfd, 0xaf, 0xa9, 0xca, 0x4f, 0x14, 0x4f, 0x66, 0xbe, 0x2f, 0x33, 0x17, 0xde, 0x90, 0x3b, 0x7b, 0xe0, 0x39, 0xe1, 0x4d, 0xe0, 0xd4, 0xd2, 0xd3, 0xe9, 0x39, 0x82, 0x69, 0x9c, 0xb6, 0xf1, 0x63, 0x3b, 0x3f, 0x84, 0x5f, 0xe7, 0xf9, 0x9b, 0x60, 0x3f, 0x7f, 0xb9, 0x0a, 0xc7, 0x04, 0x96, 0x29, 0x78, 0xce, 0x9f, 0xa7, 0xe0, 0x05, 0x5c, 0x57, 0xf0, 0xbc, 0x86, 0xef, 0x03, 0xbe, 0xf3, 0x1f, 0x11, 0x0e, 0xe0, 0x00, 0xac, 0xc0, 0xf9, 0xdc, 0x79, 0x80, 0x78, 0x4b, 0x29, 0xfd, 0xd9, 0x81, 0x98, 0xe6, 0x31, 0x99, 0x81, 0x7e, 0x01, 0x52, 0x67, 0x04, 0xd6, 0x09, 0xe4, 0x29, 0x2c, 0x40, 0x3c, 0x33, 0x63, 0x3e, 0xb4, 0xdd, 0x80, 0xf4, 0x0e, 0x34, 0x89, 0x18, 0xd3, 0xee, 0x0a, 0xb6, 0x6b, 0xa8, 0x00, 0xc6, 0xf9, 0xd0, 0x5b, 0xfe, 0x3c, 0xd4, 0x02, 0x83, 0x17, 0x08, 0x4e, 0x60, 0xf2, 0x02, 0x63, 0xf8, 0x88, 0x58, 0x0b, 0xac, 0x1b, 0xc7, 0xbe, 0x8c, 0xeb, 0x46, 0xca, 0x87, 0x4e, 0x95, 0xc0, 0xb6, 0xf7, 0x73, 0x84, 0xa1, 0x12, 0x18, 0xc3, 0x7e, 0x0a, 0xfb, 0x9b, 0x89, 0x00, 0x5e, 0x4f, 0x80, 0xcb, 0xf3, 0xe3, 0xbd, 0xab, 0x04, 0xee, 0xef, 0xf7, 0xd3, 0xf5, 0x9a, 0x1f, 0x07, 0x34, 0x30, 0x05, 0x61, 0xb0, 0xc0, 0xa0, 0x03, 0x8f, 0x49, 0x95, 0x47, 0x21, 0x41, 0x11, 0x90, 0x46, 0x7e, 0x7a, 0xdc, 0x83, 0x0d, 0x68, 0x60, 0x02, 0x00, 0xf4, 0x30, 0x01, 0x00, 0x70, 0x02, 0x1d, 0x20, 0x12, 0xe8, 0x40, 0x5b, 0x4a, 0x70, 0x59, 0x17, 0x88, 0x79, 0xac, 0x17, 0x13, 0xf4, 0xfb, 0x9f, 0xa9, 0x40, 0x4e, 0x10, 0x6c, 0x40, 0x79, 0x11, 0x2b, 0x80, 0x7c, 0x05, 0xa3, 0x11, 0xb8, 0x95, 0x12, 0x54, 0x00, 0xb1, 0x98, 0xa0, 0x02, 0x10, 0xf6, 0x31, 0x0f, 0x94, 0x13, 0x54, 0x00, 0xe5, 0x04, 0x3c, 0x20, 0x24, 0xe0, 0x81, 0xb6, 0x9c, 0x80, 0x07, 0x84, 0x04, 0x3c, 0x20, 0xdd, 0xca, 0x2c, 0x90, 0x17, 0xf1, 0xf3, 0x56, 0x66, 0x81, 0x5e, 0x48, 0x40, 0x03, 0xc2, 0x22, 0xd2, 0x40, 0x94, 0x12, 0xb0, 0x80, 0x98, 0x80, 0x05, 0xc4, 0x04, 0x2c, 0x20, 0x26, 0x20, 0x01, 0x39, 0x01, 0x09, 0xb4, 0x62, 0x02, 0x12, 0x90, 0x13, 0x90, 0x40, 0x12, 0x13, 0x70, 0xc0, 0xeb, 0xf7, 0x85, 0x60, 0x03, 0x94, 0x04, 0x1c, 0x90, 0x17, 0x71, 0xb4, 0x01, 0x51, 0x49, 0x40, 0x01, 0x5a, 0x02, 0x0a, 0x50, 0x16, 0x91, 0x03, 0xb4, 0x04, 0x0c, 0xa0, 0xec, 0x63, 0x0e, 0x50, 0x13, 0x30, 0x80, 0x9a, 0x80, 0x01, 0x92, 0x96, 0x80, 0x00, 0x1a, 0x35, 0x01, 0x01, 0x68, 0xfb, 0x98, 0x02, 0xf4, 0x04, 0x04, 0xa0, 0xed, 0x63, 0x06, 0x00, 0x09, 0x30, 0xd0, 0xeb, 0x09, 0x30, 0x70, 0xd3, 0x13, 0x40, 0x20, 0x82, 0x04, 0x10, 0x40, 0x09, 0x20, 0x80, 0x12, 0x40, 0x20, 0x81, 0x04, 0x08, 0x80, 0x09, 0x10, 0xf0, 0xf9, 0xe5, 0xbc, 0x1a, 0x80, 0x09, 0x10, 0xa0, 0xdf, 0xca, 0x18, 0x68, 0x60, 0x02, 0x00, 0xf4, 0x30, 0x01, 0x00, 0xd0, 0x3e, 0x46, 0x40, 0xc4, 0x09, 0x74, 0x80, 0x48, 0xa0, 0x03, 0x44, 0x02, 0x1d, 0x20, 0x12, 0xa8, 0x00, 0x93, 0x40, 0x05, 0x5e, 0xfb, 0xb8, 0xb3, 0x01, 0x78, 0x1f, 0x03, 0x00, 0xde, 0xca, 0x00, 0x60, 0x16, 0x51, 0x07, 0xf2, 0xdf, 0x5e, 0x94, 0x04, 0x55, 0x5f, 0xff, 0x99, 0x71, 0x00, 0x07, 0x30, 0x03, 0xdf, 0x7f, 0xdb, 0xe7, 0x06, 0xbe, 0xff, 0xce, 0xd5, 0xff, 0xda, 0xd8, 0xfd, 0xe2, 0xda, 0xff, 0xea, 0xdc, 0xfd, 0xf2, 0xde, 0xf7, 0xef, 0x03, 0xff, 0x2b, 0xec, 0x86, 0x52, 0x86, 0x8e, 0xac, 0x41, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}
	defaultFavicon := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10, 0x08, 0x03, 0x00, 0x00, 0x00, 0x28, 0x2d, 0x0f, 0x53, 0x00, 0x00, 0x00, 0x9c, 0x50, 0x4c, 0x54, 0x45, 0x1f, 0x9b, 0xed, 0x1f, 0x9b, 0xef, 0x1e, 0x9a, 0xed, 0x1e, 0x9c, 0xed, 0x1f, 0x9b, 0xee, 0x1f, 0x9c, 0xef, 0x20, 0x9c, 0xee, 0x20, 0x9c, 0xee, 0x21, 0x9c, 0xee, 0x23, 0x9d, 0xee, 0x26, 0x9e, 0xee, 0x28, 0x9f, 0xee, 0x2a, 0xa0, 0xee, 0x2e, 0xa2, 0xef, 0x31, 0xa3, 0xef, 0x37, 0xa6, 0xef, 0x39, 0xa7, 0xef, 0x45, 0xac, 0xf0, 0x55, 0xb3, 0xf1, 0x5e, 0xb7, 0xf2, 0x62, 0xb9, 0xf3, 0x63, 0xb9, 0xf2, 0x65, 0xba, 0xf2, 0x6e, 0xbe, 0xf3, 0x77, 0xc2, 0xf4, 0x78, 0xc2, 0xf4, 0x81, 0xc7, 0xf5, 0x87, 0xc9, 0xf5, 0x8a, 0xca, 0xf5, 0x8b, 0xcb, 0xf5, 0x91, 0xce, 0xf6, 0x96, 0xd0, 0xf6, 0x99, 0xd1, 0xf6, 0x9b, 0xd2, 0xf6, 0x9b, 0xd2, 0xf7, 0x9d, 0xd3, 0xf7, 0x9f, 0xd4, 0xf7, 0xb9, 0xe0, 0xf9, 0xcb, 0xe7, 0xfa, 0xd7, 0xed, 0xfb, 0xda, 0xee, 0xfb, 0xdf, 0xf0, 0xfc, 0xe5, 0xf3, 0xfc, 0xe7, 0xf4, 0xfc, 0xeb, 0xf6, 0xfd, 0xed, 0xf7, 0xfd, 0xf0, 0xf8, 0xfd, 0xf1, 0xf8, 0xfd, 0xf2, 0xf9, 0xfd, 0xf5, 0xfa, 0xfe, 0xf9, 0xfc, 0xfe, 0xff, 0xff, 0xff, 0x7a, 0x52, 0xe8, 0x58, 0x00, 0x00, 0x00, 0x07, 0x74, 0x52, 0x4e, 0x53, 0x7d, 0x7d, 0x7e, 0x7e, 0xf8, 0xf8, 0xf9, 0x01, 0xb6, 0xcf, 0xc8, 0x00, 0x00, 0x00, 0x7e, 0x49, 0x44, 0x41, 0x54, 0x18, 0x57, 0x55, 0xcf, 0xc7, 0x12, 0x82, 0x40, 0x10, 0x84, 0xe1, 0x51, 0x59, 0x7f, 0xd7, 0x84, 0x62, 0x00, 0x23, 0x06, 0xcc, 0x71, 0x9d, 0xf7, 0x7f, 0x37, 0x2f, 0x50, 0x35, 0xf4, 0xad, 0xbf, 0xaa, 0x3e, 0xb4, 0xb4, 0x1c, 0x26, 0xae, 0x21, 0x6d, 0xdb, 0x21, 0x12, 0xdb, 0x26, 0xdb, 0x18, 0x21, 0x7f, 0x95, 0x59, 0xf1, 0xd1, 0x3d, 0xc2, 0x21, 0x84, 0x10, 0xc2, 0x4f, 0xdf, 0x03, 0x66, 0xf9, 0x88, 0x6a, 0x72, 0xd6, 0x0d, 0x24, 0x69, 0x5c, 0xc1, 0x5c, 0x9f, 0x7d, 0x38, 0xe9, 0xb4, 0x04, 0x7f, 0xd5, 0x25, 0x16, 0x32, 0xbd, 0x77, 0x2d, 0xf4, 0x1e, 0xba, 0xc0, 0xc2, 0x5a, 0x6f, 0xde, 0xc2, 0xf0, 0xab, 0x29, 0x16, 0x8e, 0x7a, 0xe9, 0x58, 0xf0, 0xbb, 0x22, 0x01, 0x80, 0xac, 0x18, 0x23, 0xb5, 0xb3, 0xe0, 0xa4, 0x59, 0x93, 0x48, 0xfe, 0x29, 0x72, 0x10, 0x99, 0xc7, 0x5c, 0x2b, 0x48, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}

	for _, domain := range config.DomainNames {
		logo, err := ioutil.ReadFile(filepath.Join(config.BaseDir, domain, "logo.png"))
		if err != nil {
			if logger != nil {
				logger.Println("Missing /" + domain + "/logo.png in Template dir, fallback to default logo.png")
			}
			ImageMap[domain+"-logo"] = defaultLogo
		} else {
			ImageMap[domain+"-logo"] = logo
		}

		favicon, err := ioutil.ReadFile(filepath.Join(config.BaseDir, domain, "favicon.png"))
		if err != nil {
			if logger != nil {
				logger.Println("Missing /" + domain + "/favicon.png in Template dir, fallback to default favicon.png")
			}
			ImageMap[domain+"-favicon"] = defaultFavicon
		} else {
			ImageMap[domain+"-favicon"] = favicon
		}
	}

	mux.HandleFunc("/logo.png", getImgHandler("-logo", "image/png"))
	mux.HandleFunc("/favicon.png", getImgHandler("-favicon", "image/png"))
	mux.HandleFunc("/favicon.ico", getImgHandler("-favicon", "image/png"))
}

// handleRobots will return the robots.txt located in the Template dir specified in the config file, if no robots.txt file is found we return a 404 error
func handleRobots(mux *http.ServeMux) {
	f, err := ioutil.ReadFile(filepath.Join(config.BaseDir, "robots.txt"))
	if err != nil {
		if logger != nil {
			logger.Println("Missing robots.txt in Template dir, fallback to returning 404 on requests for robots.txt")
		}
		handler404 := func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Not Found", http.StatusNotFound)
		}
		mux.HandleFunc("/robots.txt", handler404)
		return
	}
	handleRobots := func(w http.ResponseWriter, r *http.Request) {
		addHeaders(w, r)
		if validRequest(r) {
			w.Header().Add("Content-Type", "text/plain; charset=utf-8")
			w.Header().Add("Cache-Control", "max-age=2592000, public")
			fmt.Fprintf(w, "%s", f)
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
	}
	mux.HandleFunc("/robots.txt", handleRobots)
}

func quickAddURL(w http.ResponseWriter, r *http.Request, url, key string) {
	var urlLink *LinkLen

	// Remove keys of invalid size, note that key has been validated to only contain valid characters previously
	if len(key) <= 3 || len(key) >= MaxKeyLen {
		key = ""
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	// Try to quickAddURL for first len 1, if all are full then try len 2 and lastly len 3
	for i := 0; i <= 3; i++ {
		switch i {
		case 0:
			if key == "" {
				continue
			}
			urlLink = &domainLinkLens[r.Host].LinkCustom
			if _, used := urlLink.LinkMap[key]; used {
				http.Error(w, errInvalidKeyUsed, http.StatusInternalServerError)
				return
			}
		case 1:
			urlLink = &domainLinkLens[r.Host].LinkLen1
		case 2:
			urlLink = &domainLinkLens[r.Host].LinkLen2
		case 3:
			urlLink = &domainLinkLens[r.Host].LinkLen3
		default:
		}

		urlLink.Mutex.RLock()
		linkTimeout := urlLink.Timeout
		urlLink.Mutex.RUnlock()

		isCompressed := false

		showLink := &Link{Key: key, LinkType: "url", Data: url, IsCompressed: isCompressed, Times: -1, Timeout: time.Now().Add(linkTimeout)}
		_, err := urlLink.Add(showLink)
		if err == nil {
			w.Header().Add("Content-Type", "text/html; charset=utf-8")
			t, ok := templateMap[r.Host+"#showLink"]
			if !ok {
				http.Error(w, errServerError, http.StatusInternalServerError)
				return
			}

			tmplArgs := showLinkVars{Domain: scheme + "://" + r.Host, Data: showLink.Data, Timeout: showLink.Timeout.Format("Mon 2006-01-02 15:04 MST")}
			err = t.ExecuteTemplate(w, "showLink.tmpl", tmplArgs)
			if err != nil {
				http.Error(w, errServerError, http.StatusInternalServerError)
			}
			logOK(r, http.StatusOK)
			return
		}
	}
}
