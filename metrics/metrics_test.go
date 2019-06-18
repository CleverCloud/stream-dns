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
		1,
	)

	// do
	metric.AddTag("foo", "bar")

	// want
	assert.Equal(t, 1, len(metric.Tags()))
	assert.Equal(t, 1, metric.Value().(int))
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
		1,
	)

	// do
	value, _ := metric.GetTag("foo")

	// want
	assert.True(t, metric.HasTag("foo"))
	assert.Equal(t, "bar", value)
	assert.Equal(t, 1, metric.Value().(int))
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
		1,
	)

	// do
	metric.AddTag("foo", "new")

	// want
	assert.Equal(t, "new", metric.Tags()["foo"])
	assert.Equal(t, 1, metric.Value().(int))
}

func TestTagsToString(t *testing.T) {
	tags := make(map[string]string)
	tags["foo"] = "bar"
	tags["bar"] = "foo"

	// got
	metric := NewMetric(
		"name",
		tags,
		time.Now(),
		Counter,
		1,
	)

	// do
	res := metric.TagsToString()

	// want
	assert.Equal(t, "foo=bar bar=foo", res)
}
