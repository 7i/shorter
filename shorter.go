package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	yaml "gopkg.in/yaml.v2"
)

func main() {
	// mount "/7i/shorterdata"
	MountShorterData()

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
			configPath := findFolderDefaultLocations("shorterdata")
			if configPath != "" {
				conf, err = ioutil.ReadFile(filepath.Join(configPath, "config"))
				if err != nil {
					log.Fatalln("Invalid config file:\n", err)
				}
			}
		}
	}

	// Populate the global config variable with the data from the config file
	if err := yaml.UnmarshalStrict(conf, &config); err != nil {
		log.Fatalln("Unable to parse config file:\n", err)
	}

	// if BaseDir is not specified in the config search for a directory named shorterdata in the current directory and if not found search for a directory "src/github.com/7i/shorter/shorterdata" under all paths specified in GOPATH
	if config.BaseDir == "" {
		dataPath := findFolderDefaultLocations("shorterdata")
		if dataPath != "" {
			config.BaseDir = dataPath
		} else {
			log.Fatalln("Unable to locate a valid BaseDir, please specify BaseDir in the shorter config file")
		}
	}

	if config.Logging {
		var f *os.File
		if config.Logfile != "" {
			f, err = os.OpenFile(config.Logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		} else {
			f, err = os.OpenFile(filepath.Join(config.BaseDir, "shorter.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		}
		if err != nil {
			log.Println(err)
			logger = nil
		}
		defer f.Close()
		logger = log.New(f, "shorter ", log.LstdFlags)
	}

	// Set up DB file so we can resume state if server goes down
	//setupDB() // defined in db.go

	// Write out server config on startup if logging is enabled
	if logger != nil {
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

	mux := http.NewServeMux()

	// Handle requests to /sjcl.js
	handleCSS(mux)    // defined in handlers.go
	handleJS(mux)     // defined in handlers.go
	handleImages(mux) // defined in handlers.go
	handleRobots(mux) // defined in handlers.go
	handleRoot(mux)   // defined in handlers.go
	// Start server
	if logger != nil {
		logger.Println("Starting server", logSep)
	}
	// if NoTLS is set only start a http server
	if config.NoTLS {
		log.Fatalln(http.ListenAndServe(config.AddressPort, mux))
	}
	server := getServer(mux) // defined in letsencrypt.go
	// Using LetsEncrypt, no premade cert and keyfiles needed
	log.Fatalln(server.ListenAndServeTLS("", ""))
}
