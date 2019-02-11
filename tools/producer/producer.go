package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	log "github.com/sirupsen/logrus"
)

type Record struct {
	Name     string
	Type     string
	Content  string
	Ttl      int
	Priority int
}

func main() {
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg) != 5 {
		fmt.Printf("Register a record DNS in the local kafka node\n\nUSAGE: producer name type content ttl priority")
		os.Exit(1)
	}

	name := argsWithoutProg[0]
	qtype := argsWithoutProg[1]
	ttl, _ := strconv.Atoi(argsWithoutProg[3])
	priority, _ := strconv.Atoi(argsWithoutProg[4])

	record := []Record{
		Record{
			name,
			qtype,
			argsWithoutProg[2],
			ttl,
			priority,
		},
	}

	recordsJson, _ := json.Marshal(record)

	topic := "records"
	partition := 0

	conn, _ := kafka.DialLeader(context.Background(), "tcp", "localhost:9092", topic, partition)

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := conn.WriteMessages(kafka.Message{
		Key: []byte(name + ".|" + qtype),
		Value: recordsJson,
	})

	if err != nil {
		log.Fatal(err.Error())
	} else {
		log.Info("records posted")
	}

	conn.Close()
}
