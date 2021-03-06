//                           _       _
// __      _____  __ ___   ___  __ _| |_ ___
// \ \ /\ / / _ \/ _` \ \ / / |/ _` | __/ _ \
//  \ V  V /  __/ (_| |\ V /| | (_| | ||  __/
//   \_/\_/ \___|\__,_| \_/ |_|\__,_|\__\___|
//
//  Copyright © 2016 - 2020 SeMI Technologies B.V. All rights reserved.
//
//  CONTACT: hello@semi.technology
//

package rest

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/elastic/go-elasticsearch/v5"
	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/semi-technologies/weaviate/adapters/clients/contextionary"
	"github.com/semi-technologies/weaviate/adapters/handlers/rest/operations"
	"github.com/semi-technologies/weaviate/adapters/handlers/rest/state"
	"github.com/semi-technologies/weaviate/adapters/locks"
	"github.com/semi-technologies/weaviate/adapters/repos/db"
	"github.com/semi-technologies/weaviate/adapters/repos/esvector"
	"github.com/semi-technologies/weaviate/adapters/repos/etcd"
	"github.com/semi-technologies/weaviate/entities/models"
	"github.com/semi-technologies/weaviate/entities/search"
	"github.com/semi-technologies/weaviate/usecases/classification"
	"github.com/semi-technologies/weaviate/usecases/config"
	"github.com/semi-technologies/weaviate/usecases/kinds"
	"github.com/semi-technologies/weaviate/usecases/nearestneighbors"
	"github.com/semi-technologies/weaviate/usecases/network/common/peers"
	"github.com/semi-technologies/weaviate/usecases/projector"
	schemaUC "github.com/semi-technologies/weaviate/usecases/schema"
	"github.com/semi-technologies/weaviate/usecases/schema/migrate"
	"github.com/semi-technologies/weaviate/usecases/sempath"
	"github.com/semi-technologies/weaviate/usecases/traverser"
	libvectorizer "github.com/semi-technologies/weaviate/usecases/vectorizer"
	"github.com/sirupsen/logrus"
)

const MinimumRequiredContextionaryVersion = "0.4.19"

func makeConfigureServer(appState *state.State) func(*http.Server, string, string) {
	return func(s *http.Server, scheme, addr string) {
		// Add properties to the config
		appState.ServerConfig.Hostname = addr
		appState.ServerConfig.Scheme = scheme
	}
}

type vectorRepo interface {
	kinds.BatchVectorRepo
	traverser.VectorSearcher
	classification.VectorRepo
	SetSchemaGetter(schemaUC.SchemaGetter)
	WaitForStartup(time.Duration) error
}

type vectorizer interface {
	kinds.Vectorizer
	traverser.CorpiVectorizer
	SetIndexChecker(libvectorizer.IndexCheck)
}

type explorer interface {
	GetClass(ctx context.Context, params traverser.GetParams) ([]interface{}, error)
	Concepts(ctx context.Context, params traverser.ExploreParams) ([]search.Result, error)
}

func configureAPI(api *operations.WeaviateAPI) http.Handler {
	appState, etcdClient, esClient := startupRoutine()

	validateContextionaryVersion(appState)

	api.ServeError = errors.ServeError

	api.JSONConsumer = runtime.JSONConsumer()

	api.OidcAuth = func(token string, scopes []string) (*models.Principal, error) {
		return appState.OIDC.ValidateAndExtract(token, scopes)
	}

	api.Logger = func(msg string, args ...interface{}) {
		appState.Logger.WithField("action", "restapi_management").Infof(msg, args...)
	}

	var vectorRepo vectorRepo
	var vectorMigrator migrate.Migrator
	var vectorizer vectorizer
	var migrator migrate.Migrator
	var explorer explorer
	nnExtender := nearestneighbors.NewExtender(appState.Contextionary)
	featureProjector := projector.New()
	pathBuilder := sempath.New(appState.Contextionary)

	if appState.ServerConfig.Config.Standalone {
		repo := db.New(appState.Logger, db.Config{
			RootPath: appState.ServerConfig.Config.Persistence.DataPath,
		})
		vectorMigrator = db.NewMigrator(repo)
		vectorRepo = repo
		migrator = vectorMigrator
		vectorizer = libvectorizer.New(appState.Contextionary, nil)
		explorer = traverser.NewExplorer(repo, vectorizer, libvectorizer.NormalizedDistance,
			appState.Logger, nnExtender, featureProjector, pathBuilder)
	} else {
		repo := esvector.NewRepo(esClient, appState.Logger, nil,
			*appState.ServerConfig.Config.VectorIndex.NumberOfShards,     // guaranteed not to be nil as there are defaults
			*appState.ServerConfig.Config.VectorIndex.AutoExpandReplicas, // guaranteed not to be nil as there are defaults
		)
		vectorMigrator = esvector.NewMigrator(repo)
		vectorRepo = repo
		migrator = vectorMigrator
		vectorizer = libvectorizer.New(appState.Contextionary, nil)
		explorer = traverser.NewExplorer(repo, vectorizer, libvectorizer.NormalizedDistance,
			appState.Logger, nnExtender, featureProjector, pathBuilder)
	}

	schemaRepo := etcd.NewSchemaRepo(etcdClient)
	classifierRepo := etcd.NewClassificationRepo(etcdClient)

	schemaManager, err := schemaUC.NewManager(migrator, schemaRepo,
		appState.Locks, appState.Network, appState.Logger, appState.Contextionary,
		appState.Authorizer, appState.StopwordDetector)
	if err != nil {
		appState.Logger.
			WithField("action", "startup").WithError(err).
			Fatal("could not initialize schema manager")
		os.Exit(1)
	}

	vectorRepo.SetSchemaGetter(schemaManager)
	vectorizer.SetIndexChecker(schemaManager)

	err = vectorRepo.WaitForStartup(2 * time.Minute)
	if err != nil {
		appState.Logger.
			WithError(err).
			WithField("action", "startup").WithError(err).
			Fatal("db didn't start up")
		os.Exit(1)
	}

	kindsManager := kinds.NewManager(appState.Locks,
		schemaManager, appState.Network, appState.ServerConfig, appState.Logger,
		appState.Authorizer, vectorizer, vectorRepo, nnExtender, featureProjector)
	batchKindsManager := kinds.NewBatchManager(vectorRepo, vectorizer, appState.Locks,
		schemaManager, appState.Network, appState.ServerConfig, appState.Logger,
		appState.Authorizer)
	vectorInspector := libvectorizer.NewInspector(appState.Contextionary)

	kindsTraverser := traverser.NewTraverser(appState.ServerConfig, appState.Locks,
		appState.Logger, appState.Authorizer, vectorizer,
		vectorRepo, explorer, schemaManager)

	classifier := classification.New(schemaManager, classifierRepo, vectorRepo, appState.Authorizer,
		appState.Contextionary, appState.Logger)

	updateSchemaCallback := makeUpdateSchemaCall(appState.Logger, appState, kindsTraverser)
	schemaManager.RegisterSchemaUpdateCallback(updateSchemaCallback)

	// manually update schema once
	schema := schemaManager.GetSchemaSkipAuth()
	updateSchemaCallback(schema)

	appState.Network.RegisterUpdatePeerCallback(func(peers peers.Peers) {
		schemaManager.TriggerSchemaUpdateCallbacks()
	})
	appState.Network.RegisterSchemaGetter(schemaManager)

	setupSchemaHandlers(api, schemaManager)
	setupKindHandlers(api, kindsManager, appState.ServerConfig.Config, appState.Logger)
	setupKindBatchHandlers(api, batchKindsManager)
	setupC11yHandlers(api, vectorInspector, appState.Contextionary)
	setupGraphQLHandlers(api, appState)
	setupMiscHandlers(api, appState.ServerConfig, appState.Network, schemaManager, appState.Contextionary)
	setupClassificationHandlers(api, classifier)

	api.ServerShutdown = func() {}
	configureServer = makeConfigureServer(appState)
	setupMiddlewares := makeSetupMiddlewares(appState)
	setupGlobalMiddleware := makeSetupGlobalMiddleware(appState)
	return setupGlobalMiddleware(api.Serve(setupMiddlewares))
}

// TODO: Split up and don't write into global variables. Instead return an appState
func startupRoutine() (*state.State, *clientv3.Client, *elasticsearch.Client) {
	appState := &state.State{}
	// context for the startup procedure. (So far the only subcommand respecting
	// the context is the schema initialization, as this uses the etcd client
	// requiring context. Nevertheless it would make sense to have everything
	// that goes on in here pay attention to the context, so we can have a
	// "startup in x seconds or fail")
	ctx := context.Background()
	// The timeout is arbitrary we have to adjust it as we go along, if we
	// realize it is to big/small
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	logger := logger()
	appState.Logger = logger

	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("created startup context, nothing done so far")

	// Load the config using the flags
	serverConfig := &config.WeaviateConfig{}
	appState.ServerConfig = serverConfig
	err := serverConfig.LoadConfig(connectorOptionGroup, logger)
	if err != nil {
		logger.WithField("action", "startup").WithError(err).Error("could not load config")
		logger.Exit(1)
	}

	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("config loaded")

	appState.OIDC = configureOIDC(appState)
	appState.AnonymousAccess = configureAnonymousAccess(appState)
	appState.Authorizer = configureAuthorizer(appState)

	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("configured OIDC and anonymous access client")

	appState.Network = connectToNetwork(logger, appState.ServerConfig.Config)
	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("network configured")

	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("created db connector")

	// parse config store URL
	configURL := serverConfig.Config.ConfigurationStorage.URL
	configStore, err := url.Parse(configURL)
	if err != nil || configURL == "" {
		logger.WithField("action", "startup").WithField("url", configURL).
			WithError(err).Error("cannot parse config store URL")
		logger.Exit(1)
	}

	// Construct a distributed lock
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{configStore.String()}})
	if err != nil {
		logger.WithField("action", "startup").
			WithError(err).Error("cannot construct distributed lock with etcd")
		logger.Exit(1)
	}
	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("created etcd client")

	esClient, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{serverConfig.Config.VectorIndex.URL},
	})
	if err != nil {
		logger.WithField("action", "startup").
			WithError(err).Error("cannot create es client for vector index")
		logger.Exit(1)
	}
	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("created es client for vector index")

	// new lock
	etcdLock, err := locks.NewEtcdLock(etcdClient, "/weaviate/schema-connector-rw-lock", logger)
	if err != nil {
		logger.WithField("action", "startup").
			WithError(err).Error("cannot create etcd-based lock")
		logger.Exit(1)
	}
	appState.Locks = etcdLock

	// appState.Locks = &dummyLock{}

	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("created etcd session")
		// END remove

	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("initialized schema")

	logger.WithField("action", "startup").WithField("startup_time_left", timeTillDeadline(ctx)).
		Debug("initialized stopword detector")

	c11y, err := contextionary.NewClient(appState.ServerConfig.Config.Contextionary.URL)
	if err != nil {
		logger.WithField("action", "startup").
			WithError(err).Error("cannot create c11y client")
		logger.Exit(1)
	}

	appState.StopwordDetector = c11y
	appState.Contextionary = c11y

	return appState, etcdClient, esClient
}

// logger does not parse the regular config object, as logging needs to be
// configured before the configuration is even loaded/parsed. We are thus
// "manually" reading the desired env vars and set reasonable defaults if they
// are not set.
//
// Defaults to log level info and json format
func logger() *logrus.Logger {
	logger := logrus.New()
	if os.Getenv("LOG_FORMAT") != "text" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}
	if os.Getenv("LOG_LEVEL") == "debug" {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	return logger
}

type dummyLock struct{}

func (d *dummyLock) LockConnector() (func() error, error) {
	return func() error { return nil }, nil
}

func (d *dummyLock) LockSchema() (func() error, error) {
	return func() error { return nil }, nil
}

func validateContextionaryVersion(appState *state.State) {
	for {
		time.Sleep(1 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		v, err := appState.Contextionary.Version(ctx)
		if err != nil {
			appState.Logger.WithField("action", "startup_check_contextionary").WithError(err).
				Warnf("could not connect to contextionary at startup, trying again in 1 sec")
			continue
		}

		ok, err := extractVersionAndCompare(v, MinimumRequiredContextionaryVersion)
		if err != nil {
			appState.Logger.WithField("action", "startup_check_contextionary").
				WithField("requiredMinimumContextionaryVersion", MinimumRequiredContextionaryVersion).
				WithField("contextionaryVersion", v).
				WithError(err).
				Warnf("cannot determine if contextionary version is compatible. This is fine in development, but probelematic if you see this production")
			break
		}

		if ok {
			appState.Logger.WithField("action", "startup_check_contextionary").
				WithField("requiredMinimumContextionaryVersion", MinimumRequiredContextionaryVersion).
				WithField("contextionaryVersion", v).
				Infof("found a valid contextionary version")
			break
		} else {
			appState.Logger.WithField("action", "startup_check_contextionary").
				WithField("requiredMinimumContextionaryVersion", MinimumRequiredContextionaryVersion).
				WithField("contextionaryVersion", v).
				Fatalf("insufficient contextionary version, cannot start up")
			break
		}
	}
}
