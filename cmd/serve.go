package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
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
		"bms", cfg.BMS != nil,
		"conditioning", cfg.Conditioning != nil,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	hubCtx, stopHub := context.WithCancel(context.Background())
	defer stopHub()

	// Create hub and web server immediately so the UI is available during startup
	hub := serve.NewHub(nil, pollInterval, settingsRefresh)
	webServer := serve.NewWebServer(hub, nil)
	var bmsStore *interpack.Store

	if cfg.BMS != nil && cfg.BMS.SerialDevice != "" {
		bmsStore = &interpack.Store{}
		webServer.SetBMS(bmsStore, cfg.BMS)
		go interpack.Run(ctx, cfg.BMS.SerialDevice, bmsStore.Record, bmsStore.RecordSettings)
		slog.Info("bms monitor started", "device", cfg.BMS.SerialDevice)
	}

	var conditioner *serve.ConditioningService
	if cfg.Conditioning != nil {
		conditioner = serve.NewConditioningService(hub, bmsStore, cfg.Conditioning.StateFile)
		webServer.SetConditioning(conditioner)
		// Claim the write path before the web server or inverter hub becomes
		// available when a prior session's journal needs restoration.
		if _, err := os.Stat(cfg.Conditioning.StateFile); err == nil || !errors.Is(err, os.ErrNotExist) {
			conditioner.ExpectRecovery()
			if err := hub.ReserveConditioning(); err != nil {
				return fmt.Errorf("reserve conditioning recovery: %w", err)
			}
		}
	}

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
	go hub.Run(hubCtx)

	var stopConditioningTicks context.CancelFunc
	if conditioner != nil {
		recoverCtx, cancelRecovery := context.WithTimeout(context.Background(), 2*time.Minute)
		if err := conditioner.Recover(recoverCtx); err != nil {
			slog.Error("conditioning restoration pending", "error", err)
		}
		cancelRecovery()

		tickCtx, cancelTicks := context.WithCancel(hubCtx)
		defer cancelTicks()
		stopConditioningTicks = cancelTicks
		go runConditioningTicks(tickCtx, conditioner)
	}

	// Start MQTT if configured
	if cfg.MQTT != nil {
		pub, mqttErr := serve.NewMQTTPublisher(cfg.MQTT, hub, units, mpptLabels)
		if mqttErr != nil {
			return fmt.Errorf("mqtt: %w", mqttErr)
		}
		if conditioner != nil {
			pub.SetConditioning(conditioner)
		}
		go pub.Run(ctx)
		slog.Info("mqtt publisher started", "broker", cfg.MQTT.Broker, "prefix", cfg.MQTT.TopicPrefix)
	}

	// Block until shutdown
	<-ctx.Done()
	if stopConditioningTicks != nil {
		stopConditioningTicks()
	}
	if conditioner != nil {
		restoreCtx, cancelRestore := context.WithTimeout(context.Background(), 2*time.Minute)
		if err := conditioner.Stop(restoreCtx); err != nil {
			slog.Error("conditioning restoration remains pending on shutdown", "error", err)
		}
		cancelRestore()
	}
	stopHub()
	slog.Info("shutdown complete")
	return nil
}

func runConditioningTicks(ctx context.Context, conditioner *serve.ConditioningService) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			status := conditioner.Status()
			if !status.Active && !status.RestorePending {
				continue
			}
			operationCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			var err error
			if status.RestorePending && !status.Active {
				err = conditioner.Recover(operationCtx)
			} else {
				err = conditioner.Tick(operationCtx, now)
			}
			cancel()
			if err != nil {
				slog.Warn("conditioning update or restoration pending", "error", err)
			}
		}
	}
}

func buildServeClient(index int, inv serve.InverterConfig) (modbus.Client, error) {
	switch inv.Driver {
	case "mock":
		sim := mock.NewSim()
		// Host is already used as the display label in the UI; mock state is varied by index below.
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
