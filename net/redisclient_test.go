package net

import (
	"context"
	"log"
	"os/exec"
	"testing"
	"time"

	"github.com/zalando/skipper/tracing/tracers/basic"
)

func startRedis(port, password string) func() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	cmdArgs := []string{"--port", port}
	if password != "" {
		cmdArgs = append(cmdArgs, "--requirepass")
		cmdArgs = append(cmdArgs, password)
	}
	cmd := exec.CommandContext(ctx, "redis-server", cmdArgs...)
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Run '%q %q' failed, caused by: %s", cmd.Path, cmd.Args, err)
	}

	return func() { cancel(); _ = cmd.Wait() }
}

func TestRedisClient(t *testing.T) {
	tracer, err := basic.InitTracer([]string{"recorder=in-memory"})
	if err != nil {
		t.Fatalf("Failed to get a tracer: %v", err)
	}

	t.Log("starting redis...")
	redisPort := "16383"
	cancel := startRedis(redisPort, "")
	defer cancel()
	t.Log("started redis")

	for _, tt := range []struct {
		name    string
		options *RedisOptions
		wantErr bool
	}{
		{
			name: "All defaults",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			wantErr: false,
		},
		{
			name: "With tracer",
			options: &RedisOptions{
				Addrs:  []string{"127.0.0.1:" + redisPort},
				Tracer: tracer,
			},
			wantErr: false,
		},
		{
			name: "With metrics",
			options: &RedisOptions{
				Addrs:               []string{"127.0.0.1:" + redisPort},
				ConnMetricsInterval: 10 * time.Millisecond,
			},
			wantErr: false,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()

			if !cli.RingAvailable() {
				t.Errorf("Failed to have a connected redis client, ring not available")
			}

			// if tt.options.Tracer != opentracing.Tracer{} { // cli.tracer == opentracing.Tracer{}{
			// 	t.Errorf("Found an unexpected tracer, want: %v, got: %v", tt.options.Tracer, cli.tracer)
			// }

			if tt.options.ConnMetricsInterval != defaultConnMetricsInterval {
				cli.StartMetricsCollection()
				time.Sleep(tt.options.ConnMetricsInterval)
			}
		})
	}
}

func TestRedisClientZAddZCard(t *testing.T) {
	redisPort := "16384"
	cancel := startRedis(redisPort, "")
	defer cancel()

	for _, tt := range []struct {
		name    string
		options *RedisOptions
		key     string
		val     int64
		score   float64
		expire  time.Duration
		zcard   int64
		wantErr bool
	}{
		{
			name: "add none",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			wantErr: true,
		},
		{
			name: "add one",
			options: &RedisOptions{
				Addrs: []string{"127.0.0.1:" + redisPort},
			},
			key:     "k1",
			val:     10,
			score:   5.0,
			zcard:   1,
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("run TestRedisClientZAddZCard %s", tt.name)
			cli := NewRedisRingClient(tt.options)
			defer cli.Close()

			_, err := cli.ZAdd(context.Background(), tt.key, tt.val, tt.score)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			val, err := cli.ZCard(context.Background(), tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcard {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			_, err = cli.ZRem(context.Background(), tt.key, tt.val)
			if err != nil {
				t.Errorf("Failed to remove key %s: %v", tt.key, err)
			}

		})
	}
}
