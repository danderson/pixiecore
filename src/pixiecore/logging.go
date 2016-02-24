package main

import (
	"fmt"
	"log"
)

type LogEntry struct {
	Subsystem string
	Debug     bool
	Msg       string
}

var logCh = make(chan LogEntry)

func RecordLogs(debug bool) {
	for l := range logCh {
		if l.Debug && !debug {
			continue
		}
		log.Printf("[%s] %s", l.Subsystem, l.Msg)
	}
}

func Log(subsystem string, msg string, args ...interface{}) {
	logCh <- LogEntry{subsystem, false, fmt.Sprintf(msg, args...)}
}

func Debug(subsystem string, msg string, args ...interface{}) {
	logCh <- LogEntry{subsystem, true, fmt.Sprintf(msg, args...)}
}
