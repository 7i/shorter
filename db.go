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

// New BoltDB restore
//startRestoreDB(&domainLinkLens[domain].LinkLen1, domain, "linkLen1")
//startRestoreDB(&domainLinkLens[domain].LinkLen2, domain, "linkLen2")
//startRestoreDB(&domainLinkLens[domain].LinkLen3, domain, "linkLen3")
//startRestoreDB(&domainLinkLens[domain].LinkCustom, domain, "linkCustom")
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
// new bolt implementation of backup ============
// Config2 type

/*
type Config2 struct {
	Height   float64   `json:"height"`
	Birthday time.Time `json:"birthday"`
}

// Entry type
type Entry struct {
	Calories int    `json:"calories"`
	Food     string `json:"food"`
}

func startRestoreDB(l *LinkLen, domain, linkLen string) {
	db := setupDB2(domain)
	defer db.Close()
	restoreDBLinkLen(db, domain)
}

// restoreDBLinkLen will read out all links for all linkLen from the bolt.DB for the specified domain and populate the domainLinkLens map
func restoreDBLinkLen(db *bolt.DB, domain string) {
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(domain)).Bucket([]byte("linkLen1"))
		b.ForEach(func(k, v []byte) error {
			// TODO Restore entries from DB HERE

			var lnk Link
			err := json.Unmarshal(v, &lnk)
			if err != nil {
				logger.Fatalln("Unable to restore link,", err)
			}
			l1 := &domainLinkLens[domain].LinkLen1

			l1.Add(&lnk)



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

			fmt.Println(string(k), string(v))
			return nil
		})
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func setupDB2(domain string) *bolt.DB {

	db, err := bolt.Open(filepath.Join(config.BaseDir, domain, domain+".db"), 0600, nil)

	if err != nil && logger != nil {
		logger.Fatalln("could not open db,", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists([]byte(domain))
		if err != nil {
			logger.Fatalln("could not create root bucket:", err)
		}
		_, err = root.CreateBucketIfNotExists([]byte("linkLen1"))
		if err != nil {
			logger.Fatalln("could not create linkLen1 bucket:", err)
		}
		_, err = root.CreateBucketIfNotExists([]byte("linkLen2"))
		if err != nil {
			logger.Fatalln("could not create linkLen2 bucket:", err)
		}
		_, err = root.CreateBucketIfNotExists([]byte("linkLen3"))
		if err != nil {
			logger.Fatalln("could not create linkLen3 bucket:", err)
		}
		_, err = root.CreateBucketIfNotExists([]byte("linkCustom"))
		if err != nil {
			logger.Fatalln("could not create linkCustom bucket:", err)
		}
		return nil
	})
	if err != nil {
		logger.Fatalln("could not set up buckets,", err)
	}
	logger.Println("")
	fmt.Println("DB Setup for", domain, "Done")
	return db
}

func setConfig2(db *bolt.DB, Config2 Config2) error {
	confBytes, err := json.Marshal(Config2)
	if err != nil {
		return fmt.Errorf("could not marshal Config2 json: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		err = tx.Bucket([]byte("DB")).Put([]byte("CONFIG"), confBytes)
		if err != nil {
			return fmt.Errorf("could not set Config2: %v", err)
		}
		return nil
	})
	fmt.Println("Set Config2")
	return err
}

func addWeight(db *bolt.DB, weight string, date time.Time) error {
	err := db.Update(func(tx *bolt.Tx) error {
		err := tx.Bucket([]byte("DB")).Bucket([]byte("WEIGHT")).Put([]byte(date.Format(time.RFC3339)), []byte(weight))
		if err != nil {
			return fmt.Errorf("could not insert weight: %v", err)
		}
		return nil
	})
	fmt.Println("Added Weight")
	return err
}

func addEntry(db *bolt.DB, calories int, food string, date time.Time) error {
	entry := Entry{Calories: calories, Food: food}
	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("could not marshal entry json: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		err := tx.Bucket([]byte("DB")).Bucket([]byte("ENTRIES")).Put([]byte(date.Format(time.RFC3339)), entryBytes)
		if err != nil {
			return fmt.Errorf("could not insert entry: %v", err)
		}

		return nil
	})
	fmt.Println("Added Entry")
	return err
}
*/
