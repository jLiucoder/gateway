package main

import (
	"log"
)

func main() {
	log.Println("----------Start of the main program----------")

	config, error := loadConfig("config.yaml")

	if error != nil {
		log.Panic("Can't load config file, make sure the format is correct: ", error)
	}

	startServer(config)
}
