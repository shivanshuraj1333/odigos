package collectorprofiles

import (
	"context"
	"log"
	"sync"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"go.opentelemetry.io/collector/receiver/xreceiver"
)

const defaultProfilesPort = "4318"

// ProfileStoreRef is a small interface for HTTP handlers that need StartViewing, GetProfileData, and optional DebugSlots.
type ProfileStoreRef interface {
	StartViewing(sourceKey string)
	GetProfileData(sourceKey string) [][]byte
	MaxSlots() int
	DebugSlots() (activeKeys []string, keysWithData []string)
}

// RunWithStore starts the OTLP profiles gRPC receiver when receiverEnabled is true.
// The caller supplies enablement from Odigos effective configuration (and optional ENABLE_PROFILES_RECEIVER override).
// The caller is responsible for starting the store's cleanup (store.RunCleanup(ctx)).
func RunWithStore(ctx context.Context, store *ProfileStore, receiverEnabled bool) (ProfileStoreRef, *sync.WaitGroup) {
	if !receiverEnabled {
		log.Println("profiles: OTLP receiver disabled (OdigosConfiguration profiling or env override)")
		return store, &sync.WaitGroup{}
	}

	profilesConsumer, err := NewProfilesConsumer(store)
	if err != nil {
		log.Printf("profiles: failed to create consumer: %v", err)
		return store, &sync.WaitGroup{}
	}

	f := otlpreceiver.NewFactory()
	cfg, ok := f.CreateDefaultConfig().(*otlpreceiver.Config)
	if !ok {
		log.Printf("profiles: failed to cast config to otlpreceiver.Config")
		return store, &sync.WaitGroup{}
	}
	cfg.GRPC = configoptional.Some(configgrpc.ServerConfig{
		NetAddr: confignet.AddrConfig{
			Endpoint:  "0.0.0.0:" + defaultProfilesPort,
			Transport: confignet.TransportTypeTCP,
		},
	})
	cfg.HTTP = configoptional.None[otlpreceiver.HTTPConfig]()

	xFactory, ok := f.(xreceiver.Factory)
	if !ok {
		log.Printf("profiles: otlpreceiver factory does not implement xreceiver.Factory")
		return store, &sync.WaitGroup{}
	}

	r, err := xFactory.CreateProfiles(ctx, receivertest.NewNopSettings(f.Type()), cfg, profilesConsumer)
	if err != nil {
		log.Printf("profiles: failed to create receiver: %v", err)
		return store, &sync.WaitGroup{}
	}

	if err := r.Start(ctx, componenttest.NewNopHost()); err != nil {
		log.Printf("profiles: failed to start receiver: %v", err)
		return store, &sync.WaitGroup{}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		if err := r.Shutdown(ctx); err != nil {
			log.Printf("profiles: shutdown error: %v", err)
		}
	}()

	log.Printf("profiles: OTLP gRPC receiver listening on port %s", defaultProfilesPort)
	return store, &wg
}
