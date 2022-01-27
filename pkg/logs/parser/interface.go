// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser supports transforming raw log "lines" into messages with some
// associated metadata (timestamp, severity, etc.).
//
// This parsing comes after "line parsing" (breaking input into multiple lines) and
// before further processing and aggregation of log messages.
package parser

// Message represents a message parsed from a single line of log data
type Message struct {
	// Content is the message content.  If this is nil then the message
	// should be considered empty and ignored.
	Content []byte

	// Status is the status parsed from the message, if any.
	Status string

	// Timestamp is the message timestamp, if any.  It is an ISO-8601-formatted
	// string (YYYY-MM-DDThh:mm:ss.sZ)
	Timestamp string

	// IsPartial indicates that this is a partial message.  If the parser
	// supports partial lines, then this is true only for the message returned
	// from the last parsed line in a multi-line message.
	IsPartial bool
}

// Parser parses messages, given as a raw byte sequence, into content and metadata.
type Parser interface {
	// Parse parses a line of log input.
	Parse([]byte) (Message, error)

	// SupportsPartialLine returns true for sources that can have partial
	// lines. If SupportsPartialLine is true, Parse can return messages with
	// IsPartial: true
	SupportsPartialLine() bool
}
