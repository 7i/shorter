package main

import (
	"log"

	"github.com/boltdb/bolt"
)

const (
	// charset consists of alphanumeric characters with some characters removed due to them being to similar in some fonts.
	charset = "abcdefghijkmnopqrstuvwxyz23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	// charset consists of characters that are valid for custom keys.
	customKeyCharset = "abcdefghijklmnopqrstuvwxyzåäö0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZÅÄÖ-_"
	// dateFormat specifies the format in which date and time is represented.
	dateFormat = "Mon 2006-01-02 15:04 MST"
	// logSep sets the separator between log entrys in the log file, only used for aesthetics purposes.
	// do not rely on this if doing log parsing.
	// TODO, add secure log entrys (generate seperator from sha256(key+time+lognr) and/or header for all entrys with lenght specified and/or signed log entrys so we can verify all entrys.)
	logSep = "\n---\n"
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
	MaxKeyLen = 64
)

var (
	// Server config variable
	config Config
	// linkLen1, linkLen2 and linkLen3 will contain all data related to their respective key length and linkCustom will contain all data related to custom keys.
	linkLen1   linkLen
	linkLen2   linkLen
	linkLen3   linkLen
	linkCustom linkLen
	// If we want to log errors logger will write these to a file specified in the config
	logger *log.Logger
	//
	db *bolt.DB
	// ImageMap is used in handlers.go to map requests to imagedata
	ImageMap map[string][]byte
	// TextBlobs is a temporary map untill saving to DB is implemented
	TextBlobs map[string][]byte
)
