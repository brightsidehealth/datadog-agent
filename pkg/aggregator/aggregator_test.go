// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build test

package aggregator

import (
	// stdlib
	"errors"
	"expvar"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var checkID1 check.ID = "1"
var checkID2 check.ID = "2"

func TestRegisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := InitAggregator(nil, nil, "")
	err := agg.registerSender(checkID1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	err = agg.registerSender(checkID2)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	// Already registered sender => error
	err = agg.registerSender(checkID2)
	assert.NotNil(t, err)
}

func TestDeregisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := InitAggregator(nil, nil, "")
	agg.registerSender(checkID1)
	agg.registerSender(checkID2)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	agg.deregisterSender(checkID1)
	require.Len(t, aggregatorInstance.checkSamplers, 1)
	_, ok := agg.checkSamplers[checkID1]
	assert.False(t, ok)
	_, ok = agg.checkSamplers[checkID2]
	assert.True(t, ok)
}

func TestAddServiceCheckDefaultValues(t *testing.T) {
	resetAggregator()
	agg := InitAggregator(nil, nil, "resolved-hostname")

	agg.addServiceCheck(metrics.ServiceCheck{
		// leave Host and Ts fields blank
		CheckName: "my_service.can_connect",
		Status:    metrics.ServiceCheckOK,
		Tags:      []string{"bar", "foo", "bar"},
		Message:   "message",
	})
	agg.addServiceCheck(metrics.ServiceCheck{
		CheckName: "my_service.can_connect",
		Status:    metrics.ServiceCheckOK,
		Host:      "my-hostname",
		Tags:      []string{"foo", "foo", "bar"},
		Ts:        12345,
		Message:   "message",
	})

	require.Len(t, agg.serviceChecks, 2)
	assert.Equal(t, "", agg.serviceChecks[0].Host)
	assert.ElementsMatch(t, []string{"bar", "foo"}, agg.serviceChecks[0].Tags)
	assert.NotZero(t, agg.serviceChecks[0].Ts) // should be set to the current time, let's just check that it's not 0
	assert.Equal(t, "my-hostname", agg.serviceChecks[1].Host)
	assert.ElementsMatch(t, []string{"foo", "bar"}, agg.serviceChecks[1].Tags)
	assert.Equal(t, int64(12345), agg.serviceChecks[1].Ts)
}

func TestAddEventDefaultValues(t *testing.T) {
	resetAggregator()
	agg := InitAggregator(nil, nil, "resolved-hostname")

	agg.addEvent(metrics.Event{
		// only populate required fields
		Title: "An event occurred",
		Text:  "Event description",
	})
	agg.addEvent(metrics.Event{
		// populate all fields
		Title:          "Another event occurred",
		Text:           "Other event description",
		Ts:             12345,
		Priority:       metrics.EventPriorityNormal,
		Host:           "my-hostname",
		Tags:           []string{"foo", "bar", "foo"},
		AlertType:      metrics.EventAlertTypeError,
		AggregationKey: "my_agg_key",
		SourceTypeName: "custom_source_type",
	})

	require.Len(t, agg.events, 2)
	// Default values are set on Ts
	event1 := agg.events[0]
	assert.Equal(t, "An event occurred", event1.Title)
	assert.Equal(t, "", event1.Host)
	assert.NotZero(t, event1.Ts) // should be set to the current time, let's just check that it's not 0
	assert.Zero(t, event1.Priority)
	assert.Zero(t, event1.Tags)
	assert.Zero(t, event1.AlertType)
	assert.Zero(t, event1.AggregationKey)
	assert.Zero(t, event1.SourceTypeName)

	event2 := agg.events[1]
	// No change is made
	assert.Equal(t, "Another event occurred", event2.Title)
	assert.Equal(t, "my-hostname", event2.Host)
	assert.Equal(t, int64(12345), event2.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, event2.Priority)
	assert.ElementsMatch(t, []string{"foo", "bar"}, event2.Tags)
	assert.Equal(t, metrics.EventAlertTypeError, event2.AlertType)
	assert.Equal(t, "my_agg_key", event2.AggregationKey)
	assert.Equal(t, "custom_source_type", event2.SourceTypeName)
}

func TestSetHostname(t *testing.T) {
	resetAggregator()
	agg := InitAggregator(nil, nil, "hostname")
	assert.Equal(t, "hostname", agg.hostname)
	sender, err := GetSender(checkID1)
	require.NoError(t, err)
	checkSender, ok := sender.(*checkSender)
	require.True(t, ok)
	assert.Equal(t, "hostname", checkSender.defaultHostname)

	agg.SetHostname("different-hostname")
	assert.Equal(t, "different-hostname", agg.hostname)
	assert.Equal(t, "different-hostname", checkSender.defaultHostname)
}

func TestDefaultData(t *testing.T) {
	resetAggregator()
	s := &serializer.MockSerializer{}
	agg := InitAggregator(s, nil, "hostname")
	start := time.Now()

	s.On("SendServiceChecks", metrics.ServiceChecks{{
		CheckName: "datadog.agent.up",
		Status:    metrics.ServiceCheckOK,
		Tags:      []string{},
		Ts:        start.Unix(),
		Host:      agg.hostname,
	}}).Return(nil).Times(1)

	series := metrics.Series{&metrics.Serie{
		Name:           fmt.Sprintf("datadog.%s.running", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
		Tags:           metrics.NewCompositeTags([]string{fmt.Sprintf("version:%s", version.AgentVersion)}, nil),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}, &metrics.Serie{
		Name:           fmt.Sprintf("n_o_i_n_d_e_x.datadog.%s.payload.dropped", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 0, Ts: float64(start.Unix())}},
		Host:           agg.hostname,
		Tags:           metrics.NewCompositeTags([]string{}, nil),
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}}

	s.On("SendSeries", series).Return(nil).Times(1)

	agg.Flush(start, false)
	s.AssertNotCalled(t, "SendEvents")
	s.AssertNotCalled(t, "SendSketch")

	// not counted as huge for (just checking the first threshold..)
	assert.Equal(t, uint64(0), atomic.LoadUint64(&tagsetTlm.hugeSeriesCount[0]))
}

func TestSeriesTooManyTags(t *testing.T) {
	test := func(tagCount int) func(t *testing.T) {
		expHugeCounts := make([]uint64, tagsetTlm.size)

		for i, thresh := range tagsetTlm.sizeThresholds {
			if uint64(tagCount) > thresh {
				expHugeCounts[i]++
			}
		}

		return func(t *testing.T) {
			resetAggregator()
			s := &serializer.MockSerializer{}
			agg := InitAggregator(s, nil, "hostname")
			start := time.Now()

			var tags []string
			for i := 0; i < tagCount; i++ {
				tags = append(tags, fmt.Sprintf("tag%d", i))
			}

			ser := &metrics.Serie{
				Name:           "test.series",
				Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
				Tags:           metrics.NewCompositeTags(tags, nil),
				Host:           agg.hostname,
				MType:          metrics.APIGaugeType,
				SourceTypeName: "System",
			}
			AddRecurrentSeries(ser)

			s.On("SendServiceChecks", mock.Anything).Return(nil).Times(1)
			s.On("SendSeries", mock.Anything).Return(nil).Times(1)

			agg.Flush(start, true)
			s.AssertNotCalled(t, "SendEvents")
			s.AssertNotCalled(t, "SendSketch")

			expMap := map[string]uint64{}
			for i, thresh := range tagsetTlm.sizeThresholds {
				assert.Equal(t, expHugeCounts[i], atomic.LoadUint64(&tagsetTlm.hugeSeriesCount[i]))
				expMap[fmt.Sprintf("Above%d", thresh)] = expHugeCounts[i]
			}
			gotMap := aggregatorExpvars.Get("MetricTags").(expvar.Func).Value().(map[string]map[string]uint64)["Series"]
			assert.Equal(t, expMap, gotMap)
		}
	}
	t.Run("not-huge", test(10))
	t.Run("almost-huge", test(95))
	t.Run("huge", test(110))
}

func TestDistributionsTooManyTags(t *testing.T) {
	test := func(tagCount int) func(t *testing.T) {
		expHugeCounts := make([]uint64, tagsetTlm.size)

		for i, thresh := range tagsetTlm.sizeThresholds {
			if uint64(tagCount) > thresh {
				expHugeCounts[i]++
			}
		}

		return func(t *testing.T) {
			resetAggregator()
			s := &serializer.MockSerializer{}
			agg := InitAggregator(s, nil, "hostname")
			start := time.Now()

			var tags []string
			for i := 0; i < tagCount; i++ {
				tags = append(tags, fmt.Sprintf("tag%d", i))
			}

			samp := &metrics.MetricSample{
				Name:  "test.sample",
				Value: 13.0,
				Mtype: metrics.DistributionType,
				Tags:  tags,
				Host:  agg.hostname,
			}
			agg.addSample(samp, timeNowNano()-10000000)

			s.On("SendServiceChecks", mock.Anything).Return(nil).Times(1)
			s.On("SendSeries", mock.Anything).Return(nil).Times(1)
			s.On("SendSketch", mock.Anything).Return(nil).Times(1)

			agg.Flush(start, true)
			s.AssertNotCalled(t, "SendEvents")

			expMap := map[string]uint64{}
			for i, thresh := range tagsetTlm.sizeThresholds {
				assert.Equal(t, expHugeCounts[i], atomic.LoadUint64(&tagsetTlm.hugeSketchesCount[i]))
				expMap[fmt.Sprintf("Above%d", thresh)] = expHugeCounts[i]
			}
			gotMap := aggregatorExpvars.Get("MetricTags").(expvar.Func).Value().(map[string]map[string]uint64)["Sketches"]
			assert.Equal(t, expMap, gotMap)
		}
	}
	t.Run("not-huge", test(10))
	t.Run("almost-huge", test(95))
	t.Run("huge", test(110))
}

func TestRecurentSeries(t *testing.T) {
	resetAggregator()
	s := &serializer.MockSerializer{}
	agg := NewBufferedAggregator(s, nil, "hostname", DefaultFlushInterval)

	// Add two recurrentSeries
	AddRecurrentSeries(&metrics.Serie{
		Name:   "some.metric.1",
		Points: []metrics.Point{{Value: 21}},
		Tags:   metrics.NewCompositeTags([]string{"tag:1", "tag:2"}, nil),
		MType:  metrics.APIGaugeType,
	})
	AddRecurrentSeries(&metrics.Serie{
		Name:           "some.metric.2",
		Points:         []metrics.Point{{Value: 22}},
		Tags:           metrics.NewCompositeTags([]string{}, nil),
		Host:           "non default host",
		MType:          metrics.APIGaugeType,
		SourceTypeName: "non default SourceTypeName",
	})

	start := time.Now()

	series := metrics.Series{&metrics.Serie{
		Name:           "some.metric.1",
		Points:         []metrics.Point{{Value: 21, Ts: float64(start.Unix())}},
		Tags:           metrics.NewCompositeTags([]string{"tag:1", "tag:2"}, nil),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}, &metrics.Serie{
		Name:           "some.metric.2",
		Points:         []metrics.Point{{Value: 22, Ts: float64(start.Unix())}},
		Tags:           metrics.NewCompositeTags([]string{}, nil),
		Host:           "non default host",
		MType:          metrics.APIGaugeType,
		SourceTypeName: "non default SourceTypeName",
	}, &metrics.Serie{
		Name:           fmt.Sprintf("datadog.%s.running", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
		Tags:           metrics.NewCompositeTags([]string{fmt.Sprintf("version:%s", version.AgentVersion)}, nil),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}, &metrics.Serie{
		Name:           fmt.Sprintf("n_o_i_n_d_e_x.datadog.%s.payload.dropped", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 0, Ts: float64(start.Unix())}},
		Host:           agg.hostname,
		Tags:           metrics.NewCompositeTags([]string{}, nil),
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}}

	// Check only the name for `datadog.agent.up` as the timestamp may not be the same.
	agentUpMatcher := mock.MatchedBy(func(m metrics.ServiceChecks) bool {
		require.Equal(t, 1, len(m))
		require.Equal(t, "datadog.agent.up", m[0].CheckName)
		require.Equal(t, metrics.ServiceCheckOK, m[0].Status)
		require.Equal(t, []string{}, m[0].Tags)
		require.Equal(t, agg.hostname, m[0].Host)

		return true
	})
	s.On("SendServiceChecks", agentUpMatcher).Return(nil).Times(1)
	s.On("SendSeries", series).Return(nil).Times(1)

	agg.Flush(start, true)
	s.AssertNotCalled(t, "SendEvents")
	s.AssertNotCalled(t, "SendSketch")

	// Assert that recurrentSeries are sent on each flushed
	s.On("SendServiceChecks", agentUpMatcher).Return(nil).Times(1)
	s.On("SendSeries", series).Return(nil).Times(1)
	agg.Flush(start, true)
	s.AssertNotCalled(t, "SendEvents")
	s.AssertNotCalled(t, "SendSketch")
	s.AssertExpectations(t)
}

func TestTags(t *testing.T) {
	tests := []struct {
		name                    string
		tlmContainerTagsEnabled bool
		agentTags               func(collectors.TagCardinality) ([]string, error)
		withVersion             bool
		want                    []string
	}{
		{
			name:                    "tags disabled, with version",
			tlmContainerTagsEnabled: false,
			agentTags:               func(collectors.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			withVersion:             true,
			want:                    []string{"version:" + version.AgentVersion},
		},
		{
			name:                    "tags disabled, without version",
			tlmContainerTagsEnabled: false,
			agentTags:               func(collectors.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			withVersion:             false,
			want:                    []string{},
		},
		{
			name:                    "tags enabled, with version",
			tlmContainerTagsEnabled: true,
			agentTags:               func(collectors.TagCardinality) ([]string, error) { return []string{"container_name:agent"}, nil },
			withVersion:             true,
			want:                    []string{"container_name:agent", "version:" + version.AgentVersion},
		},
		{
			name:                    "tags enabled, without version",
			tlmContainerTagsEnabled: true,
			agentTags:               func(collectors.TagCardinality) ([]string, error) { return []string{"container_name:agent"}, nil },
			withVersion:             false,
			want:                    []string{"container_name:agent"},
		},
		{
			name:                    "tags enabled, with version, tagger error",
			tlmContainerTagsEnabled: true,
			agentTags:               func(collectors.TagCardinality) ([]string, error) { return nil, errors.New("no tags") },
			withVersion:             true,
			want:                    []string{"version:" + version.AgentVersion},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Datadog.Set("basic_telemetry_add_container_tags", tt.tlmContainerTagsEnabled)
			agg := NewBufferedAggregator(nil, nil, "hostname", time.Second)
			agg.agentTags = tt.agentTags
			assert.ElementsMatch(t, tt.want, agg.tags(tt.withVersion))
		})
	}
}
