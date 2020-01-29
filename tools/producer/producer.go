package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/miekg/dns"
	"github.com/segmentio/kafka-go"
	log "github.com/sirupsen/logrus"
)

type TimeStamp int64

type Record struct {
	Name      string
	Type      string
	Content   string
	Ttl       int
	Priority  int
	Metadatas Metadatas `json:",omitempty"`
}

type Metadatas struct {
	CreatedAt TimeStamp // UNIX timestap when the record was created
	Producer  string    // Name of the record producer
}

func main() {
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg) != 7 {
		fmt.Printf("Register a record DNS in the local kafka node\n\nUSAGE: ./producer broker name type content ttl priority producer")
		os.Exit(1)
	}

	name := dns.Fqdn(argsWithoutProg[1])
	qtype := strings.ToUpper(argsWithoutProg[2])
	ttl, _ := strconv.Atoi(argsWithoutProg[4])
	priority, _ := strconv.Atoi(argsWithoutProg[5])
	metadatas := Metadatas{TimeStamp(time.Now().Unix()), argsWithoutProg[6]}

	record := []Record{
		Record{
			name,
			qtype,
			argsWithoutProg[3],
			ttl,
			priority,
			metadatas,
		},
	}

	key := name + "|" + qtype
	recordsJSON, err := json.Marshal(record)

	if err != nil {
		log.Error(err)
	}

	if argsWithoutProg[0] == "kafka" {
		err = sendToKafka(key, recordsJSON)
	} else if argsWithoutProg[0] == "pulsar" {
		err = sendToPulsar(key, recordsJSON)
	} else {
		log.Errorf("Incorrect broker type name, expected kafka|pulsar but got: %s", argsWithoutProg[0])
		return
	}

	if err != nil {
		log.Errorf("Failed to publish message: %s", err)
	} else {
		log.Info("Published message")
	}
}

func sendToKafka(key string, record []byte) error {
	topic := "records"
	partition := 0
	addr := "localhost:9092"

	log.Infof("Trying to connect at %s and the topic %s", addr, topic)

	conn, err := kafka.DialLeader(context.Background(), "tcp", addr, topic, partition)

	if err != nil {
		return err
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err = conn.WriteMessages(kafka.Message{
		Key:   []byte(key),
		Value: record,
	})

	if err != nil {
		return err
	} else {
		log.Info("records posted")
	}

	conn.Close()

	return nil
}

func sendToPulsar(key string, record []byte) error {
	topic := "records"
	addr := "pulsar://localhost:6650"

	log.Infof("Trying to connect at %s and the topic %s", addr, topic)

	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: addr,
	})

	defer client.Close()

	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})

	if err != nil {
		return err
	}

	_, err = producer.Send(context.Background(), &pulsar.ProducerMessage{
		Key:     key,
		Payload: record,
	})

	defer producer.Close()

	if err != nil {
		return err
	}

	return nil
}
