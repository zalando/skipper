package main

import (
	"testing"
)

func Test_swarmRedisFlags_Set(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		redisURLs []string
		wantErr   bool
	}{
		{
			name: "split 1",
			args: "skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379",
			redisURLs: []string{
				"skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379",
			},
			wantErr: false,
		}, {
			name: "split 2",
			args: "skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379,skipper-redis-1.skipper-redis.kube-system.svc.cluster.local.:6379",
			redisURLs: []string{
				"skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379",
				"skipper-redis-1.skipper-redis.kube-system.svc.cluster.local.:6379",
			},
			wantErr: false,
		}, {
			name: "split 3",
			args: "skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379,skipper-redis-1.skipper-redis.kube-system.svc.cluster.local.:6379,127.0.0.1:1234",
			redisURLs: []string{
				"skipper-redis-0.skipper-redis.kube-system.svc.cluster.local.:6379",
				"skipper-redis-1.skipper-redis.kube-system.svc.cluster.local.:6379",
				"127.0.0.1:1234",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sf := &swarmRedisFlags{
				redisURLs: tt.redisURLs,
			}
			if err := sf.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("swarmRedisFlags.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
