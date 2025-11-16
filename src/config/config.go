package config

import (
	"log"
	"net"
	"os"

	"github.com/joho/godotenv"
)

type Conf struct {
	Server ConfServer
	DB     ConfDB
}

type ConfServer struct {
	Host string
	Port string
}

func (server ConfServer) GetURL() string {
	return net.JoinHostPort(server.Host, server.Port)
}

type ConfDB struct {
		URL string
	// Host     string
	// Port     int
	// Username string
	// Password string
	// DBName   string
}

func New() *Conf {
	var c Conf

	err := godotenv.Load()	
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	c.DB.URL = os.Getenv("DATABASE_URL")

	c.Server.Host = os.Getenv("SERVER_HOST")
	c.Server.Port = os.Getenv("SERVER_PORT")

	return &c
}

func NewDB() *ConfDB {

	var c ConfDB

	// parse from env

	return &c
}
