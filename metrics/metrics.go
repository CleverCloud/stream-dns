package metrics

import (
	"fmt"
	"time"
)

type ValueType int

const (
	_ ValueType = iota
	Counter
	Gauge
	Message
)

var TypeToString = map[ValueType]string{
	Counter: "counter",
	Gauge:   "gauge",
	Message: "message",
}

type Tag struct {
	Key   string
	Value string
}

type Metric interface {
	// data structure functions
	Name() string
	Tags() map[string]string
	Time() time.Time
	Type() ValueType

	SetName(name string)

	// Tag functions
	GetTag(key string) (string, bool)
	HasTag(key string) bool
	AddTag(key, value string)

	SetTime(t time.Time)

	SetAggregate(bool)
	IsAggregate() bool
}

type metric struct {
	name      string
	tags      []Tag
	tm        time.Time
	tp        ValueType
	aggregate bool
}

func NewMetric(
	name string,
	tags map[string]string,
	tm time.Time,
	tp ...ValueType,
) Metric {
	m := &metric{
		name: name,
		tags: nil,
		tm:   tm,
		tp:   tp[0],
	}

	if len(tags) > 0 {
		m.tags = make([]Tag, 0, len(tags))
		for k, v := range tags {
			m.tags = append(m.tags,
				Tag{Key: k, Value: v})
		}
	}

	return m
}

func (m *metric) ToString() string {
	return fmt.Sprintf("%s %v %v %d", m.name, m.Tags(), m.tm.UnixNano())
}

func (m *metric) Name() string {
	return m.name
}

func (m *metric) Tags() map[string]string {
	tags := make(map[string]string, len(m.tags))
	for _, tag := range m.tags {
		tags[tag.Key] = tag.Value
	}
	return tags
}

func (m *metric) Time() time.Time {
	return m.tm
}

func (m *metric) Type() ValueType {
	return m.tp
}

func (m *metric) SetName(name string) {
	m.name = name
}

func (m *metric) AddTag(key, value string) {
	for i, tag := range m.tags {
		if tag.Key == key {
			m.tags[i] = Tag{key, value}
			return
		}
	}

	m.tags = append(m.tags, Tag{Key: key, Value: value})
}

func (m *metric) HasTag(key string) bool {
	for _, tag := range m.tags {
		if tag.Key == key {
			return true
		}
	}
	return false
}

func (m *metric) GetTag(key string) (string, bool) {
	for _, tag := range m.tags {
		if tag.Key == key {
			return tag.Value, true
		}
	}
	return "", false
}

func (m *metric) SetTime(t time.Time) {
	m.tm = t
}

func (m *metric) SetAggregate(b bool) {
	m.aggregate = true
}

func (m *metric) IsAggregate() bool {
	return m.aggregate
}
