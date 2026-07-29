package constants

import "time"

const (
	ProxyDialTimeout = 5 * time.Second
	Timeout          = 30 * time.Second
	KeepAlive        = 30 * time.Second
)

const MaxBodyReadBytes = 5 * 1024 * 1024
