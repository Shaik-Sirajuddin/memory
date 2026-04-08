package main

import (
	"log"

	"github.com/Shaik-Sirajuddin/memory/cli"
	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/operator"
)

func main() {
	op, err := operator.New()
	if err != nil {
		log.Fatal(err)
	}

	c := cli.Entrypoint(op, &config.DefaultOmniConfigResolver{})
	if err := c.Install(); err != nil {
		log.Fatal(err)
	}
}
