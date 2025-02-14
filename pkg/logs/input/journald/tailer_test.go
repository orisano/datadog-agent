// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build systemd

package journald

import (
	"testing"
	"time"

	"github.com/coreos/go-systemd/sdjournal"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestIdentifier(t *testing.T) {
	var tailer *Tailer
	var source *config.LogSource

	// expect default identifier
	source = config.NewLogSource("", &config.LogsConfig{})
	tailer = NewTailer(source, nil)
	assert.Equal(t, "journald:default", tailer.Identifier())

	// expect identifier to be overidden
	source = config.NewLogSource("", &config.LogsConfig{Path: "any_path"})
	tailer = NewTailer(source, nil)
	assert.Equal(t, "journald:any_path", tailer.Identifier())
}

func TestShouldDropEntry(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{ExcludeUnits: []string{"foo", "bar"}})
	tailer := NewTailer(source, nil)
	err := tailer.setup()
	assert.Nil(t, err)

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "bar",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "boo",
			},
		}))
}

func TestApplicationName(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	assert.Equal(t, "foo", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
			},
		}, []string{}))

	assert.Equal(t, "foo.service", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:         "foo.sh",
			},
		}, []string{}))

	assert.Equal(t, "foo.sh", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_COMM: "foo.sh",
			},
		}, []string{}))

	assert.Equal(t, "", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		}, []string{}))
}

func TestContent(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	assert.Equal(t, []byte(`{"journald":{"_A":"foo.service"},"message":"bar"}`), tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_MESSAGE: "bar",
				"_A":                               "foo.service",
			},
		}))

	assert.Equal(t, []byte(`{"journald":{"_A":"foo.service"}}`), tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"_A": "foo.service",
			},
		}))

	assert.Equal(t, []byte(`{"journald":{}}`), tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		}))
}

func TestSeverity(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	priorityValues := []string{"0", "1", "2", "3", "4", "5", "6", "7", "foo"}
	statuses := []string{message.StatusEmergency, message.StatusAlert, message.StatusCritical, message.StatusError, message.StatusWarning, message.StatusNotice, message.StatusInfo, message.StatusDebug, message.StatusInfo}

	for i, priority := range priorityValues {
		assert.Equal(t, statuses[i], tailer.getStatus(
			&sdjournal.JournalEntry{
				Fields: map[string]string{
					sdjournal.SD_JOURNAL_FIELD_PRIORITY: priority,
				},
			}))
	}
}

func TestApplicationNameShouldBeDockerForContainerEntries(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	assert.Equal(t, "docker", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               "bar",
			},
		}, []string{}))
}

func TestApplicationNameShouldBeShortImageForContainerEntries(t *testing.T) {
	containerID := "bar"

	source := config.NewLogSource("", &config.LogsConfig{ContainerMode: true})
	tailer := NewTailer(source, nil)

	assert.Equal(t, "testImage", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               containerID,
			},
		}, []string{"short_image:testImage"}))

	// Verify we have the value in our cache
	_, hit := cache.Cache.Get(getImageCacheKey(containerID))
	assert.True(t, hit)
}

func TestApplicationNameShouldBeDockerWhenTagNotFound(t *testing.T) {
	containerID := "bar2"

	source := config.NewLogSource("", &config.LogsConfig{ContainerMode: true})
	tailer := NewTailer(source, nil)

	assert.Equal(t, "docker", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               containerID,
			},
		}, []string{"not_short_image:testImage"}))

	// Verify we don't have value in our cache
	_, hit := cache.Cache.Get(getImageCacheKey(containerID))
	assert.False(t, hit)
}

func TestWrongTypeFromCache(t *testing.T) {
	containerID := "bar3"

	// Store wrong type in cache, verify we ignore the value
	cache.Cache.Set(getImageCacheKey(containerID), 10, 30*time.Second)

	source := config.NewLogSource("", &config.LogsConfig{ContainerMode: true})
	tailer := NewTailer(source, nil)

	assert.Equal(t, "testImage", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               containerID,
			},
		}, []string{"short_image:testImage"}))

	// Verify we have the value in our cache
	_, hit := cache.Cache.Get(getImageCacheKey(containerID))
	assert.True(t, hit)
}
