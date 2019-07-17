package metrics

import (
	"fmt"
	"strings"
	"time"
)

type ValueType int

const (
	_ ValueType = iota
	Counter
	Gauge
)

var TypeToString = map[ValueType]string{
	Counter: "counter",
	Gauge:   "gauge",
}

type Tag struct {
	Key   string
	Value string
}

type Metric interface {
	Name() string
	Tags() map[string]string
	Time() time.Time
	Type() ValueType

	SetName(name string)

	ToString() string

	GetTag(key string) (string, bool)
	HasTag(key string) bool
	AddTag(key, value string)
	TagsToString() string

	SetTime(t time.Time)

	SetAggregate(bool)
	IsAggregate() bool

	Value() interface{}
}

type metric struct {
	name      string
	tags      []Tag
	tm        time.Time
	tp        ValueType
	value     interface{}
	aggregate bool
}

func NewMetric(
	name string,
	tags map[string]string,
	tm time.Time,
	tp ValueType,
	value interface{},
) Metric {
	m := &metric{
		name:  name,
		tags:  nil,
		tm:    tm,
		tp:    tp,
		value: value,
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
	var val string

	switch m.tp {
	case Counter, Gauge:
		val = fmt.Sprintf("%d", m.value)
	default:
		val = "[undefined type]"
	}

	tags := m.TagsToString()

	return fmt.Sprintf("metric name: %s tags: %s at %s value = %s", m.name, tags, m.tm.UTC().String(), val)
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

func (m *metric) TagsToString() string {
	var buf string

	for _, tag := range m.tags {
		buf = buf + fmt.Sprintf("%s=%s ", tag.Key, tag.Value)
	}

	return strings.TrimRight(buf, " ")
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

// You must in the caller, cast the value into the correct type bu using the typevalue field
func (m *metric) Value() interface{} {
	return m.value
}
