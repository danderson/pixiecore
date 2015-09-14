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

func Log(subsystem string, debug bool, msg string, args ...interface{}) {
	logCh <- LogEntry{subsystem, debug, fmt.Sprintf(msg, args...)}
}
