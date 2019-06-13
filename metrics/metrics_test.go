package metrics

import (
	"time"

	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoDuplicateWhenAddingTag(t *testing.T) {
	tags := make(map[string]string)
	tags["foo"] = "bar"

	// got
	metric := NewMetric(
		"name",
		tags,
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
		time.Now(),
		Counter,
	)

	// do
	metric.AddTag("foo", "new")

	// want
	assert.Equal(t, "new", metric.Tags()["foo"])
}
