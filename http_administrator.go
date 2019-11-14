package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	u "stream-dns/utils"

	"github.com/dgrijalva/jwt-go"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

type HttpAdministrator struct {
	db        *bolt.DB
	jwtSecret []byte
	creds     Credentials
	address   string
	servermux *http.ServeMux
}

type Credentials struct {
	Password string `json:"password"`
	Username string `json:"username"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.StandardClaims
}

func NewHttpAdministrator(db *bolt.DB, config AdministratorConfig) *HttpAdministrator {
	creds := Credentials{Username: config.Username, Password: config.Password}

	s := HttpAdministrator{
		db:        db,
		creds:     creds,
		jwtSecret: []byte(config.JwtSecret),
		address:   config.Address,
		servermux: http.NewServeMux(),
	}

	s.servermux.HandleFunc("/signin", s.signin)
	s.servermux.HandleFunc("/search", s.searchRecords)

	return &s
}

func (h *HttpAdministrator) StartHttpAdministrator() error {
	log.Infof("Administrator running on http://%s", h.address)
	err := http.ListenAndServe(h.address, h.servermux)

	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func (h *HttpAdministrator) signin(w http.ResponseWriter, r *http.Request) {
	log.Infof("Got new signin request %s at %s", requestToString(r), time.Now().Format(time.UnixDate))
	var creds Credentials

	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		log.Errorf("Administrator deserialization error on creds: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if creds.Username != h.creds.Username || creds.Password != h.creds.Password {
		log.Infof("Unauthorized signin tried by %s at %s", creds.Username, time.Now().Format(time.UnixDate))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	expirationTime := time.Now().Add(1 * time.Hour)

	claims := &Claims{
		Username: creds.Username,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "token",
		Value:   tokenString,
		Expires: expirationTime,
	})
}

// Get the list of records that match a pattern
// We just look if the pattern (substring) is contains in the domain within the key
// curl -X GET http://<address>/tools/dnsrecords/search?pattern=<pattern>
func (h *HttpAdministrator) searchRecords(w http.ResponseWriter, r *http.Request) {
	log.Infof("Got new administrator search request %s", requestToString(r))

	if h.creds.Password != "" && h.creds.Username != "" {
		c, err := r.Cookie("token")
		if err != nil {
			if err == http.ErrNoCookie {
				log.Errorf("Missing jwt token for %s", requestToString(r))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			log.Errorf("bad request for %s", requestToString(r))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if isAuthJwtTokenValide(c, h.jwtSecret) == false {
			log.Errorf("Unauthorized %s for %s at %s", r.RemoteAddr, requestToString(r), time.Now().Format(time.UnixDate))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

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

	var rrsRaw []PairKeyRRraw

	h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(RecordBucket)

		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			domain, _ := u.ExtractQnameAndQtypeFromKey(k)

			if strings.Contains(domain, pattern) {
				rrsRaw = append(rrsRaw, PairKeyRRraw{k, v})
			}
		}

		return nil
	})

	rrs := [][]dns.RR{}

	for _, rrRaw := range rrsRaw {
		rrstmp, err := mapPairKeyRawRRsIntoRR(rrRaw)

		//TODO: improve this
		if err != nil {
			log.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		log.Info(rrstmp)

		rrs = append(rrs, rrstmp)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(rrs)
}

//FIXME: Manage the http.StatusBadRequest too.
func isAuthJwtTokenValide(cookie *http.Cookie, jwtSecret []byte) bool {
	tknStr := cookie.Value
	claims := &Claims{}

	tkn, err := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return false
	}

	if !tkn.Valid {
		return false
	}

	return true
}

func requestToString(r *http.Request) string {
	return fmt.Sprintf("%s %s %s", r.Method, r.URL.String(), r.Proto)
}
