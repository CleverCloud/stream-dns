package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	u "stream-dns/utils"

	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

type HttpAdministrator struct {
	db        *bolt.DB
	address   string
	servermux *http.ServeMux
}

func NewHttpAdministrator(db *bolt.DB, address string) *HttpAdministrator {
	s := HttpAdministrator{
		db:        db,
		address:   address,
		servermux: http.NewServeMux(),
	}

	s.servermux.HandleFunc("/tools/dnsrecords/search", s.searchRecords)

	return &s
}

func (h *HttpAdministrator) StartHttpAdministrator() error {
	log.Info("Administrator running on ", fmt.Sprintf("http://%s", h.address))
	err := http.ListenAndServe(h.address, h.servermux)

	if err != nil {
		log.Fatal(err)
		return err
	}

	return nil
}

// Get the list of records that match a pattern
// We just look if the pattern (substring) is contains in the domain within the key
// curl -X GET http://<address>/tools/dnsrecords/search?pattern=<pattern>
func (h *HttpAdministrator) searchRecords(w http.ResponseWriter, r *http.Request) {

	queryParams := r.URL.Query()
	pattern := queryParams.Get("pattern")

	if r.Method != http.MethodGet {
		http.Error(w, "only method GET allowed", http.StatusMethodNotAllowed)
		return
	}

	if pattern == "" {
		http.Error(w, "missing pattern", http.StatusBadRequest)
		return
	}

	var records [][]Record

	h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("records"))

		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			domain, _ := u.ExtractQnameAndQtypeFromConsumerKey(k)

			if strings.Contains(domain, pattern) {
				var record []Record
				err := json.Unmarshal([]byte(v), &record)

				if err != nil {
					return err
				}
				records = append(records, record)
			}
		}

		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(records)
}
