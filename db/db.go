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
}

func (d *Db) FindServerAddrByHost(host string) *Server {
	var srvr Server

	db := d.cl.Database(d.dbName)

	err := db.Collection("servers").FindOne(context.Background(), map[string]string{"subdomain": host}).Decode(&srvr)
	if err != nil {
		fmt.Println("Error finding server by host:", err)
		return nil
	}
	return &srvr
}
