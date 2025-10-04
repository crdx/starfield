package main

type Config struct {
	SQL []Entry `yaml:"sql"`
}

type Entry struct {
	Schema string `yaml:"schema"`
}
