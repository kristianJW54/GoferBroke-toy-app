package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/kristianJW54/GoferBroke/pkg/gossip"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type clusterSeedAddrs []string

func (a *clusterSeedAddrs) String() string {
	return strings.Join(*a, ",")
}

func (a *clusterSeedAddrs) Set(value string) error {
	*a = append(*a, value)
	return nil
}

type Envelope struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

func normDelta(d *gossip.DeltaUpdateEvent) map[string]any {
	return map[string]any{
		"group":          d.DeltaGroup,
		"key":            d.DeltaKey,
		"version":        d.CurrentVersion,
		"from":           d.PreviousVersion,
		"previous_value": string(d.PreviousValue),
		"value":          string(d.CurrentValue),
	}
}

func normDeltaAdded(d *gossip.DeltaAddedEvent) map[string]any {
	return map[string]any{
		"group": d.DeltaGroup,
		"key":   d.DeltaKey,
		"value": string(d.DeltaValue),
	}

}

func normParticipant(p *gossip.NewParticipantJoin) map[string]any {
	return map[string]any{
		"node": p.Name,
		"time": p.Time,
		"mv":   p.MaxVersion,
	}
}

func normDead(p *gossip.ParticipantFaulty) map[string]any {
	return map[string]any{
		"node":    p.Name,
		"time":    p.Time,
		"address": p.Address,
	}
}

func getDistDir() string {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("failed to get executable path:", err)
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, "dist")
}

func main() {

	modeFlag := flag.String("mode", "node", "Mode: seed | node")
	nodeName := flag.String("name", "node-a", "Node name")
	nodeAddr := flag.String("nodeAddr", "127.0.0.1:8081", "Node listen addr (host:port)")
	clientPort := flag.String("clientPort", "5000", "Client port")
	network := flag.String("network", "LOCAL", "LOCAL | PRIVATE | PUBLIC")
	webAddr := flag.String("web", "127.0.0.1:9091", "Web listen addr (host:port)")
	showLogs := flag.Bool("showLogs", false, "Show logs")

	var routes clusterSeedAddrs
	flag.Var(&routes, "routes", "Route addresses - can be specified multiple times")

	flag.Parse()

	var isSeed bool
	var cNet gossip.ClusterNetworkType
	var net gossip.NodeNetworkType

	switch *modeFlag {
	case "seed":
		isSeed = true
	case "node":
		isSeed = false
	default:
		log.Fatal("mode must be seed | node")
	}

	switch *network {
	case "LOCAL":
		net = gossip.LOCAL
		cNet = gossip.C_LOCAL
	case "PRIVATE":
		net = gossip.PRIVATE
		cNet = gossip.C_PRIVATE
	case "PUBLIC":
		net = gossip.PUBLIC
		cNet = gossip.C_PUBLIC
	default:
		log.Fatal("network must be -> LOCAL | PRIVATE | PUBLIC")
	}

	c, err := gossip.BuildClusterConfig("toy-cluster", func(config *gossip.ClusterConfig) error {

		config.SeedServers = make([]*gossip.Seeds, 0, len(routes))
		for _, r := range routes {
			parts := strings.SplitN(r, ":", 2)
			if len(parts) == 2 {
				config.SeedServers = append(config.SeedServers, &gossip.Seeds{Host: parts[0], Port: parts[1]})
			}
		}

		config.Cluster.ClusterNetworkType = cNet

		return nil

	})

	if err != nil {
		panic(err)
	}

	n, err := gossip.BuildNodeConfig(*nodeName, *nodeAddr, func(cfg *gossip.NodeConfig) (*gossip.NodeConfig, error) {

		parts := strings.SplitN(*nodeAddr, ":", 2)
		if len(parts) == 2 {
			cfg.Host = parts[0]
			cfg.Port = parts[1]
		}

		cfg.ClientPort = *clientPort

		cfg.NetworkType = net

		cfg.IsSeed = isSeed

		cfg.Internal.DefaultLoggerEnabled = *showLogs

		return cfg, nil

	})

	node, err := gossip.NewNodeFromConfig(c, n)
	if err != nil {
		panic(err)
	}

	// simple channel for events
	events := make(chan Envelope, 1024)
	publish := func(ev Envelope) {
		select {
		case events <- ev:
		default:

		}
	}

	_, err = node.OnEvent(gossip.ParticipantMarkedDead, func(event gossip.Event) error {
		if upd, ok := event.Payload().(*gossip.ParticipantFaulty); ok {
			fmt.Println("[EVENT] participant_dead triggered:", upd)
			publish(Envelope{Type: "participant_dead", Payload: normDead(upd)})
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	_, err = node.OnEvent(gossip.DeltaUpdated, func(event gossip.Event) error {

		if delta, ok := event.Payload().(*gossip.DeltaUpdateEvent); ok {
			publish(Envelope{Type: "delta_updated", Payload: normDelta(delta)})
		}
		return nil

	})
	if err != nil {
		panic(err)
	}

	_, err = node.OnEvent(gossip.NewDeltaAdded, func(event gossip.Event) error {

		if delta, ok := event.Payload().(*gossip.DeltaAddedEvent); ok {
			publish(Envelope{Type: "delta_added", Payload: normDeltaAdded(delta)})
		}
		return nil

	})
	if err != nil {
		panic(err)
	}

	_, err = node.OnEvent(gossip.NewParticipantAdded, func(event gossip.Event) error {
		if upd, ok := event.Payload().(*gossip.NewParticipantJoin); ok {
			publish(Envelope{Type: "participant_added", Payload: normParticipant(upd)})
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	node.Start()
	defer node.Stop()

	fmt.Printf("server started on %s\n\n", *webAddr)

	// ----- Minimal Fiber app (per instance) -----
	app := fiber.New()
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	app.Get("/events", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			for ev := range events {
				b, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", b)
				_ = w.Flush()
			}
		})
		return nil
	})

	app.Post("/api/delta", func(c *fiber.Ctx) error {
		body := c.Body()
		fmt.Println("[POST /api/delta] Raw body:", string(body))

		var in struct {
			Group string
			Key   string
			Value string
		}
		if err := c.BodyParser(&in); err != nil {
			fmt.Println("Failed to parse body:", err)
			return fiber.ErrBadRequest
		}

		buf := append([]byte(in.Value), '\r', '\n')
		d := gossip.CreateNewDelta(in.Group, in.Key, gossip.STRING, buf)
		err := node.Add(d)
		if err != nil {
			fmt.Println("Update error:", err)
			return fiber.ErrBadRequest
		}

		fmt.Printf("Parsed delta: %+v\n", d)
		// ...
		return c.JSON(fiber.Map{"ok": true})
	})

	//distDir := getDistDir()
	//fmt.Printf("[web] Serving UI from %s\n", distDir)
	//app.Static("/", distDir)

	app.Static("/", "../dist")

	fmt.Printf("Listening on http://%s\n", *webAddr)
	if err := app.Listen(*webAddr); err != nil {
		log.Fatal(err)
	}

}
