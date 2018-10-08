package config

import logging "github.com/op/go-logging"

type ValetConfig struct {
	ParController  ControllerConfig
	NagController  ControllerConfig
	PLController   ControllerConfig
	LoggingBackend logging.LeveledBackend
}

type ControllerConfig struct {
	Threads   int
	ShouldRun bool
}
