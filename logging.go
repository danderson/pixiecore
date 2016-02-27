package main

import (
	"fmt"
	"log"
)

type logEntry struct {
	Subsystem string
	Debug     bool
	Msg       string
}

var logCh = make(chan logEntry)

func recordLogs(debug bool) {
	for l := range logCh {
		if l.Debug && !debug {
			continue
		}
		log.Printf("[%s] %s", l.Subsystem, l.Msg)
	}
}

// Log records an informational message.
func Log(subsystem string, msg string, args ...interface{}) {
	logCh <- logEntry{subsystem, false, fmt.Sprintf(msg, args...)}
}

// Debug records a message about the internals of Pixiecore.
func Debug(subsystem string, msg string, args ...interface{}) {
	logCh <- logEntry{subsystem, true, fmt.Sprintf(msg, args...)}
}
