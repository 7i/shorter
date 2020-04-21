package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/boltdb/bolt"
	yaml "gopkg.in/yaml.v2"
)

func main() {
	var conf []byte
	var err error
	// accept if we specify the path to the config directly without a flag, e.g. shorter /path/to/config
	if len(os.Args) == 2 {
		conf, err = ioutil.ReadFile(os.Args[1])
		if err != nil {
			log.Fatalln("Invalid config file:\n", err)
		}
	} else {
		// Parse command line arguments.
		var confFile string // confDir specifies the path to config file.
		flag.StringVar(&confFile, "config", filepath.Join(".", "config"), "path to the config file")
		flag.Parse()
		conf, err = ioutil.ReadFile(confFile)
		if err != nil {
			log.Fatalln("Invalid config file:\n", err)
		}
	}
	// Populate the global config variable with the data from the config file
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

	// create bolt db file
	db, err := bolt.Open(filepath.Join(config.BackupDBDir, "shorter.db"), 0600, nil)
	if err != nil {
		log.Fatalln("Unable to open Backup database file", err)
	}
	defer db.Close()

	//  Create index page
	indexTmpl := template.Must(template.ParseFiles(filepath.Join(config.TemplateDir, "index.tmpl")))

	if debug && logger != nil {
		logger.Println("config:\n", config, logSep)
	}

	// init linkLen1, linkLen2, linkLen3 and fill each freeMap with all valid keys for each len. Defined in misc.go
	initLinkLens()

	// Start TimeoutManager for all key lengths. Defined in types.go
	go linkLen1.TimeoutManager()
	go linkLen2.TimeoutManager()
	go linkLen3.TimeoutManager()
	// TODO: find better solution, maybe waitgroup so all TimeoutManager have started before starting the server
	time.Sleep(time.Millisecond * 200)

	// Handle requests to sjcl.js
	f, err := ioutil.ReadFile(filepath.Join(config.TemplateDir, "sjcl.js"))
	if err != nil {
		log.Fatalln("Missing sjcl.js in Template dir")
	}
	handlejsfile := func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", f)
	}
	http.HandleFunc("/sjcl.js", handlejsfile)

	// Setup handler for web requests
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Defined in handlers.go
		handleRequests(w, r, indexTmpl)
	}
	http.HandleFunc("/", handler)

	// Start server
	if debug && logger != nil {
		logger.Println("Starting server", logSep)
	}
	log.Fatalln(http.ListenAndServe(config.AddressPort, nil))
}
