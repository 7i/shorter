package main

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

// Fugly solution, TODO switch to real DB like bolt
func setupDB() {

	if logger != nil {
		logger.Println("Reading in links and data from db")
	}
	for _, domain := range config.DomainNames {
		restoreLinkLen(&domainLinkLens[domain].LinkLen1, "len1", domain)
		restoreLinkLen(&domainLinkLens[domain].LinkLen2, "len2", domain)
		restoreLinkLen(&domainLinkLens[domain].LinkLen3, "len3", domain)
		restoreLinkLen(&domainLinkLens[domain].LinkCustom, "custom", domain)
	}
}

func restoreLinkLen(l *LinkLen, typ, domain string) {
	var backupLinkLen []Link
	fileName := "backupdb-" + domain + "-" + typ + ".gob"

	d, err := ioutil.ReadFile(filepath.Join(config.BaseDir, domain, fileName))
	if err != nil && logger != nil {
		logger.Println(err, "ReadFile - Skipping "+fileName)
	} else {
		buf := bytes.NewBuffer(d)
		dec := gob.NewDecoder(buf)
		err := dec.Decode(&backupLinkLen)
		if err != nil && logger != nil {
			logger.Println(err, "Unmarshal - Skipping"+fileName)
		} else {
			if len(backupLinkLen) > 0 && backupLinkLen[0].Key != "" {
				l.NextClear = &backupLinkLen[0]
				l.EndClear = &backupLinkLen[len(backupLinkLen)-1]
				l.Links = len(backupLinkLen)
				l.LinkMap[backupLinkLen[0].Key] = &backupLinkLen[0]
				delete(l.FreeMap, backupLinkLen[0].Key)
			}
			for i := 1; i < len(backupLinkLen); i++ {
				if backupLinkLen[i].Key != "" {
					l.LinkMap[backupLinkLen[i].Key] = &backupLinkLen[i]
					backupLinkLen[i-1].NextClear = &backupLinkLen[i]
					delete(l.FreeMap, backupLinkLen[i].Key)
				}
			}
		}
	}
}

// part 2 of the fugly solution
func BackupRoutine() {

	for {
		time.Sleep(time.Minute * 30)

		for _, domain := range config.DomainNames {
			saveBackup(&domainLinkLens[domain].LinkLen1, "len1", domain)
			saveBackup(&domainLinkLens[domain].LinkLen2, "len2", domain)
			saveBackup(&domainLinkLens[domain].LinkLen3, "len3", domain)
			saveBackup(&domainLinkLens[domain].LinkCustom, "custom", domain)
		}

		logger.Println("Finished saving new backup")
	}
}

func saveBackup(l *LinkLen, typ, domain string) {
	var err error
	var backupLinkLen []Link
	filename := "backupdb-" + domain + "-" + typ + ".gob"

	if l == nil {
		logger.Println("*LinkLen is nil, skipping ", filename)
		return
	}
	if l.NextClear == nil {
		logger.Println("l.NextClear is nil, skipping ", filename)
		return
	}

	l.Mutex.Lock()

	next := *l.NextClear

	stop := false
	for !stop {
		backupLinkLen = append(backupLinkLen, next)
		if next.NextClear != nil {
			next = *next.NextClear
		} else {
			stop = true
		}
	}
	l.Mutex.Unlock()

	var backupBuffer bytes.Buffer
	enc := gob.NewEncoder(&backupBuffer)
	err = enc.Encode(backupLinkLen)
	if err != nil {
		logger.Println(err, "Error while saving backup in enc.Encode()")
	}

	backupLinkLen = nil

	if err = os.WriteFile(filepath.Join(config.BaseDir, domain, filename), backupBuffer.Bytes(), 0644); err != nil && logger != nil {
		logger.Println(err, "failed to save DB")
	}

	logger.Println("Backed up:", filename)
}
