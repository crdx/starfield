package main

type Config struct {
	SQL []Entry `yaml:"sql"`
}

type Entry struct {
	Name   string `yaml:"name"`
	Schema string `yaml:"schema"`
}
