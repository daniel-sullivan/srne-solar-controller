package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/interfaces/solarman"
	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/serve"
	"github.com/spf13/cobra"
)

var configPath string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the service (web dashboard + MQTT publisher)",
	Long:  "Runs a long-lived service that polls inverters and exposes data via a web dashboard (HTMX/SSE) and optional MQTT with Home Assistant auto-discovery.",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&configPath, "config", "srne.toml", "Path to TOML config file")
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg, err := serve.LoadConfig(configPath)
	if err != nil {
		return err
	}

	pollInterval, _ := cfg.PollIntervalDuration()
	settingsRefresh, _ := cfg.SettingsRefreshDuration()

	slog.Info("loaded config",
		"inverters", len(cfg.Inverters),
		"poll_interval", pollInterval,
		"settings_refresh", settingsRefresh,
		"web_port", cfg.Server.WebPort,
		"mqtt", cfg.MQTT != nil,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create hub and web server immediately so the UI is available during startup
	hub := serve.NewHub(nil, pollInterval, settingsRefresh)
	webServer := serve.NewWebServer(hub, nil)

	mpptLabels := make(map[string][2]string, len(cfg.Inverters))
	for _, inv := range cfg.Inverters {
		if inv.MPPT1Label != "" || inv.MPPT2Label != "" {
			mpptLabels[inv.Host] = [2]string{inv.MPPT1Label, inv.MPPT2Label}
		}
	}
	webServer.SetMPPTLabels(mpptLabels)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.WebPort),
		Handler: webServer.Handler(),
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// Start web server immediately
	go func() {
		slog.Info("web server started", "addr", fmt.Sprintf("http://localhost:%d", cfg.Server.WebPort))
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("web server failed", "error", err)
		}
	}()

	// Connect to inverters (this may take time)
	clients := make([]modbus.Client, len(cfg.Inverters))
	for i, inv := range cfg.Inverters {
		client, err := buildServeClient(i, inv)
		if err != nil {
			return err
		}
		defer closeServeClient(client)
		clients[i] = client
	}

	// Initialize inverter system
	hosts := make([]string, len(cfg.Inverters))
	for i, inv := range cfg.Inverters {
		hosts[i] = inv.Host
	}
	system := inverter.NewSystem(clients, hosts)
	if err := system.Init(ctx); err != nil {
		return fmt.Errorf("system init: %w", err)
	}

	units := system.Units()
	for _, u := range units {
		slog.Info("inverter ready",
			"host", u.Host,
			"serial", u.Serial,
			"model", u.Model,
			"parallel_mode", u.ParallelMode,
		)
	}
	slog.Info("system initialized", "units", len(units), "parallel", system.IsParallel())

	// Wire up the system to the hub and web server, start polling
	hub.SetSystem(system)
	webServer.SetSystem(system)
	go hub.Run(ctx)

	// Start MQTT if configured
	if cfg.MQTT != nil {
		pub, mqttErr := serve.NewMQTTPublisher(cfg.MQTT, hub, units, mpptLabels)
		if mqttErr != nil {
			return fmt.Errorf("mqtt: %w", mqttErr)
		}
		go pub.Run(ctx)
		slog.Info("mqtt publisher started", "broker", cfg.MQTT.Broker, "prefix", cfg.MQTT.TopicPrefix)
	}

	// Block until shutdown
	<-ctx.Done()
	slog.Info("shutdown complete")
	return nil
}

func buildServeClient(index int, inv serve.InverterConfig) (modbus.Client, error) {
	switch inv.Driver {
	case "mock":
		sim := mock.NewSim()
		if len(inv.Host) > 0 {
			// Host is already used as the display label in the UI; keep the mock state varied by index.
		}
		if err := sim.Connect(); err != nil {
			return nil, fmt.Errorf("mock inverter %s: %w", inv.Host, err)
		}
		sim.Start(1 * time.Second)
		sim.SetParallelMode(1)
		if index%2 == 0 {
			sim.SetSOC(56)
			sim.SetPV(2600, 1800)
			sim.SetLoad(780)
			sim.SetGridVoltage(107)
		} else {
			sim.SetSOC(56)
			sim.SetPV(2100, 1700)
			sim.SetLoad(720)
			sim.SetGridVoltage(108)
		}
		return sim, nil
	case "solarman":
		slaveID := inv.SlaveID
		if slaveID == 0 {
			slaveID = 1
		}
		client := solarman.NewClient(inv.Host, inv.Port, inv.Serial, slaveID)
		client.Debug = debug
		if err := client.Connect(); err != nil {
			return nil, fmt.Errorf("inverter %s: %w", inv.Host, err)
		}
		if inv.SlaveID == 0 {
			found, probeErr := client.ProbeSlaveID(10)
			if probeErr != nil {
				return nil, fmt.Errorf("inverter %s: %w", inv.Host, probeErr)
			}
			slog.Info("detected slave ID", "host", inv.Host, "slave_id", found)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("inverter %s: unsupported driver %q", inv.Host, inv.Driver)
	}
}

func closeServeClient(client modbus.Client) {
	if sim, ok := client.(*mock.Sim); ok {
		sim.Stop()
	}
	_ = client.Close()
}
