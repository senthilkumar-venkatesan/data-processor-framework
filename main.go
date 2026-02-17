package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/warpstreamlabs/bento/public/service"

	// Register standard components
	_ "github.com/warpstreamlabs/bento/public/components/io"
	_ "github.com/warpstreamlabs/bento/public/components/kafka"
	_ "github.com/warpstreamlabs/bento/public/components/pure"

	// Register custom components
	_ "github.com/data-processor-framework/internal/inputs"
	_ "github.com/data-processor-framework/internal/processors"
	// _ "github.com/data-processor-framework/internal/outputs"  // Uncomment when you create custom outputs
)

func main() {
	// Parse command line flags
	configFlag := flag.String("c", "", "Path to configuration file")
	flag.Parse()

	// Determine config path: -c flag > env var > default
	configPath := *configFlag
	if configPath == "" {
		configPath = os.Getenv("BENTO_CONFIG_PATH")
	}
	if configPath == "" {
		configPath = "config/pipeline.yaml"
	}

	cfg, err := os.ReadFile(configPath)
	if err != nil {
		panic("failed to read config: " + err.Error())
	}

	builder := service.NewStreamBuilder()
	if err := builder.SetYAML(string(cfg)); err != nil {
		panic("failed to parse config: " + err.Error())
	}

	stream, err := builder.Build()
	if err != nil {
		panic("failed to build stream: " + err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := stream.Run(ctx); err != nil {
		panic(err)
	}
}
