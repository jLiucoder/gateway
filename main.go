package main

import (
	"log"
	"os"
	"strings"
	"github.com/joho/godotenv"
)

func main() {

	godotenv.Load()
	keys := strings.Split(os.Getenv("API_KEYS"), ",")
	log.Println("----------Start of the main program----------")

	config, err := loadConfig("config.yaml", keys)

	if err != nil {
		log.Panic("Can't load config file, make sure the format is correct: ", err)
	}

	startServer(config)
}
