package model

import "time"

type Backend struct {
	ID       string
	Name     string
	Address  string
	Priority int
	Latency  time.Duration
}
