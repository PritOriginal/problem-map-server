package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PritOriginal/problem-map-server/configs"
	"github.com/PritOriginal/problem-map-server/internal/handler"
	"github.com/PritOriginal/problem-map-server/internal/storage/db"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

func main() {
	cfg, err := getConfig()
	if err != nil {
		log.Fatalf("error get config: %v", err)
	}

	var logFIle *os.File
	if cfg.Env == slogger.Prod {
		logFIle, err = os.OpenFile("log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("error opening file: %v", err)
		}
		defer logFIle.Close()
	}

	logger, err := slogger.SetupLogger(cfg.Env, logFIle)
	if err != nil {
		log.Fatalf("error init logger: %v", err)
	}

	// Init DB
	dbConn, err := db.Initialize(cfg.DB)
	if err != nil {
		logger.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	logger.Info("PostgreSQL connected!")
	defer dbConn.Close()

	r := handler.GetRoute(logger, dbConn)

	server, _ := New(cfg, logger, r)
	server.Start()
}

func getConfig() (configs.Config, error) {
	var cfg configs.Config

	viper.AddConfigPath("./configs")
	if err := viper.ReadInConfig(); err != nil {
		return cfg, err
	}

	err := viper.Unmarshal(&cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

type Server struct {
	cfg    configs.Config
	log    *slog.Logger
	router *chi.Mux
}

func New(cfg configs.Config, log *slog.Logger, router *chi.Mux) (*Server, error) {
	s := &Server{cfg: cfg, log: log, router: router}
	return s, nil
}

func (s *Server) Start() {
	server := &http.Server{
		Addr:         s.cfg.Server.Host + ":" + s.cfg.Server.Port,
		Handler:      s.router,
		ReadTimeout:  s.cfg.Server.Timeout.Read * time.Second,
		WriteTimeout: s.cfg.Server.Timeout.Write * time.Second,
		IdleTimeout:  s.cfg.Server.Timeout.Idle * time.Second,
	}

	s.log.Info("starting server", slog.String("address", ":"+s.cfg.Server.Port))
	go func() {
		if err := server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {

			} else {
				s.log.Error("failed to start server")
			}
		}
	}()
	s.log.Info("server started")

	// listen for ctrl+c signal from terminal
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	s.log.Info("server stopped")
}
