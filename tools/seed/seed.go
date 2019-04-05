// Use with env variable TOPIC and BROKER_ADDRESS
package main

import (
	"encoding/json"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
)

type Record struct {
	Name     string
	Type     string
	Content  string
	Ttl      int
	Priority int
}

type RecordsEncoder []Record

func (r RecordsEncoder) Encode() ([]byte, error) {
	res, err := json.Marshal(r)

	return res, err
}

func (r RecordsEncoder) Length() int {
	res, _ := json.Marshal(r)
	return len(res)
}

var records = [][]Record{
	[]Record{Record{"example.internal.", "A", "93.184.216.34", 5570, 0}},
	[]Record{Record{"clever-cloud.internal.", "A", "185.42.117.112", 3600, 0}},
	[]Record{Record{"wiki.test.internal.", "AAAA", "2620:0:862:ed1a::1", 3600, 0}},
}

func main() {
	viper.AutomaticEnv()

	brokerList := viper.GetStringSlice("broker_address")
	topic := viper.GetString("topic")

	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true
	producer, err := sarama.NewSyncProducer(brokerList, config)

	if err != nil {
		panic(err)
	}

	defer func() {
		if err := producer.Close(); err != nil {
			panic(err)
		}
	}()

	for _, record := range records {
		msg := &sarama.ProducerMessage{
			Key:   sarama.StringEncoder(record[0].Name + "|" + record[0].Type),
			Topic: topic,
			Value: RecordsEncoder(record),
		}

		partition, offset, err := producer.SendMessage(msg)

		if err != nil {
			panic(err)
		}

		fmt.Printf("Message %s is stored in topic(%s)/partition(%d)/offset(%d)\n", record[0].Name+"|"+record[0].Type, topic, partition, offset)
	}

}
