package main

import (
	"html/template"
	"log"
)

const (
	// charset consists of alphanumeric characters with some characters removed due to them being to similar in some fonts.
	charset = "abcdefghijkmnopqrstuvwxyz23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	// charset consists of characters that are valid for custom keys.
	customKeyCharset = "abcdefghijklmnopqrstuvwxyzåäö0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZÅÄÖ-_"
	// dateFormat specifies the format in which date and time is represented.
	dateFormat = "Mon 2006-01-02 15:04 MST"
	// errServerError contains the generic error message users will se when somthing goes wrong
	errServerError      = "Internal Server Error"
	errInvalidKey       = "Invalid key"
	errInvalidKeyUsed   = "Invalid key, key is already in use"
	errInvalidCustomKey = "Invalid Custom Key was provided, valid characters are:\n" + customKeyCharset
	errNotImplemented   = "Not Implemented"
	errLowRAM           = "No Space available, new space will be available as old links become invalid"
	// Do not try to gzip data that is less than minSizeToGzip
	minSizeToGzip = 128
	// Max key length for custom links
	maxKeyLen = 64
)

var (
	// logSep is a 128bit random number together with a configured log separator string to make it harder to forge log entry's
	logSep string
	// Server config variable
	config Config
	// linkLen1, linkLen2 and linkLen3 will contain all data related to their respective key length and linkCustom will contain all data related to custom keys.
	domainLinkLens map[string]*LinkLens
	// If we want to log errors logger will write these to a file specified in the config
	logger *log.Logger

	// ImageMap is used in handlers.go to map requests to imagedata
	ImageMap map[string][]byte
	// TextBlobs is a temporary map until saving to DB is implemented
	TextBlobs map[string][]byte
	// BackupLinkLen is used to repopulate the database after loading backuped data from a file
	BackupLinkLen1 []Link
	BackupLinkLen2 []Link
	BackupLinkLen3 []Link
	BackupLinkLenC []Link

	templateMap map[string]*template.Template
)
