package main

import "time"

// Event is the shared in-memory representation used by both the ICS and
// CSV readers/writers, so converting between formats is just parse+write.
type Event struct {
	UID         string    `json:"uid"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	Location    string    `json:"location,omitempty"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"all_day"`
}
