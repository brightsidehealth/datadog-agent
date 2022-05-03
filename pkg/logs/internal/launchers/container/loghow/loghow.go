// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

// Package loghow determines how a particular container source should be
// logged, satisfying a plethora of backward-compatibility requirements.
package loghow

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
)

type FromWhere int

const (
	// FromFile means to tail the container from an on-disk file
	FromFile FromWhere = iota

	// FromSocket means to tail the container from a docker-like socket API
	FromSocket
)

// Query defines a request to determine how to log a LogSource.
//
// Some values are available immediately, while others are loaded lazily via
// function callbacks.
type Query struct {
	// ContainersOrPods determines which "flavor" of logging the agent prefers
	ContainersOrPods containersorpods.Chooser

	// DockerContainerUseFile is the `logs_config.docker_container_use_file` config
	DockerContainerUseFile bool

	// DockerContainerForceUseFile is the `logs_config.docker_container_force_use_file` config
	DockerContainerForceUseFile bool

	// SocketInRegistry checks whether the registry contains an entry for tailing
	// this container via API
	SocketInRegistry func() bool
}

// Result defines the result of a logwhere query.
type Result struct {
	// From indicates the sort of tailer the launcher should create.
	From FromWhere

	// Path indicates the path to log from, if From == FromFile
	Path string

	// Source gives the "source" property to include with log messages
	Source string

	// Service gives the "service" property to include with log messages
	Service string
}

// Decide calculates the Result for this Query.
//
// It may return an error if the give context is cancelled, or if no container logging
// facilities are available (containersorpods.LogNothing).
func (q Query) Decide(ctx context.Context) (Result, error) {
	// Wait to determine if we are logging pods or containers.  By the time we
	// have a LogSource, this should be decided with no further waiting -- if
	// AD has discovered a container, then either dockerutil or kubelet has
	// already started.
	switch q.ContainersOrPods.Wait(ctx) {
	case containersorpods.LogPods:
		return Result{
			From: FromFile,
			// TODO...
		}, nil
	default:
		return Result{}, errors.New("NOT IMPLEMENTED")
	}
}
