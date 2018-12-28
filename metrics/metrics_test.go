package metrics

import (
	"time"

	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNoDuplicateWhenAddingTag(t *testing.T) {
	tags := make(map[string]string)
	tags["foo"] = "bar"

	// got
	metric := NewMetric(
		"name",
		tags,
		make(map[string]interface{}),
		time.Now(),
		Counter,
	)

	// do
	metric.AddTag("foo", "bar")

	// want
	assert.Equal(t, 1, len(metric.Tags()))
}

func TestFindTagWhenExist(t *testing.T) {
	tags := make(map[string]string, 1)
	tags["foo"] = "bar"

	// got
	metric := NewMetric(
		"name",
		tags,
		make(map[string]interface{}, 1),
		time.Now(),
		Counter,
	)

	// do
	value, _ := metric.GetTag("foo")

	// want
	assert.True(t, metric.HasTag("foo"))
	assert.Equal(t, "bar", value)
}

func TestUpdateTheValueWhenTagAlreadyExist(t *testing.T) {
	tags := make(map[string]string)
	tags["foo"] = "bar"

	// got
	metric := NewMetric(
		"name",
		tags,
		make(map[string]interface{}),
		time.Now(),
		Counter,
	)

	// do
	metric.AddTag("foo", "new")

	// want
	assert.Equal(t, "new", metric.Tags()["foo"])
}

func TestNoDuplicateWhenAddingField(t *testing.T) {
	fields := make(map[string]interface{}, 1)
	fields["foo"] = float64(1)

	// got
	metric := NewMetric(
		"name",
		make(map[string]string),
		fields,
		time.Now(),
		Counter,
	)

	// do
	metric.AddField("foo", float64(2))

	// want
	assert.Equal(t, 1, len(metric.Fields()))
}

func TestFindFieldWhenExist(t *testing.T) {
	fields := make(map[string]interface{}, 1)
	fields["foo"] = float64(1)

	// got
	metric := NewMetric(
		"name",
		make(map[string]string),
		fields,
		time.Now(),
		Counter,
	)

	// do
	value, _ := metric.GetField("foo")

	// want
	assert.True(t, metric.HasField("foo"))
	assert.Equal(t, float64(1), value)
}

func TestUpdateTheValueWhenFieldAlreadyExist(t *testing.T) {
	fields := make(map[string]interface{}, 1)
	fields["foo"] = float64(1)

	// got
	metric := NewMetric(
		"name",
		make(map[string]string),
		fields,
		time.Now(),
		Counter,
	)

	// do
	metric.AddField("foo", float64(2))

	// want
	assert.Equal(t, float64(2), metric.Fields()["foo"])
}
