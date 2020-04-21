package main

import "log"

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
