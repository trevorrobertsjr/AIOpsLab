package main

import (
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/harlow/go-micro-services/registry"
	"github.com/harlow/go-micro-services/services/frontend"
	"github.com/harlow/go-micro-services/tune"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/trevorrobertsjr/datadogwriter"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func main() {
	tune.Init()
	// Configure tracing
	tracer.Start(
		tracer.WithEnv("staging"),
		tracer.WithService("frontend"),
		tracer.WithServiceVersion("1.0"),
	)
	// When the tracer is stopped, it will flush everything it has to the Datadog Agent before quitting.
	// Make sure this line stays in your main function.
	defer tracer.Stop()

	// Configure logger
	datadogWriter := &datadogwriter.DatadogWriter{
		Service:  "frontend",
		Hostname: "localhost",
		Tags:     "env:staging,version:1.0",
		Source:   "frontend-service",
	}
	log.Logger = zerolog.New(io.MultiWriter(os.Stdout, datadogWriter)).With().Timestamp().Logger()

	log.Info().Msg("Reading config...")
	jsonFile, err := os.Open("config.json")
	if err != nil {
		log.Error().Msgf("Got error while reading config: %v", err)
	}
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)

	var result map[string]string
	json.Unmarshal([]byte(byteValue), &result)

	serv_port, _ := strconv.Atoi(result["FrontendPort"])
	serv_ip := result["FrontendIP"]
	knative_dns := result["KnativeDomainName"]

	log.Info().Msgf("Read target port: %v", serv_port)
	log.Info().Msgf("Read consul address: %v", result["consulAddress"])
	// log.Info().Msgf("Read jaeger address: %v", result["jaegerAddress"])
	var (
		// port       = flag.Int("port", 5000, "The server port")
		// jaegeraddr = flag.String("jaegeraddr", result["jaegerAddress"], "Jaeger address")
		consuladdr = flag.String("consuladdr", result["consulAddress"], "Consul address")
	)
	flag.Parse()
	// log.Info().Msgf("Initializing jaeger agent [service name: %v | host: %v]...", "frontend", *jaegeraddr)
	// tracer, err := tracing.Init("frontend", *jaegeraddr)
	// if err != nil {
	// 	log.Panic().Msgf("Got error while initializing jaeger agent: %v", err)
	// }
	// log.Info().Msg("Jaeger agent initialized")

	log.Info().Msgf("Initializing consul agent [host: %v]...", *consuladdr)
	registry, err := registry.NewClient(*consuladdr)
	if err != nil {
		log.Panic().Msgf("Got error while initializing consul agent: %v", err)
	}
	log.Info().Msg("Consul agent initialized")

	srv := &frontend.Server{
		KnativeDns: knative_dns,
		Registry:   registry,
		// Tracer:     tracer,
		IpAddr: serv_ip,
		Port:   serv_port,
	}

	log.Info().Msg("Starting server...")
	log.Fatal().Msg(srv.Run().Error())
}
