package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/PritOriginal/problem-map-server/internal/handler"
	"github.com/PritOriginal/problem-map-server/internal/storage/db"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/paulmach/orb/geojson"
	"github.com/spf13/viper"
)

func main() {
	rawJSON := []byte(`{"type":"Point","coordinates":[41.402893,52.700111]}`)

	fc, _ := geojson.UnmarshalFeatureCollection(rawJSON)
	fmt.Print(fc)

	viper.SetConfigFile(".env")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}

	f, err := os.OpenFile("log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	logger := slogger.SetupLogger(viper.GetString("ENV"), f)

	// Init DB
	dbConn, err := db.Initialize()
	if err != nil {
		logger.Error("Failed Connection to database", slog.Attr{Key: "error", Value: slog.StringValue(err.Error())})
		panic(err)
	}
	logger.Info("PostgreSQL Connected!")
	defer dbConn.Close()

	r := handler.GetRoute(logger, dbConn)

	// Start Server
	logger.Info("starting server", slog.String("address", ":3333"))
	go func() {
		if err := http.ListenAndServe(":3333", r); err != nil {
			logger.Error("failed to start server")
		}
	}()
	logger.Info("server started")

	// listen for ctrl+c signal from terminal
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	logger.Info("server stopped")
}
