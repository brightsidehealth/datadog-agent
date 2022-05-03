// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package loghow

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestLogPods(t *testing.T) {
	res, err := Query{
		ContainersOrPods:            containersorpods.NewDecidedChooser(containersorpods.LogPods),
		DockerContainerUseFile:      false,
		DockerContainerForceUseFile: false,
		SocketInRegistry:            func() bool { return false },
	}.Decide(context.TODO())
	require.NoError(t, err)

	assert.Equal(t, FromFile, res.From)
	assert.Equal(t, "", res.Path)       // TODO
	assert.Equal(t, "foo", res.Source)  // TODO
	assert.Equal(t, "boo", res.Service) // TODO
}
