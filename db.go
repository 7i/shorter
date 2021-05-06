package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/boltdb/bolt"
)

func setupDB() {
	var err error
	if config.BackupDBDir == "" {
		db, err = bolt.Open(filepath.Join(config.BaseDir, "shorter.db"), 0600, nil)
	} else {
		db, err = bolt.Open(filepath.Join(config.BackupDBDir, "shorter.db"), 0600, nil)
	}
	if err != nil {
		log.Fatalln("Unable to open Backup database file", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucketIfNotExists([]byte("len1"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucketIfNotExists([]byte("len2"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucketIfNotExists([]byte("len3"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucketIfNotExists([]byte("customKeys"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}

	logger.Println("setupDB finished init of len1-3 and custom keys")
}
