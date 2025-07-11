package db

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type Db struct {
	cl     *mongo.Client
	dbName string
}

func NewDb(uri string, dbName string) (*Db, error) {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	// Ping the primary to verify connection
	if err := client.Ping(context.Background(), readpref.Primary()); err != nil {
		return nil, err
	}

	return &Db{cl: client, dbName: dbName}, nil
}

func (db *Db) Close() error {
	if db.cl != nil {
		return db.cl.Disconnect(context.Background())
	}
	return nil
}

type Server struct {
	Name       string `bson:"name"`
	Subdomain  string `bson:"subdomain"`
	Endpoint   string `bson:"endpoint"`
	Status     string `bson:"status"`
	GameConfig struct {
		MessageofTheDay string
		Version         string
	} `bson:"game_config"`
	EcoConfig EcoConfig `bson:"eco_config"`
}

type EcoConfig struct {
	Enabled                bool     `json:"enabled" bson:"enabled"`                                                         // Whether Eco mode is enabled for the server. Stops the server when no players are online.
	Timeout                int      `json:"timeout" bson:"timeout"`                                                         // Timeout in minutes before stopping the server when no players are online.
	StartWhenJoined        bool     `json:"start_when_joined" bson:"start_when_joined"`                                     // Whether to start the server when a player joins.
	StartWhenJoinWhitelist []string `json:"start_when_join_whitelist,omitempty" bson:"start_when_join_whitelist,omitempty"` // List of players who can start the server when they join. This is used to combat abusing autostart.
}

func (d *Db) FindServerAddrBySubdomain(host string) *Server {
	var srvr Server

	db := d.cl.Database(d.dbName)

	err := db.Collection("servers").FindOne(context.Background(), map[string]string{"subdomain": host}).Decode(&srvr)
	if err != nil {
		fmt.Println("Error finding server by host:", err)
		return nil
	}
	return &srvr
}
